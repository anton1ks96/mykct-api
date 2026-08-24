package service

import (
	"context"

	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// GetAccessToken выдаёт новый access-токен по refresh-токену, не трогая саму
// сессию: профиль берётся из её снимка, каталог не опрашивается.
func (s *Service) GetAccessToken(ctx context.Context, refreshToken string) (*AccessTokenOutput, error) {
	op := logger.NewLogOp(ctx, log, "GetAccessToken")

	session, err := s.sessions.FindByTokenHash(ctx, hashToken(refreshToken))
	if err != nil {
		logFailure(op, err, "failed to find refresh session")
		return nil, err
	}

	user := session.User()

	accessToken, err := s.tokens.newAccessToken(user)
	if err != nil {
		op.Failed(err).Str("user_id", user.ID).Msg("failed to issue access token")
		return nil, err
	}

	op.Completed().Str("user_id", user.ID).Msg("access token issued")

	return &AccessTokenOutput{
		AccessToken: accessToken,
		ExpiresIn:   s.accessTokenTTL,
		User:        user,
	}, nil
}
