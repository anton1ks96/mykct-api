// Package service реализует юзкейсы аутентификации.
package service

import (
	"errors"
	"time"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/internal/auth/repository"
	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

var log = logger.ComponentLogger("auth.service")

// Service - юзкейсы аутентификации: вход, выход, ротация и выдача токенов.
type Service struct {
	directory       repository.UserDirectory
	sessions        repository.SessionRepository
	tokens          *tokenManager
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	testMode        bool
}

// NewService собирает сервис аутентификации из хранилищ и настроек.
func NewService(
	directory repository.UserDirectory,
	sessions repository.SessionRepository,
	cfg config.AuthConfig,
) *Service {
	return &Service{
		directory:       directory,
		sessions:        sessions,
		tokens:          newTokenManager(cfg.JWTSigningKey, cfg.AccessTokenTTL),
		accessTokenTTL:  cfg.AccessTokenTTL,
		refreshTokenTTL: cfg.RefreshTokenTTL,
		testMode:        cfg.TestMode,
	}
}

// logFailure пишет ожидаемые отказы клиента в warn, а неожиданные - в error.
// Error-логи уезжают в Sentry, и протухшему токену там не место.
func logFailure(op *logger.LogOp, err error, msg string) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials),
		errors.Is(err, domain.ErrSessionNotFound),
		errors.Is(err, domain.ErrInvalidToken),
		errors.Is(err, domain.ErrRoleNotDetermined):
		op.Warn().Err(err).Msg(msg)
	default:
		op.Failed(err).Msg(msg)
	}
}
