package service

import (
	"context"
	"time"

	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// RefreshToken обменивает refresh-токен на новый, продлевая сессию. Подмена
// идёт одной операцией, поэтому старый токен перестаёт действовать ровно тогда,
// когда начинает действовать новый.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*RefreshTokenOutput, error) {
	op := logger.NewLogOp(ctx, log, "RefreshToken")

	newToken, err := newRefreshToken()
	if err != nil {
		op.Failed(err).Msg("failed to issue refresh token")
		return nil, err
	}

	session, err := s.sessions.Rotate(
		ctx,
		hashToken(refreshToken),
		hashToken(newToken),
		time.Now().Add(s.refreshTokenTTL),
	)
	if err != nil {
		logFailure(op, err, "failed to rotate refresh session")
		return nil, err
	}

	op.Completed().Str("user_id", session.UserID).Msg("refresh token rotated")

	return &RefreshTokenOutput{
		RefreshToken: newToken,
		ExpiresIn:    s.refreshTokenTTL,
	}, nil
}
