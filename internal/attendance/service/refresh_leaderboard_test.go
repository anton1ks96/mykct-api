package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
)

// TestDecideRefresh - главная страховка воркера: лежачий портал и пустой ответ
// не должны переписывать серию нулём. Портал отвечает на ошибку кодом 200 с
// текстом, поэтому отличить сбой от честно пустого списка нельзя.
func TestDecideRefresh(t *testing.T) {
	tests := []struct {
		name    string
		records []domain.Record
		err     error
		want    refreshAction
	}{
		{
			name: "портал недоступен",
			err:  domain.ErrPortalUnavailable,
			want: refreshSkip,
		},
		{
			name: "посторонняя ошибка",
			err:  errors.New("unexpected"),
			want: refreshSkip,
		},
		{
			name:    "ошибка вместе с отметками",
			records: []domain.Record{{Day: "2026-09-01", Status: domain.StatusPresent}},
			err:     domain.ErrPortalUnavailable,
			want:    refreshSkip,
		},
		{
			name:    "пустой срез",
			records: []domain.Record{},
			want:    refreshEmpty,
		},
		{
			name: "нулевой срез",
			want: refreshEmpty,
		},
		{
			name:    "есть отметки",
			records: []domain.Record{{Day: "2026-09-01", Status: domain.StatusPresent}},
			want:    refreshSave,
		},
		{
			name:    "есть только прогулы",
			records: []domain.Record{{Day: "2026-09-01", Status: 0}},
			want:    refreshSave,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideRefresh(tt.records, tt.err); got != tt.want {
				t.Errorf("decideRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNextRefreshTick - пауза между прогонами не опускается ниже нижней границы,
// иначе воркер с опечаткой в конфиге уходит в busy loop по порталу.
func TestNextRefreshTick(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{"обычный интервал", time.Hour, time.Hour},
		{"ровно граница", minRefreshTick, minRefreshTick},
		{"слишком часто", time.Second, minRefreshTick},
		{"нулевой интервал", 0, minRefreshTick},
		{"отрицательный интервал", -time.Hour, minRefreshTick},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextRefreshTick(tt.interval); got != tt.want {
				t.Errorf("nextRefreshTick(%v) = %v, want %v", tt.interval, got, tt.want)
			}
		})
	}
}

// TestRefreshSleepStopsOnCancel - отменённый контекст прерывает паузу, иначе
// shutdown ждал бы воркер до конца интервала.
func TestRefreshSleepStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if refreshSleep(ctx, time.Hour) {
		t.Error("refreshSleep() = true on a cancelled context, want false")
	}
	if refreshSleep(ctx, 0) {
		t.Error("refreshSleep() = true on a cancelled context with zero delay, want false")
	}
}
