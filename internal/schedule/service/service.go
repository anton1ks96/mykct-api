// Package service реализует юзкейсы расписания.
package service

import (
	"github.com/anton1ks96/mykct-api/internal/schedule/repository"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

var log = logger.ComponentLogger("schedule.service")

// Service - юзкейсы расписания: выдача расписания группы и деталей занятия с
// откатом на сохранённый снимок, пока портал колледжа недоступен.
type Service struct {
	portal    repository.Portal
	snapshots repository.SnapshotRepository
}

// NewService собирает сервис расписания из портала и кэша снимков.
func NewService(portal repository.Portal, snapshots repository.SnapshotRepository) *Service {
	return &Service{portal: portal, snapshots: snapshots}
}
