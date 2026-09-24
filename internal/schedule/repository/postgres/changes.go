package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/internal/schedule/repository"
	pgdb "github.com/anton1ks96/mykct-api/pkg/database/postgres"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/jmoiron/sqlx"
)

// Соответствие интерфейсам проверяется на этапе компиляции.
var (
	_ repository.ChangeRepository = (*ChangeRepository)(nil)
	_ pgdb.ExpiryCleaner          = (*ChangeRepository)(nil)
)

// changeJSON - одно изменение занятия в JSONB.
type changeJSON struct {
	Kind   string     `json:"kind"`
	Fields []string   `json:"fields,omitempty"`
	Before *eventJSON `json:"before,omitempty"`
	After  *eventJSON `json:"after,omitempty"`
}

// weekChangesRow - строка таблицы schedule_changes.
type weekChangesRow struct {
	ID         int64     `db:"id"`
	GroupName  string    `db:"group_name"`
	WeekStart  string    `db:"week_start"`
	DetectedAt time.Time `db:"detected_at"`
	Changes    []byte    `db:"changes"`
}

// marshalChanges собирает изменения доменной модели в JSONB.
func marshalChanges(changes []domain.EventChange) ([]byte, error) {
	items := make([]changeJSON, 0, len(changes))
	for _, change := range changes {
		item := changeJSON{Kind: change.Kind, Fields: change.Fields}
		if change.Before != nil {
			before := eventFromDomain(*change.Before)
			item.Before = &before
		}
		if change.After != nil {
			after := eventFromDomain(*change.After)
			item.After = &after
		}
		items = append(items, item)
	}

	raw, err := marshalJSONB(items)
	if err != nil {
		return nil, fmt.Errorf("failed to encode schedule changes: %w", err)
	}

	return raw, nil
}

// unmarshalChanges разбирает изменения из JSONB обратно в доменную модель.
func unmarshalChanges(raw []byte) ([]domain.EventChange, error) {
	var items []changeJSON
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("failed to decode schedule changes: %w", err)
	}

	changes := make([]domain.EventChange, 0, len(items))
	for _, item := range items {
		change := domain.EventChange{Kind: item.Kind, Fields: item.Fields}
		if item.Before != nil {
			before := item.Before.toDomain()
			change.Before = &before
		}
		if item.After != nil {
			after := item.After.toDomain()
			change.After = &after
		}
		changes = append(changes, change)
	}

	return changes, nil
}

// ChangeRepository хранит изменения расписания в PostgreSQL.
type ChangeRepository struct {
	db  *sqlx.DB
	ttl time.Duration
}

// NewChangeRepository создаёт хранилище изменений поверх клиента PostgreSQL.
func NewChangeRepository(db *sqlx.DB, ttl time.Duration) *ChangeRepository {
	return &ChangeRepository{db: db, ttl: ttl}
}

// Save записывает замеченную разницу. Одна строка - одно будущее уведомление.
func (r *ChangeRepository) Save(ctx context.Context, changes *domain.WeekChanges) error {
	op := logger.NewLogOp(ctx, log, "Save")

	body, err := marshalChanges(changes.Changes)
	if err != nil {
		op.Failed(err).Str("group", changes.Group).Msg("failed to encode schedule changes")
		return err
	}

	query := `
		INSERT INTO schedule_changes (group_name, week_start, detected_at, changes, notified_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err = r.db.ExecContext(ctx, query, changes.Group, changes.WeekStart, changes.DetectedAt,
		body, changes.NotifiedAt, changes.DetectedAt.Add(r.ttl))
	if err != nil {
		op.Failed(err).Str("group", changes.Group).Msg("failed to save schedule changes")
		return fmt.Errorf("failed to save schedule changes: %w", err)
	}

	op.Debug().Str("group", changes.Group).Str("week_start", changes.WeekStart).
		Int("changes", len(changes.Changes)).Msg("schedule changes saved")

	return nil
}

// Pending возвращает неразосланные разницы, замеченные не раньше since.
func (r *ChangeRepository) Pending(ctx context.Context, since time.Time) ([]*domain.WeekChanges, error) {
	op := logger.NewLogOp(ctx, log, "Pending")

	query := `
		SELECT id, group_name, week_start, detected_at, changes
		FROM schedule_changes
		WHERE notified_at IS NULL AND detected_at >= $1
		ORDER BY detected_at`

	var rows []weekChangesRow
	if err := r.db.SelectContext(ctx, &rows, query, since); err != nil {
		op.Failed(err).Msg("failed to find pending schedule changes")
		return nil, fmt.Errorf("failed to find pending schedule changes: %w", err)
	}

	out := make([]*domain.WeekChanges, 0, len(rows))
	for _, row := range rows {
		changes, err := unmarshalChanges(row.Changes)
		if err != nil {
			op.Failed(err).Msg("failed to decode pending schedule changes")
			return nil, err
		}
		out = append(out, &domain.WeekChanges{
			ID:         strconv.FormatInt(row.ID, 10),
			Group:      row.GroupName,
			WeekStart:  row.WeekStart,
			DetectedAt: row.DetectedAt,
			Changes:    changes,
		})
	}

	return out, nil
}

// MarkNotified отмечает рассылку разницы. Условие notified_at IS NULL стоит в
// запросе, поэтому при нескольких инстансах рассылкой владеет ровно один.
func (r *ChangeRepository) MarkNotified(ctx context.Context, id string, at time.Time) (bool, error) {
	changeID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid schedule change id %q: %w", id, err)
	}

	query := `UPDATE schedule_changes SET notified_at = $2 WHERE id = $1 AND notified_at IS NULL`
	res, err := r.db.ExecContext(ctx, query, changeID, at)
	if err != nil {
		logger.NewLogOp(ctx, log, "MarkNotified").Failed(err).
			Msg("failed to mark schedule change notified")
		return false, fmt.Errorf("failed to mark schedule change notified: %w", err)
	}

	updated, _ := res.RowsAffected()

	return updated == 1, nil
}

// DeleteExpired удаляет разницы, которые уже никому не нужны.
func (r *ChangeRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM schedule_changes WHERE expires_at <= $1`, time.Now())
	if err != nil {
		logger.NewLogOp(ctx, log, "DeleteExpired").Failed(err).
			Msg("failed to delete expired schedule changes")
		return 0, fmt.Errorf("failed to delete expired schedule changes: %w", err)
	}

	deleted, _ := res.RowsAffected()

	return deleted, nil
}
