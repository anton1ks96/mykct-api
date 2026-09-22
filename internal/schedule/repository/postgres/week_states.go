package postgres

import (
	"context"
	"database/sql"
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
	_ repository.WeekStateRepository = (*WeekStateRepository)(nil)
	_ pgdb.ExpiryCleaner             = (*WeekStateRepository)(nil)
)

// Колонки состояния недели. Базовый снимок берётся отдельным списком: статусу
// недели занятия не нужны, а весят они десятки килобайт на строку.
const (
	weekStateColumns = `group_name, week_start, week_end, published, events_count,
		published_at, notified_at, last_checked_at, events_hash`
	weekStateFullColumns = weekStateColumns + `, events`
)

// weekStateRow - строка таблицы schedule_week_states. Времена публикации и
// рассылки - указатели: рассылка ищет неразосланное по явному NULL.
type weekStateRow struct {
	GroupName     string     `db:"group_name"`
	WeekStart     string     `db:"week_start"`
	WeekEnd       string     `db:"week_end"`
	Published     bool       `db:"published"`
	EventsCount   int        `db:"events_count"`
	PublishedAt   *time.Time `db:"published_at"`
	NotifiedAt    *time.Time `db:"notified_at"`
	LastCheckedAt time.Time  `db:"last_checked_at"`
	EventsHash    string     `db:"events_hash"`
	Events        []byte     `db:"events"`
}

// toDomain переводит строку состояния в доменную модель.
func (r *weekStateRow) toDomain() (*domain.WeekState, error) {
	events, err := unmarshalEvents(r.Events)
	if err != nil {
		return nil, err
	}

	return &domain.WeekState{
		Group:         r.GroupName,
		WeekStart:     r.WeekStart,
		WeekEnd:       r.WeekEnd,
		Published:     r.Published,
		EventsCount:   r.EventsCount,
		PublishedAt:   r.PublishedAt,
		NotifiedAt:    r.NotifiedAt,
		LastCheckedAt: r.LastCheckedAt,
		Events:        events,
		EventsHash:    r.EventsHash,
	}, nil
}

// WeekStateRepository хранит состояния недель расписания в PostgreSQL.
type WeekStateRepository struct {
	db  *sqlx.DB
	ttl time.Duration
}

// NewWeekStateRepository создаёт хранилище состояний недель поверх клиента PostgreSQL.
func NewWeekStateRepository(db *sqlx.DB, ttl time.Duration) *WeekStateRepository {
	return &WeekStateRepository{db: db, ttl: ttl}
}

// Find возвращает состояние недели группы вместе с базовым снимком.
func (r *WeekStateRepository) Find(ctx context.Context, group, weekStart string) (*domain.WeekState, error) {
	return r.find(ctx, group, weekStart, weekStateFullColumns)
}

// FindStatus возвращает состояние недели без базового снимка.
func (r *WeekStateRepository) FindStatus(ctx context.Context, group, weekStart string) (*domain.WeekState, error) {
	return r.find(ctx, group, weekStart, weekStateColumns)
}

// find читает состояние недели, отдавая выбор колонок вызывающему.
func (r *WeekStateRepository) find(
	ctx context.Context,
	group, weekStart, columns string,
) (*domain.WeekState, error) {
	op := logger.NewLogOp(ctx, log, "Find")

	query := `SELECT ` + columns + `
		FROM schedule_week_states
		WHERE group_name = $1 AND week_start = $2`

	var row weekStateRow
	if err := r.db.GetContext(ctx, &row, query, group, weekStart); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			op.Debug().Str("group", group).Str("week_start", weekStart).Msg("week state not found")
			return nil, domain.ErrWeekStateNotFound
		}
		op.Failed(err).Str("group", group).Msg("failed to find week state")
		return nil, fmt.Errorf("failed to find week state: %w", err)
	}

	state, err := row.toDomain()
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to decode week state")
		return nil, fmt.Errorf("failed to decode week state: %w", err)
	}

	return state, nil
}

// Create заводит состояние недели. Гонка инстансов упирается в первичный ключ
// и превращается в ErrWeekStateExists.
func (r *WeekStateRepository) Create(ctx context.Context, state *domain.WeekState) error {
	op := logger.NewLogOp(ctx, log, "Create")

	events, err := marshalEvents(state.Events)
	if err != nil {
		op.Failed(err).Str("group", state.Group).Msg("failed to encode week baseline")
		return fmt.Errorf("failed to encode week baseline: %w", err)
	}

	query := `
		INSERT INTO schedule_week_states (group_name, week_start, week_end, published, events_count,
			published_at, notified_at, last_checked_at, expires_at, events, events_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (group_name, week_start) DO NOTHING`
	res, err := r.db.ExecContext(ctx, query, state.Group, state.WeekStart, state.WeekEnd,
		state.Published, state.EventsCount, state.PublishedAt, state.NotifiedAt,
		state.LastCheckedAt, state.LastCheckedAt.Add(r.ttl), events, state.EventsHash)
	if err != nil {
		op.Failed(err).Str("group", state.Group).Msg("failed to create week state")
		return fmt.Errorf("failed to create week state: %w", err)
	}

	if created, _ := res.RowsAffected(); created == 0 {
		op.Debug().Str("group", state.Group).Msg("week state already created by another instance")
		return domain.ErrWeekStateExists
	}

	op.Debug().Str("group", state.Group).Str("week_start", state.WeekStart).
		Bool("published", state.Published).Msg("week state created")

	return nil
}

