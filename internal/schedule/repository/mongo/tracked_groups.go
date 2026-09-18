package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/repository"
	"github.com/anton1ks96/mykct-api/pkg/database/mongodb"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Соответствие интерфейсам проверяется на этапе компиляции.
var (
	_ repository.TrackedGroupRepository = (*TrackedGroupRepository)(nil)
	_ mongodb.IndexEnsurer              = (*TrackedGroupRepository)(nil)
)

// trackedGroupsCollection - коллекция отметок о слежении за группами.
const trackedGroupsCollection = "schedule_tracked_groups"

// TrackedGroupRepository хранит отметки о слежении за группами в MongoDB.
// Без TTL: отметка переживает любые состояния недель.
type TrackedGroupRepository struct {
	coll *mongo.Collection
}

// NewTrackedGroupRepository создаёт хранилище отметок поверх клиента MongoDB.
func NewTrackedGroupRepository(client *mongo.Client, database string) *TrackedGroupRepository {
	return &TrackedGroupRepository{coll: client.Database(database).Collection(trackedGroupsCollection)}
}

// EnsureIndexes заводит индексы коллекции отметок.
func (r *TrackedGroupRepository) EnsureIndexes(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "EnsureIndexes")

	model := mongo.IndexModel{
		Keys:    bson.D{{Key: "group", Value: 1}},
		Options: options.Index().SetName("tracked_group_uniq").SetUnique(true),
	}

	if _, err := r.coll.Indexes().CreateOne(ctx, model); err != nil {
		op.Failed(err).Msg("failed to create tracked group indexes")
		return fmt.Errorf("failed to create tracked group indexes: %w", err)
	}

	op.Completed().Msg("tracked group indexes ensured")

	return nil
}

// All возвращает группы, за которыми уже следят.
func (r *TrackedGroupRepository) All(ctx context.Context) ([]string, error) {
	op := logger.NewLogOp(ctx, log, "All")

	var groups []string
	if err := r.coll.Distinct(ctx, "group", bson.M{}).Decode(&groups); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		op.Failed(err).Msg("failed to list tracked groups")
		return nil, fmt.Errorf("failed to list tracked groups: %w", err)
	}

	op.Debug().Int("groups", len(groups)).Msg("tracked groups listed")

	return groups, nil
}

// Track отмечает начало слежения за группой. Повторный вызов момент не сдвигает.
func (r *TrackedGroupRepository) Track(ctx context.Context, group string, at time.Time) error {
	op := logger.NewLogOp(ctx, log, "Track")

	update := bson.M{"$setOnInsert": bson.M{"group": group, "tracked_since": at}}

	res, err := r.coll.UpdateOne(ctx, bson.M{"group": group}, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to track group")
		return fmt.Errorf("failed to track group: %w", err)
	}

	if res.UpsertedCount == 1 {
		op.Debug().Str("group", group).Msg("group tracking started")
	}

	return nil
}
