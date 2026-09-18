package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/internal/schedule/repository"
	"github.com/anton1ks96/mykct-api/pkg/database/mongodb"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Соответствие интерфейсам проверяется на этапе компиляции.
var (
	_ repository.WeekStateRepository = (*WeekStateRepository)(nil)
	_ mongodb.IndexEnsurer           = (*WeekStateRepository)(nil)
)

// weekStatesCollection - коллекция состояний недель.
const weekStatesCollection = "schedule_week_states"

// weekStateDoc - состояние недели в документе. Времена публикации и рассылки -
// указатели без omitempty: рассылка ищет их по явному null.
type weekStateDoc struct {
	Group         string     `bson:"group"`
	WeekStart     string     `bson:"week_start"`
	WeekEnd       string     `bson:"week_end"`
	Published     bool       `bson:"published"`
	EventsCount   int        `bson:"events_count"`
	PublishedAt   *time.Time `bson:"published_at"`
	NotifiedAt    *time.Time `bson:"notified_at"`
	LastCheckedAt time.Time  `bson:"last_checked_at"`
	ExpiresAt     time.Time  `bson:"expires_at"`
}

// toDomain переводит документ состояния в доменную модель.
func (d *weekStateDoc) toDomain() *domain.WeekState {
	return &domain.WeekState{
		Group:         d.Group,
		WeekStart:     d.WeekStart,
		WeekEnd:       d.WeekEnd,
		Published:     d.Published,
		EventsCount:   d.EventsCount,
		PublishedAt:   d.PublishedAt,
		NotifiedAt:    d.NotifiedAt,
		LastCheckedAt: d.LastCheckedAt,
	}
}

// WeekStateRepository хранит состояния недель расписания в MongoDB.
type WeekStateRepository struct {
	coll *mongo.Collection
	ttl  time.Duration
}

// NewWeekStateRepository создаёт хранилище состояний недель поверх клиента MongoDB.
func NewWeekStateRepository(client *mongo.Client, database string, ttl time.Duration) *WeekStateRepository {
	return &WeekStateRepository{
		coll: client.Database(database).Collection(weekStatesCollection),
		ttl:  ttl,
	}
}

// EnsureIndexes заводит индексы коллекции состояний недель.
func (r *WeekStateRepository) EnsureIndexes(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "EnsureIndexes")

	models := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "group", Value: 1},
				{Key: "week_start", Value: 1},
			},
			Options: options.Index().SetName("week_state_key_uniq").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("week_state_expires_at_ttl").SetExpireAfterSeconds(0),
		},
		{
			// Выборка того, о чём ещё не уведомляли. Равенство notified_at:null
			// ведущее: по нему рассылка отсекает основную массу документов
			Keys: bson.D{
				{Key: "notified_at", Value: 1},
				{Key: "published_at", Value: 1},
			},
			Options: options.Index().SetName("week_state_notify_idx"),
		},
	}

	if _, err := r.coll.Indexes().CreateMany(ctx, models); err != nil {
		op.Failed(err).Msg("failed to create week state indexes")
		return fmt.Errorf("failed to create week state indexes: %w", err)
	}

	op.Completed().Msg("week state indexes ensured")

	return nil
}

// Find возвращает состояние недели группы.
func (r *WeekStateRepository) Find(ctx context.Context, group, weekStart string) (*domain.WeekState, error) {
	op := logger.NewLogOp(ctx, log, "Find")

	var doc weekStateDoc
	err := r.coll.FindOne(ctx, weekStateFilter(group, weekStart)).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			op.Debug().Str("group", group).Str("week_start", weekStart).Msg("week state not found")
			return nil, domain.ErrWeekStateNotFound
		}
		op.Failed(err).Str("group", group).Msg("failed to find week state")
		return nil, fmt.Errorf("failed to find week state: %w", err)
	}

	return doc.toDomain(), nil
}

// Create заводит состояние недели. Гонка инстансов упирается в уникальный
// индекс и превращается в ErrWeekStateExists.
func (r *WeekStateRepository) Create(ctx context.Context, state *domain.WeekState) error {
	op := logger.NewLogOp(ctx, log, "Create")

	doc := weekStateDoc{
		Group:         state.Group,
		WeekStart:     state.WeekStart,
		WeekEnd:       state.WeekEnd,
		Published:     state.Published,
		EventsCount:   state.EventsCount,
		PublishedAt:   state.PublishedAt,
		NotifiedAt:    state.NotifiedAt,
		LastCheckedAt: state.LastCheckedAt,
		ExpiresAt:     state.LastCheckedAt.Add(r.ttl),
	}

	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			op.Debug().Str("group", state.Group).Msg("week state already created by another instance")
			return domain.ErrWeekStateExists
		}
		op.Failed(err).Str("group", state.Group).Msg("failed to create week state")
		return fmt.Errorf("failed to create week state: %w", err)
	}

	op.Debug().Str("group", state.Group).Str("week_start", state.WeekStart).
		Bool("published", state.Published).Msg("week state created")

	return nil
}

// MarkPublished фиксирует появление расписания. Условие published:false стоит в
// фильтре, поэтому при нескольких инстансах переход засчитывается ровно одному.
func (r *WeekStateRepository) MarkPublished(
	ctx context.Context,
	group, weekStart string,
	eventsCount int,
	at time.Time,
) (bool, error) {
	op := logger.NewLogOp(ctx, log, "MarkPublished")

	filter := weekStateFilter(group, weekStart)
	filter["published"] = false

	update := bson.M{"$set": bson.M{
		"published":       true,
		"published_at":    at,
		"events_count":    eventsCount,
		"last_checked_at": at,
		"expires_at":      at.Add(r.ttl),
	}}

	res, err := r.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to mark week published")
		return false, fmt.Errorf("failed to mark week published: %w", err)
	}

	return res.ModifiedCount == 1, nil
}

// Touch отмечает, что неделю опросили, не трогая признак публикации.
func (r *WeekStateRepository) Touch(
	ctx context.Context,
	group, weekStart string,
	eventsCount int,
	at time.Time,
) error {
	op := logger.NewLogOp(ctx, log, "Touch")

	update := bson.M{"$set": bson.M{
		"events_count":    eventsCount,
		"last_checked_at": at,
		"expires_at":      at.Add(r.ttl),
	}}

	res, err := r.coll.UpdateOne(ctx, weekStateFilter(group, weekStart), update)
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to touch week state")
		return fmt.Errorf("failed to touch week state: %w", err)
	}

	// Документ мог вымести TTL между чтением и записью: молча потерять отметку
	// нельзя, иначе статус недели пропадёт без следа в логах
	if res.MatchedCount == 0 {
		op.Warn().Str("group", group).Str("week_start", weekStart).
			Msg("week state disappeared before it was touched")
	}

	return nil
}

// weekStateFilter адресует состояние недели группой и понедельником.
func weekStateFilter(group, weekStart string) bson.M {
	return bson.M{"group": group, "week_start": weekStart}
}
