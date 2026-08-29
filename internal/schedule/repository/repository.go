// Package repository объявляет интерфейсы хранилищ модуля расписания.
package repository

import (
	"context"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// Portal - портал колледжа, первоисточник расписания.
type Portal interface {
	// FetchSchedule возвращает занятия группы за период без фильтрации по подгруппам.
	FetchSchedule(ctx context.Context, group, start, end string) ([]domain.Event, error)
	// FetchClassDetails возвращает произвольный JSON с деталями занятия.
	FetchClassDetails(ctx context.Context, clid string) (map[string]any, error)
}

// SnapshotRepository - кэш ответов портала, из которого расписание отдаётся,
// пока портал недоступен.
type SnapshotRepository interface {
	// SaveSchedule сохраняет снимок расписания, затирая предыдущий за тот же период.
	SaveSchedule(ctx context.Context, snapshot *domain.Snapshot) error
	// FindSchedule возвращает последний снимок расписания за период.
	FindSchedule(ctx context.Context, group, start, end string) (*domain.Snapshot, error)
	// SaveClassDetails сохраняет детали занятия, затирая предыдущие.
	SaveClassDetails(ctx context.Context, details *domain.ClassDetails) error
	// FindClassDetails возвращает последние сохранённые детали занятия.
	FindClassDetails(ctx context.Context, clid string) (*domain.ClassDetails, error)
}
