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
	ID         bson.ObjectID `bson:"_id,omitempty"`
	Group      string        `bson:"group"`
	WeekStart  string        `bson:"week_start"`
	DetectedAt time.Time     `bson:"detected_at"`
	Changes    []changeDoc   `bson:"changes"`
	NotifiedAt *time.Time    `bson:"notified_at"`
	ExpiresAt  time.Time     `bson:"expires_at"`
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

// changesToDomain переводит документы изменений обратно в доменную модель.
func changesToDomain(docs []changeDoc) []domain.EventChange {
	changes := make([]domain.EventChange, 0, len(docs))
	for _, doc := range docs {
		change := domain.EventChange{Kind: doc.Kind, Fields: doc.Fields}
		if doc.Before != nil {
			before := doc.Before.toDomain()
			change.Before = &before
		}
		if doc.After != nil {
			after := doc.After.toDomain()
			change.After = &after
		}
		changes = append(changes, change)
	}

	return changes
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

// Pending возвращает неразосланные разницы, замеченные не раньше since.
func (r *ChangeRepository) Pending(ctx context.Context, since time.Time) ([]*domain.WeekChanges, error) {
	op := logger.NewLogOp(ctx, log, "Pending")

	filter := bson.M{"notified_at": nil, "detected_at": bson.M{"$gte": since}}
	cur, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "detected_at", Value: 1}}))
	if err != nil {
		op.Failed(err).Msg("failed to find pending schedule changes")
		return nil, fmt.Errorf("failed to find pending schedule changes: %w", err)
	}

	var docs []weekChangesDoc
	if err := cur.All(ctx, &docs); err != nil {
		op.Failed(err).Msg("failed to decode pending schedule changes")
		return nil, fmt.Errorf("failed to decode pending schedule changes: %w", err)
	}

	out := make([]*domain.WeekChanges, 0, len(docs))
	for _, doc := range docs {
		out = append(out, &domain.WeekChanges{
			ID:         doc.ID.Hex(),
			Group:      doc.Group,
			WeekStart:  doc.WeekStart,
			DetectedAt: doc.DetectedAt,
			Changes:    changesToDomain(doc.Changes),
		})
	}

	return out, nil
}

// MarkNotified отмечает рассылку разницы. Условие notified_at:null стоит в
// фильтре, поэтому при нескольких инстансах рассылкой владеет ровно один.
func (r *ChangeRepository) MarkNotified(ctx context.Context, id string, at time.Time) (bool, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, fmt.Errorf("invalid schedule change id %q: %w", id, err)
	}

	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": oid, "notified_at": nil},
		bson.M{"$set": bson.M{"notified_at": at}})
	if err != nil {
		logger.NewLogOp(ctx, log, "MarkNotified").Failed(err).Msg("failed to mark schedule change notified")
		return false, fmt.Errorf("failed to mark schedule change notified: %w", err)
	}

	return res.ModifiedCount == 1, nil
}
