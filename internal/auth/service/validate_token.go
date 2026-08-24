package service

import (
	"context"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// ValidateAccessToken проверяет access-токен и возвращает профиль из его claims,
// не обращаясь к базе. HTTP-эндпоинта у метода нет: он для middleware и для
// других модулей монолита.
func (s *Service) ValidateAccessToken(ctx context.Context, accessToken string) (*domain.UserExtended, error) {
	op := logger.NewLogOp(ctx, log, "ValidateAccessToken")

	if accessToken == "" {
		return nil, domain.ErrInvalidToken
	}

	user, err := s.tokens.parseAccessToken(accessToken)
	if err != nil {
		op.Debug().Err(err).Msg("access token rejected")
		return nil, err
	}

	return user, nil
}
