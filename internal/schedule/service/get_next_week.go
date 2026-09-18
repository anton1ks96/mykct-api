package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// GetNextWeekState возвращает состояние следующей недели для группы: появилось
// ли расписание и когда это заметил воркер.
func (s *Service) GetNextWeekState(ctx context.Context, group string) (*domain.WeekState, error) {
	op := logger.NewLogOp(ctx, log, "GetNextWeekState")

	weekStart, _ := collegetime.NextWeek(time.Now())

	state, err := s.states.FindStatus(ctx, group, weekStart)
	if err != nil {
		if errors.Is(err, domain.ErrWeekStateNotFound) {
			op.Debug().Str("group", group).Str("week_start", weekStart).
				Msg("next week is not tracked yet")
			return nil, domain.ErrWeekStateNotFound
		}
		op.Failed(err).Str("group", group).Msg("failed to read next week state")
		return nil, fmt.Errorf("failed to read next week state: %w", err)
	}

	op.Completed().Str("group", group).Bool("published", state.Published).
		Msg("next week state served")

	return state, nil
}
