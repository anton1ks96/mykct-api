package service

import (
	"context"
	"fmt"

	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// ActiveAcademicGroups возвращает группы, у которых есть живые сессии. HTTP-маршрута
// у метода нет: он для других модулей монолита.
func (s *Service) ActiveAcademicGroups(ctx context.Context) ([]string, error) {
	op := logger.NewLogOp(ctx, log, "ActiveAcademicGroups")

	groups, err := s.sessions.ActiveAcademicGroups(ctx)
	if err != nil {
		op.Failed(err).Msg("failed to list active academic groups")
		return nil, fmt.Errorf("failed to list active academic groups: %w", err)
	}

	op.Completed().Int("groups", len(groups)).Msg("active academic groups listed")

	return groups, nil
}
