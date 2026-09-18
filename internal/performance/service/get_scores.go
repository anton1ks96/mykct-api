package service

import (
	"context"

	"github.com/anton1ks96/mykct-api/internal/performance/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// GetScores возвращает оценки студента по предмету за период.
func (s *Service) GetScores(ctx context.Context, input GetScoresInput) (domain.Scores, error) {
	op := logger.NewLogOp(ctx, log, "GetScores")

	scores, err := s.portal.FetchScores(ctx, input.Login, input.SuID, input.Start, input.End)
	if err != nil {
		return nil, portalError(op, input.Login, "fetch scores", err)
	}

	op.Completed().Str("login", input.Login).Str("suid", input.SuID).
		Int("subjects", len(scores)).Msg("scores served from portal")

	return scores, nil
}
