package service

import (
	"context"

	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// SignOut отзывает refresh-сессию. Неизвестный или уже отозванный токен ошибкой
// не считается: клиенту в любом случае нужно очистить локальное состояние.
func (s *Service) SignOut(ctx context.Context, refreshToken string) error {
	op := logger.NewLogOp(ctx, log, "SignOut")

	if refreshToken == "" {
		return nil
	}

	if err := s.sessions.Revoke(ctx, hashToken(refreshToken)); err != nil {
		op.Failed(err).Msg("sign out failed")
		return err
	}

	op.Completed().Msg("signed out")

	return nil
}
