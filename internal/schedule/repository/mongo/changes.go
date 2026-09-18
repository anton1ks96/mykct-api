package mongo

import (
	"context"
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
	_ repository.ChangeRepository = (*ChangeRepository)(nil)
	_ mongodb.IndexEnsurer        = (*ChangeRepository)(nil)
)

// changesCollection - коллекция изменений расписания.
const changesCollection = "schedule_changes"

// changeDoc - одно изменение занятия в документе.
type changeDoc struct {
	Kind   string    `bson:"kind"`
	Fields []string  `bson:"fields,omitempty"`
	Before *eventDoc `bson:"before,omitempty"`
	After  *eventDoc `bson:"after,omitempty"`
}

// weekChangesDoc - разница, замеченная одним прогоном воркера. Время рассылки -
// указатель без omitempty: рассылка ищет неразосланное по явному null.
type weekChangesDoc struct {
	Group      string      `bson:"group"`
	WeekStart  string      `bson:"week_start"`
	DetectedAt time.Time   `bson:"detected_at"`
	Changes    []changeDoc `bson:"changes"`
	NotifiedAt *time.Time  `bson:"notified_at"`
	ExpiresAt  time.Time   `bson:"expires_at"`
}

// changesFromDomain переводит изменения доменной модели в документы.
func changesFromDomain(changes []domain.EventChange) []changeDoc {
	docs := make([]changeDoc, 0, len(changes))
	for _, change := range changes {
		doc := changeDoc{Kind: change.Kind, Fields: change.Fields}
		if change.Before != nil {
			before := eventFromDomain(*change.Before)
			doc.Before = &before
		}
		if change.After != nil {
			after := eventFromDomain(*change.After)
			doc.After = &after
		}
		docs = append(docs, doc)
	}

	return docs
}

// ChangeRepository хранит изменения расписания в MongoDB.
type ChangeRepository struct {
	coll *mongo.Collection
	ttl  time.Duration
}

// NewChangeRepository создаёт хранилище изменений поверх клиента MongoDB.
func NewChangeRepository(client *mongo.Client, database string, ttl time.Duration) *ChangeRepository {
	return &ChangeRepository{
		coll: client.Database(database).Collection(changesCollection),
		ttl:  ttl,
	}
}

// EnsureIndexes заводит индексы коллекции изменений.
func (r *ChangeRepository) EnsureIndexes(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "EnsureIndexes")

	models := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "group", Value: 1},
				{Key: "week_start", Value: 1},
				{Key: "detected_at", Value: -1},
			},
			Options: options.Index().SetName("change_key_idx"),
		},
		{
			// Выборка того, о чём ещё не уведомляли, как у состояний недель
			Keys: bson.D{
				{Key: "notified_at", Value: 1},
				{Key: "detected_at", Value: 1},
			},
			Options: options.Index().SetName("change_notify_idx"),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("change_expires_at_ttl").SetExpireAfterSeconds(0),
		},
	}

	if _, err := r.coll.Indexes().CreateMany(ctx, models); err != nil {
		op.Failed(err).Msg("failed to create schedule change indexes")
		return fmt.Errorf("failed to create schedule change indexes: %w", err)
	}

	op.Completed().Msg("schedule change indexes ensured")

	return nil
}

// Save записывает замеченную разницу. Один документ - одно будущее уведомление.
func (r *ChangeRepository) Save(ctx context.Context, changes *domain.WeekChanges) error {
	op := logger.NewLogOp(ctx, log, "Save")

	doc := weekChangesDoc{
		Group:      changes.Group,
		WeekStart:  changes.WeekStart,
		DetectedAt: changes.DetectedAt,
		Changes:    changesFromDomain(changes.Changes),
		NotifiedAt: changes.NotifiedAt,
		ExpiresAt:  changes.DetectedAt.Add(r.ttl),
	}

	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		op.Failed(err).Str("group", changes.Group).Msg("failed to save schedule changes")
		return fmt.Errorf("failed to save schedule changes: %w", err)
	}

	op.Debug().Str("group", changes.Group).Str("week_start", changes.WeekStart).
		Int("changes", len(changes.Changes)).Msg("schedule changes saved")

	return nil
}
