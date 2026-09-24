// Package postgres хранит снимки ответов портала, из которых расписание
// отдаётся, пока портал колледжа недоступен.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/internal/schedule/repository"
	pgdb "github.com/anton1ks96/mykct-api/pkg/database/postgres"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/jmoiron/sqlx"
)

// Соответствие интерфейсам проверяется на этапе компиляции.
var (
	_ repository.SnapshotRepository = (*SnapshotRepository)(nil)
	_ pgdb.ExpiryCleaner            = (*SnapshotRepository)(nil)
)

var log = logger.ComponentLogger("schedule.repository")

// snapshotRow - строка таблицы schedule_snapshots.
type snapshotRow struct {
	GroupName   string    `db:"group_name"`
	PeriodStart string    `db:"period_start"`
	PeriodEnd   string    `db:"period_end"`
	Events      []byte    `db:"events"`
	FetchedAt   time.Time `db:"fetched_at"`
}

// toDomain переводит строку снимка в доменную модель.
func (r *snapshotRow) toDomain() (*domain.Snapshot, error) {
	events, err := unmarshalEvents(r.Events)
	if err != nil {
		return nil, err
	}

	return &domain.Snapshot{
		Group:     r.GroupName,
		Start:     r.PeriodStart,
		End:       r.PeriodEnd,
		Events:    events,
		FetchedAt: r.FetchedAt,
	}, nil
}

// classDetailsRow - строка таблицы class_details_snapshots.
type classDetailsRow struct {
	ClID      string    `db:"cl_id"`
	Details   []byte    `db:"details"`
	FetchedAt time.Time `db:"fetched_at"`
}

// SnapshotRepository хранит снимки ответов портала в PostgreSQL.
type SnapshotRepository struct {
	db  *sqlx.DB
	ttl time.Duration
}

// NewSnapshotRepository создаёт хранилище снимков поверх клиента PostgreSQL.
func NewSnapshotRepository(db *sqlx.DB, ttl time.Duration) *SnapshotRepository {
	return &SnapshotRepository{db: db, ttl: ttl}
}

// SaveSchedule сохраняет снимок расписания, затирая предыдущий за тот же период.
func (r *SnapshotRepository) SaveSchedule(ctx context.Context, snapshot *domain.Snapshot) error {
	op := logger.NewLogOp(ctx, log, "SaveSchedule")

	events, err := marshalEvents(snapshot.Events)
	if err != nil {
		op.Failed(err).Str("group", snapshot.Group).Msg("failed to encode schedule snapshot")
		return fmt.Errorf("failed to encode schedule snapshot: %w", err)
	}

	query := `
		INSERT INTO schedule_snapshots (group_name, period_start, period_end, events, fetched_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (group_name, period_start, period_end) DO UPDATE SET
			events = EXCLUDED.events,
			fetched_at = EXCLUDED.fetched_at,
			expires_at = EXCLUDED.expires_at`
	_, err = r.db.ExecContext(ctx, query, snapshot.Group, snapshot.Start, snapshot.End,
		events, snapshot.FetchedAt, snapshot.FetchedAt.Add(r.ttl))
	if err != nil {
		op.Failed(err).Str("group", snapshot.Group).Msg("failed to save schedule snapshot")
		return fmt.Errorf("failed to save schedule snapshot: %w", err)
	}

	op.Debug().Str("group", snapshot.Group).Int("events", len(snapshot.Events)).
		Msg("schedule snapshot saved")

	return nil
}

