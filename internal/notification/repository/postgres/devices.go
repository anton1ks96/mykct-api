// Package postgres реализует хранилища модуля уведомлений поверх PostgreSQL.
package postgres

import (
	"context"
	"fmt"

	"github.com/anton1ks96/mykct-api/internal/notification/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/jmoiron/sqlx"
)

var log = logger.ComponentLogger("notification.repository")

// DeviceRepository хранит устройства пользователей в PostgreSQL.
type DeviceRepository struct {
	db *sqlx.DB
}

// NewDeviceRepository создаёт хранилище устройств поверх клиента PostgreSQL.
func NewDeviceRepository(db *sqlx.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

// Upsert заводит устройство или переписывает его токен и владельца.
func (r *DeviceRepository) Upsert(ctx context.Context, device *domain.Device) error {
	op := logger.NewLogOp(ctx, log, "Upsert")

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		op.Failed(err).Msg("failed to begin transaction")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`DELETE FROM notification_devices WHERE token = $1 AND device_id <> $2`,
		device.Token, device.DeviceID)
	if err != nil {
		op.Failed(err).Msg("failed to drop token duplicates")
		return fmt.Errorf("failed to drop token duplicates: %w", err)
	}

	query := `
		INSERT INTO notification_devices (device_id, token, platform, user_id, academic_group, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (device_id) DO UPDATE SET
			token = EXCLUDED.token,
			platform = EXCLUDED.platform,
			user_id = EXCLUDED.user_id,
			academic_group = EXCLUDED.academic_group,
			updated_at = EXCLUDED.updated_at`
	_, err = tx.ExecContext(ctx, query, device.DeviceID, device.Token, device.Platform,
		device.UserID, device.AcademicGroup, device.UpdatedAt)
	if err != nil {
		op.Failed(err).Str("user_id", device.UserID).Msg("failed to upsert device")
		return fmt.Errorf("failed to upsert device: %w", err)
	}

	if err := tx.Commit(); err != nil {
		op.Failed(err).Msg("failed to commit device upsert")
		return fmt.Errorf("failed to commit device upsert: %w", err)
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

	query, args, err := sqlx.In(`DELETE FROM notification_devices WHERE token IN (?)`, tokens)
	if err != nil {
		return fmt.Errorf("failed to build device delete query: %w", err)
	}

	if _, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
		logger.NewLogOp(ctx, log, "DeleteTokens").Failed(err).Msg("failed to delete devices")
		return fmt.Errorf("failed to delete devices: %w", err)
	}

	return nil
}

// TokensByGroup возвращает FCM-токены всех устройств группы.
func (r *DeviceRepository) TokensByGroup(ctx context.Context, group string) ([]string, error) {
	op := logger.NewLogOp(ctx, log, "TokensByGroup")

	query := `SELECT token FROM notification_devices WHERE academic_group = $1 AND token <> ''`

	var tokens []string
	if err := r.db.SelectContext(ctx, &tokens, query, group); err != nil {
		op.Failed(err).Str("group", group).Msg("failed to find group devices")
		return nil, fmt.Errorf("failed to find group devices: %w", err)
	}

	return tokens, nil
}
