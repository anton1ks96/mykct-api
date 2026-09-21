// Package service реализует юзкейсы расписания.
package service

import (
	"context"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/schedule/repository"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

var log = logger.ComponentLogger("schedule.service")

// ActiveGroups - поставщик академических групп, за которыми имеет смысл следить.
// Узкий интерфейс потребителя: реализуется сервисом модуля аутентификации.
type ActiveGroups interface {
	// ActiveAcademicGroups возвращает группы, у которых есть живые сессии.
	ActiveAcademicGroups(ctx context.Context) ([]string, error)
}

// Service - юзкейсы расписания: выдача расписания группы и деталей занятия с
// откатом на сохранённый снимок, пока портал колледжа недоступен, и детект
// появления расписания на следующую неделю и его изменений внутри недели.
type Service struct {
	portal    repository.Portal
	snapshots repository.SnapshotRepository
	states    repository.WeekStateRepository
	tracked   repository.TrackedGroupRepository
	changes   repository.ChangeRepository
	groups    ActiveGroups
	notifier  Notifier // nil - рассылка выключена, изменения только копятся
	watch     config.ScheduleWatchConfig
}

// NewService собирает сервис расписания из портала, хранилищ и настроек воркера.
func NewService(
	portal repository.Portal,
	snapshots repository.SnapshotRepository,
	states repository.WeekStateRepository,
	tracked repository.TrackedGroupRepository,
	changes repository.ChangeRepository,
	groups ActiveGroups,
	notifier Notifier,
	watch config.ScheduleWatchConfig,
) *Service {
	return &Service{
		portal:    portal,
		snapshots: snapshots,
		states:    states,
		tracked:   tracked,
		changes:   changes,
		groups:    groups,
		notifier:  notifier,
		watch:     watch,
	}
}
