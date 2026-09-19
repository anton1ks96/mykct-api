package service

import (
	"context"
	"fmt"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// ActiveStudents возвращает студентов, у которых есть живая сессия. HTTP-маршрута
// у метода нет: он для других модулей монолита.
func (s *Service) ActiveStudents(ctx context.Context) ([]domain.ActiveStudent, error) {
	op := logger.NewLogOp(ctx, log, "ActiveStudents")

	students, err := s.sessions.ActiveStudents(ctx)
	if err != nil {
		op.Failed(err).Msg("failed to list active students")
		return nil, fmt.Errorf("failed to list active students: %w", err)
	}

	op.Completed().Int("students", len(students)).Msg("active students listed")

	return students, nil
}