// FindSchedule возвращает последний снимок расписания за период. Протухший
// снимок не отдаётся: чистка приходит с задержкой, а срок кэша точный.
func (r *SnapshotRepository) FindSchedule(ctx context.Context, group, start, end string) (*domain.Snapshot, error) {
	op := logger.NewLogOp(ctx, log, "FindSchedule")

	query := `
		SELECT group_name, period_start, period_end, events, fetched_at
		FROM schedule_snapshots
		WHERE group_name = $1 AND period_start = $2 AND period_end = $3 AND expires_at > $4`

	var row snapshotRow
	if err := r.db.GetContext(ctx, &row, query, group, start, end, time.Now()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			op.Debug().Str("group", group).Msg("schedule snapshot not found")
			return nil, domain.ErrScheduleUnavailable
		}
		op.Failed(err).Str("group", group).Msg("failed to find schedule snapshot")
		return nil, fmt.Errorf("failed to find schedule snapshot: %w", err)
	}

	snapshot, err := row.toDomain()
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to decode schedule snapshot")
		return nil, fmt.Errorf("failed to decode schedule snapshot: %w", err)
	}

	op.Debug().Str("group", group).Time("fetched_at", snapshot.FetchedAt).
		Msg("schedule snapshot found")

	return snapshot, nil
}

// SaveClassDetails сохраняет детали занятия, затирая предыдущие.
func (r *SnapshotRepository) SaveClassDetails(ctx context.Context, details *domain.ClassDetails) error {
	op := logger.NewLogOp(ctx, log, "SaveClassDetails")

	body, err := marshalJSONB(details.Details)
	if err != nil {
		op.Failed(err).Str("clid", details.ClID).Msg("failed to encode class details snapshot")
		return fmt.Errorf("failed to encode class details snapshot: %w", err)
	}

	query := `
		INSERT INTO class_details_snapshots (cl_id, details, fetched_at, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (cl_id) DO UPDATE SET
			details = EXCLUDED.details,
			fetched_at = EXCLUDED.fetched_at,
			expires_at = EXCLUDED.expires_at`
	_, err = r.db.ExecContext(ctx, query, details.ClID, body,
		details.FetchedAt, details.FetchedAt.Add(r.ttl))
	if err != nil {
		op.Failed(err).Str("clid", details.ClID).Msg("failed to save class details snapshot")
		return fmt.Errorf("failed to save class details snapshot: %w", err)
	}

	op.Debug().Str("clid", details.ClID).Msg("class details snapshot saved")

	return nil
}

// FindClassDetails возвращает последние сохранённые детали занятия.
func (r *SnapshotRepository) FindClassDetails(ctx context.Context, clid string) (*domain.ClassDetails, error) {
	op := logger.NewLogOp(ctx, log, "FindClassDetails")

	query := `
		SELECT cl_id, details, fetched_at
		FROM class_details_snapshots
		WHERE cl_id = $1 AND expires_at > $2`

	var row classDetailsRow
	if err := r.db.GetContext(ctx, &row, query, clid, time.Now()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			op.Debug().Str("clid", clid).Msg("class details snapshot not found")
			return nil, domain.ErrClassDetailsUnavailable
		}
		op.Failed(err).Str("clid", clid).Msg("failed to find class details snapshot")
		return nil, fmt.Errorf("failed to find class details snapshot: %w", err)
	}

	var body map[string]any
	if err := json.Unmarshal(row.Details, &body); err != nil {
		op.Failed(err).Str("clid", clid).Msg("failed to decode class details snapshot")
		return nil, fmt.Errorf("failed to decode class details snapshot: %w", err)
	}

	op.Debug().Str("clid", clid).Time("fetched_at", row.FetchedAt).
		Msg("class details snapshot found")

	return &domain.ClassDetails{
		ClID:      row.ClID,
		Details:   body,
		FetchedAt: row.FetchedAt,
	}, nil
}

// DeleteExpired удаляет протухшие снимки расписания и деталей занятий.
func (r *SnapshotRepository) DeleteExpired(ctx context.Context) (int64, error) {
	op := logger.NewLogOp(ctx, log, "DeleteExpired")

	now := time.Now()

	var deleted int64
	for _, table := range []string{"schedule_snapshots", "class_details_snapshots"} {
		res, err := r.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE expires_at <= $1`, now)
		if err != nil {
			op.Failed(err).Str("table", table).Msg("failed to delete expired snapshots")
			return deleted, fmt.Errorf("failed to delete expired rows from %s: %w", table, err)
		}
		count, _ := res.RowsAffected()
		deleted += count
	}

	return deleted, nil
}
