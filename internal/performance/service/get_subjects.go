package service

import (
	"context"

	"github.com/anton1ks96/mykct-api/internal/performance/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// GetSubjects возвращает предметы студента в текущем семестре.
func (s *Service) GetSubjects(ctx context.Context, login string) ([]domain.Subject, error) {
	op := logger.NewLogOp(ctx, log, "GetSubjects")

	subjects, err := s.portal.FetchSubjects(ctx, login)
	if err != nil {
		return nil, portalError(op, login, "fetch subjects", err)
	}

	op.Completed().Str("login", login).Int("subjects", len(subjects)).
		Msg("subjects served from portal")

	return subjects, nil
}
