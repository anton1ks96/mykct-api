// Package service реализует юзкейсы успеваемости.
package service

import (
	"errors"
	"fmt"

	"github.com/anton1ks96/mykct-api/internal/performance/domain"
	"github.com/anton1ks96/mykct-api/internal/performance/repository"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

var log = logger.ComponentLogger("performance.service")

// Service - юзкейсы успеваемости: предметы студента и оценки по предмету.
type Service struct {
	portal repository.Portal
}

// NewService собирает сервис успеваемости из клиента портала.
func NewService(portal repository.Portal) *Service {
	return &Service{portal: portal}
}

// portalError разбирает сбой портала. Недоступный портал - ожидаемая деградация,
// а не сбой сервиса, поэтому warn: error-логи уезжают в Sentry.
func portalError(op *logger.LogOp, login, action string, err error) error {
	if errors.Is(err, domain.ErrPortalUnavailable) {
		op.Warn().Err(err).Str("login", login).Msg("college portal unavailable")
		return err
	}

	op.Failed(err).Str("login", login).Msgf("failed to %s", action)
	return fmt.Errorf("failed to %s: %w", action, err)
}
