// Package service реализует юзкейсы посещаемости.
package service

import (
	"github.com/anton1ks96/mykct-api/internal/attendance/repository"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

var log = logger.ComponentLogger("attendance.service")

// Service - юзкейсы посещаемости: отметки студента за период и серия посещений.
type Service struct {
	portal repository.Portal
}

// NewService собирает сервис посещаемости из клиента портала.
func NewService(portal repository.Portal) *Service {
	return &Service{portal: portal}
}