// MarkPublished фиксирует появление расписания. Условие published = FALSE стоит
// в запросе, поэтому при нескольких инстансах переход засчитывается одному.
func (r *WeekStateRepository) MarkPublished(
	ctx context.Context,
	group, weekStart string,
	eventsCount int,
	at time.Time,
) (bool, error) {
	op := logger.NewLogOp(ctx, log, "MarkPublished")

	query := `
		UPDATE schedule_week_states
		SET published = TRUE, published_at = $3, events_count = $4,
			last_checked_at = $3, expires_at = $5
		WHERE group_name = $1 AND week_start = $2 AND published = FALSE`
	res, err := r.db.ExecContext(ctx, query, group, weekStart, at, eventsCount, at.Add(r.ttl))
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to mark week published")
		return false, fmt.Errorf("failed to mark week published: %w", err)
	}

	updated, _ := res.RowsAffected()

	return updated == 1, nil
}

// Touch отмечает, что неделю опросили, не трогая признак публикации.
func (r *WeekStateRepository) Touch(
	ctx context.Context,
	group, weekStart string,
	eventsCount int,
	at time.Time,
) error {
	op := logger.NewLogOp(ctx, log, "Touch")

	query := `
		UPDATE schedule_week_states
		SET events_count = $3, last_checked_at = $4, expires_at = $5
		WHERE group_name = $1 AND week_start = $2`
	res, err := r.db.ExecContext(ctx, query, group, weekStart, eventsCount, at, at.Add(r.ttl))
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to touch week state")
		return fmt.Errorf("failed to touch week state: %w", err)
	}

	if updated, _ := res.RowsAffected(); updated == 0 {
		op.Warn().Str("group", group).Str("week_start", weekStart).
			Msg("week state disappeared before it was touched")
	}

	return nil
}

// ReplaceBaseline меняет базовый снимок недели, пока он тот же, от которого
// считали разницу. Условие по старому отпечатку стоит в запросе, поэтому при
// нескольких инстансах разницей владеет ровно один.
func (r *WeekStateRepository) ReplaceBaseline(
	ctx context.Context,
	group, weekStart, prevHash, nextHash string,
	events []domain.Event,
) (bool, error) {
	op := logger.NewLogOp(ctx, log, "ReplaceBaseline")

	baseline, err := marshalEvents(events)
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to encode week baseline")
		return false, fmt.Errorf("failed to encode week baseline: %w", err)
	}

	query := `
		UPDATE schedule_week_states
		SET events = $4, events_hash = $5
		WHERE group_name = $1 AND week_start = $2 AND events_hash = $3`
	res, err := r.db.ExecContext(ctx, query, group, weekStart, prevHash, baseline, nextHash)
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to replace week baseline")
		return false, fmt.Errorf("failed to replace week baseline: %w", err)
	}

	updated, _ := res.RowsAffected()

	return updated == 1, nil
}

// PendingPublished возвращает недели, опубликованные при нас не раньше since,
// о которых ещё не уведомляли. Неделям, заведённым baseline, published_at не
// ставится, поэтому под выборку они не попадают.
func (r *WeekStateRepository) PendingPublished(ctx context.Context, since time.Time) ([]*domain.WeekState, error) {
	op := logger.NewLogOp(ctx, log, "PendingPublished")

	query := `SELECT ` + weekStateColumns + `
		FROM schedule_week_states
		WHERE notified_at IS NULL AND published_at >= $1`

	var rows []weekStateRow
	if err := r.db.SelectContext(ctx, &rows, query, since); err != nil {
		op.Failed(err).Msg("failed to find pending published weeks")
		return nil, fmt.Errorf("failed to find pending published weeks: %w", err)
	}

	out := make([]*domain.WeekState, 0, len(rows))
	for i := range rows {
		state, err := rows[i].toDomain()
		if err != nil {
			op.Failed(err).Msg("failed to decode pending published weeks")
			return nil, fmt.Errorf("failed to decode pending published weeks: %w", err)
		}
		out = append(out, state)
	}

	return out, nil
}

// MarkNotified отмечает рассылку по неделе. Условие notified_at IS NULL стоит в
// запросе, поэтому при нескольких инстансах рассылкой владеет ровно один.
func (r *WeekStateRepository) MarkNotified(ctx context.Context, group, weekStart string, at time.Time) (bool, error) {
	query := `
		UPDATE schedule_week_states
		SET notified_at = $3
		WHERE group_name = $1 AND week_start = $2 AND notified_at IS NULL`
	res, err := r.db.ExecContext(ctx, query, group, weekStart, at)
	if err != nil {
		logger.NewLogOp(ctx, log, "MarkNotified").Failed(err).Str("group", group).
			Msg("failed to mark week notified")
		return false, fmt.Errorf("failed to mark week notified: %w", err)
	}

	updated, _ := res.RowsAffected()

	return updated == 1, nil
}

// DeleteExpired удаляет состояния недель, за которыми больше не следят.
func (r *WeekStateRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM schedule_week_states WHERE expires_at <= $1`, time.Now())
	if err != nil {
		logger.NewLogOp(ctx, log, "DeleteExpired").Failed(err).
			Msg("failed to delete expired week states")
		return 0, fmt.Errorf("failed to delete expired week states: %w", err)
	}

	deleted, _ := res.RowsAffected()

	return deleted, nil
}
