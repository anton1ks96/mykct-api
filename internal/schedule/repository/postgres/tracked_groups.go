package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/repository"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/jmoiron/sqlx"
)

// Соответствие интерфейсу проверяется на этапе компиляции.
var _ repository.TrackedGroupRepository = (*TrackedGroupRepository)(nil)

// TrackedGroupRepository хранит отметки о слежении за группами в PostgreSQL.
// Без срока жизни: отметка переживает любые состояния недель.
type TrackedGroupRepository struct {
	db *sqlx.DB
}

// NewTrackedGroupRepository создаёт хранилище отметок поверх клиента PostgreSQL.
func NewTrackedGroupRepository(db *sqlx.DB) *TrackedGroupRepository {
	return &TrackedGroupRepository{db: db}
}

// All возвращает группы, за которыми уже следят.
func (r *TrackedGroupRepository) All(ctx context.Context) ([]string, error) {
	op := logger.NewLogOp(ctx, log, "All")

	var groups []string
	if err := r.db.SelectContext(ctx, &groups, `SELECT group_name FROM schedule_tracked_groups`); err != nil {
		op.Failed(err).Msg("failed to list tracked groups")
		return nil, fmt.Errorf("failed to list tracked groups: %w", err)
	}

	op.Debug().Int("groups", len(groups)).Msg("tracked groups listed")

	return groups, nil
}

// Track отмечает начало слежения за группой. Повторный вызов момент не сдвигает.
func (r *TrackedGroupRepository) Track(ctx context.Context, group string, at time.Time) error {
	op := logger.NewLogOp(ctx, log, "Track")

	query := `
		INSERT INTO schedule_tracked_groups (group_name, tracked_since)
		VALUES ($1, $2)
		ON CONFLICT (group_name) DO NOTHING`
	res, err := r.db.ExecContext(ctx, query, group, at)
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to track group")
		return fmt.Errorf("failed to track group: %w", err)
	}

	if inserted, _ := res.RowsAffected(); inserted == 1 {
		op.Debug().Str("group", group).Msg("group tracking started")
	}

	return nil
}
