// Package service реализует юзкейсы посещаемости.
package service

import (
	"context"

	"github.com/anton1ks96/mykct-api/internal/attendance/repository"
	authdomain "github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

var log = logger.ComponentLogger("attendance.service")

// ActiveStudents - поставщик студентов, пользующихся приложением. Узкий
// интерфейс потребителя: реализуется сервисом модуля аутентификации.
type ActiveStudents interface {
	// ActiveStudents возвращает студентов, у которых есть живая сессия.
	ActiveStudents(ctx context.Context) ([]authdomain.ActiveStudent, error)
}

// Service - юзкейсы посещаемости: отметки студента за период, серия посещений и
// анонимный рейтинг курса по этой серии.
type Service struct {
	portal      repository.Portal
	leaderboard repository.Leaderboard
	students    ActiveStudents
	cfg         config.LeaderboardConfig
	aliasSecret []byte
}

// NewService собирает сервис посещаемости из клиента портала, реестра рейтинга и
// поставщика активных студентов.
func NewService(
	portal repository.Portal,
	leaderboard repository.Leaderboard,
	students ActiveStudents,
	cfg config.LeaderboardConfig,
) *Service {
	return &Service{
		portal:      portal,
		leaderboard: leaderboard,
		students:    students,
		cfg:         cfg,
		aliasSecret: []byte(cfg.AliasSecret),
	}
}
