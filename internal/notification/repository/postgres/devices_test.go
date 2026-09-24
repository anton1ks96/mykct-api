package postgres

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/notification/domain"
	"github.com/anton1ks96/mykct-api/internal/platform/pgtest"
)

// device собирает устройство с заданной установкой, токеном и группой.
func device(deviceID, token, group string) *domain.Device {
	return &domain.Device{
		DeviceID:      deviceID,
		Token:         token,
		Platform:      domain.PlatformIOS,
		UserID:        "i24s0291",
		AcademicGroup: group,
		UpdatedAt:     time.Now().UTC(),
	}
}

// TestDeviceUpsertKeepsTokenUnique - тот же токен мог остаться за другой
// установкой после переустановки с восстановлением данных: запись должна
// остаться одна, иначе пуш придёт дважды.
func TestDeviceUpsertKeepsTokenUnique(t *testing.T) {
	repo := NewDeviceRepository(pgtest.DB(t, "notification_devices"))
	ctx := context.Background()

	if err := repo.Upsert(ctx, device("install-1", "fcm-1", "ИТ25-11")); err != nil {
		t.Fatalf("Upsert() returned error: %v", err)
	}
	if err := repo.Upsert(ctx, device("install-2", "fcm-1", "ИТ25-11")); err != nil {
		t.Fatalf("Upsert() of a moved token returned error: %v", err)
	}

	tokens, err := repo.TokensByGroup(ctx, "ИТ25-11")
	if err != nil {
		t.Fatalf("TokensByGroup() returned error: %v", err)
	}
	if !slices.Equal(tokens, []string{"fcm-1"}) {
		t.Errorf("TokensByGroup() = %v, want [fcm-1]", tokens)
	}

	var installs []string
	if err := repo.db.SelectContext(ctx, &installs,
		`SELECT device_id FROM notification_devices`); err != nil {
		t.Fatalf("failed to list devices: %v", err)
	}
	if !slices.Equal(installs, []string{"install-2"}) {
		t.Errorf("Upsert() left %v, want [install-2]", installs)
	}
}

// TestDeviceUpsertRebindsInstall - на той же установке сменился аккаунт или
// токен: запись переходит к новому владельцу, а не удваивается.
func TestDeviceUpsertRebindsInstall(t *testing.T) {
	repo := NewDeviceRepository(pgtest.DB(t, "notification_devices"))
	ctx := context.Background()

	if err := repo.Upsert(ctx, device("install-1", "fcm-1", "ИТ25-11")); err != nil {
		t.Fatalf("Upsert() returned error: %v", err)
	}

	moved := device("install-1", "fcm-2", "ИТ24-11")
	moved.UserID = "i24s0292"
	if err := repo.Upsert(ctx, moved); err != nil {
		t.Fatalf("Upsert() returned error: %v", err)
	}

	old, err := repo.TokensByGroup(ctx, "ИТ25-11")
	if err != nil {
		t.Fatalf("TokensByGroup() returned error: %v", err)
	}
	if len(old) != 0 {
		t.Errorf("TokensByGroup() of the previous group = %v, want empty", old)
	}

	tokens, err := repo.TokensByGroup(ctx, "ИТ24-11")
	if err != nil {
		t.Fatalf("TokensByGroup() returned error: %v", err)
	}
	if !slices.Equal(tokens, []string{"fcm-2"}) {
		t.Errorf("TokensByGroup() = %v, want [fcm-2]", tokens)
	}
}

// TestDeviceDeleteTokens - выход уносит только свои токены, а неизвестный
// токен и пустой список ошибкой не считаются.
func TestDeviceDeleteTokens(t *testing.T) {
	repo := NewDeviceRepository(pgtest.DB(t, "notification_devices"))
	ctx := context.Background()

	for i, token := range []string{"fcm-1", "fcm-2"} {
		if err := repo.Upsert(ctx, device(string(rune('a'+i)), token, "ИТ25-11")); err != nil {
			t.Fatalf("Upsert() returned error: %v", err)
		}
	}

	if err := repo.DeleteTokens(ctx, nil); err != nil {
		t.Errorf("DeleteTokens() of an empty list returned error: %v", err)
	}
	if err := repo.DeleteTokens(ctx, []string{"fcm-1", "fcm-unknown"}); err != nil {
		t.Fatalf("DeleteTokens() returned error: %v", err)
	}

	tokens, err := repo.TokensByGroup(ctx, "ИТ25-11")
	if err != nil {
		t.Fatalf("TokensByGroup() returned error: %v", err)
	}
	if !slices.Equal(tokens, []string{"fcm-2"}) {
		t.Errorf("DeleteTokens() left %v, want [fcm-2]", tokens)
	}
}
