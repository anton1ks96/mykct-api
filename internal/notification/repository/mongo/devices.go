// Package mongo реализует хранилища модуля уведомлений поверх MongoDB.
package mongo

import (
	"context"
	"fmt"

	"github.com/anton1ks96/mykct-api/internal/notification/domain"
	"github.com/anton1ks96/mykct-api/pkg/database/mongodb"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var log = logger.ComponentLogger("notification.repository")

// Соответствие интерфейсу проверяется на этапе компиляции.
var _ mongodb.IndexEnsurer = (*DeviceRepository)(nil)

// devicesCollection - коллекция устройств с FCM-токенами.
const devicesCollection = "notification_devices"

// DeviceRepository хранит устройства пользователей в MongoDB.
type DeviceRepository struct {
	coll *mongo.Collection
}

// NewDeviceRepository создаёт хранилище устройств поверх клиента MongoDB.
func NewDeviceRepository(client *mongo.Client, database string) *DeviceRepository {
	return &DeviceRepository{coll: client.Database(database).Collection(devicesCollection)}
}

// EnsureIndexes заводит индексы коллекции устройств.
func (r *DeviceRepository) EnsureIndexes(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "EnsureIndexes")

	models := []mongo.IndexModel{
		{
			// Ключ записи - установка приложения, а не пара пользователь+устройство:
			// иначе после смены аккаунта на телефоне уведомления шли бы обоим
			Keys:    bson.D{{Key: "device_id", Value: 1}},
			Options: options.Index().SetName("device_id_uniq").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "token", Value: 1}},
			Options: options.Index().SetName("device_token_uniq").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "academic_group", Value: 1}},
			Options: options.Index().SetName("device_group_idx"),
		},
	}

	if _, err := r.coll.Indexes().CreateMany(ctx, models); err != nil {
		op.Failed(err).Msg("failed to create device indexes")
		return fmt.Errorf("failed to create device indexes: %w", err)
	}

	op.Completed().Msg("device indexes ensured")

	return nil
}

// Upsert заводит устройство или переписывает его токен и владельца.
func (r *DeviceRepository) Upsert(ctx context.Context, device *domain.Device) error {
	op := logger.NewLogOp(ctx, log, "Upsert")

	// Тот же токен мог остаться за другой установкой (переустановка с
	// восстановлением данных): держим его в одной записи, иначе пуш придёт дважды
	if _, err := r.coll.DeleteMany(ctx, bson.M{
		"token":     device.Token,
		"device_id": bson.M{"$ne": device.DeviceID},
	}); err != nil {
		op.Failed(err).Msg("failed to drop token duplicates")
		return fmt.Errorf("failed to drop token duplicates: %w", err)
	}

	update := bson.M{"$set": bson.M{
		"token":          device.Token,
		"platform":       device.Platform,
		"user_id":        device.UserID,
		"academic_group": device.AcademicGroup,
		"updated_at":     device.UpdatedAt,
	}}

	_, err := r.coll.UpdateOne(ctx, bson.M{"device_id": device.DeviceID}, update,
		options.UpdateOne().SetUpsert(true))
	if err != nil {
		op.Failed(err).Str("user_id", device.UserID).Msg("failed to upsert device")
		return fmt.Errorf("failed to upsert device: %w", err)
	}

	op.Debug().Str("user_id", device.UserID).Str("group", device.AcademicGroup).
		Str("platform", device.Platform).Msg("device registered")

	return nil
}

// DeleteTokens удаляет устройства с переданными токенами. Отсутствующий токен
// ошибкой не считается: клиенту при выходе всё равно, был ли он у нас.
func (r *DeviceRepository) DeleteTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}

	if _, err := r.coll.DeleteMany(ctx, bson.M{"token": bson.M{"$in": tokens}}); err != nil {
		logger.NewLogOp(ctx, log, "DeleteTokens").Failed(err).Msg("failed to delete devices")
		return fmt.Errorf("failed to delete devices: %w", err)
	}

	return nil
}

// TokensByGroup возвращает FCM-токены всех устройств группы.
func (r *DeviceRepository) TokensByGroup(ctx context.Context, group string) ([]string, error) {
	op := logger.NewLogOp(ctx, log, "TokensByGroup")

	cur, err := r.coll.Find(ctx, bson.M{"academic_group": group},
		options.Find().SetProjection(bson.M{"token": 1}))
	if err != nil {
		op.Failed(err).Str("group", group).Msg("failed to find group devices")
		return nil, fmt.Errorf("failed to find group devices: %w", err)
	}

	var docs []struct {
		Token string `bson:"token"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		op.Failed(err).Str("group", group).Msg("failed to decode group devices")
		return nil, fmt.Errorf("failed to decode group devices: %w", err)
	}

	tokens := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc.Token != "" {
			tokens = append(tokens, doc.Token)
		}
	}

	return tokens, nil
}
