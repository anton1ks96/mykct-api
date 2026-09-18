package service

import (
	"context"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// detectWeekChanges сверяет свежий ответ портала с базовым снимком недели и
// записывает разницу. Возвращает число записанных изменений.
func (s *Service) detectWeekChanges(
	ctx context.Context,
	group, weekStart string,
	state *domain.WeekState,
	events []domain.Event,
	now time.Time,
) (int, error) {
	op := logger.NewLogOp(ctx, log, "detectWeekChanges")

	var (
		baseline []domain.Event
		prevHash string
	)
	if state != nil {
		baseline, prevHash = state.Events, state.EventsHash
	}

	// 1. Пустой ответ базис не трогает: сбой портала иначе превратится в
	// уведомление о том, что пропала вся неделя
	if len(events) == 0 {
		if len(baseline) > 0 {
			op.Warn().Str("group", group).Str("week_start", weekStart).
				Msg("portal returned an empty week, keeping the baseline")
		}
		return 0, nil
	}

	// 2. Быстрый путь: неделя не менялась, а так на большинстве опросов
	nextHash := eventsHash(events)
	if nextHash == prevHash {
		return 0, nil
	}

	// 3. При пустом базисе неделю только засеиваем, поэтому разницу не считаем:
	// иначе на первом прогоне каждой группы строится список из всех занятий,
	// который шагом ниже целиком выбрасывается
	var changes []domain.EventChange
	if len(baseline) > 0 {
		changes = diffEvents(baseline, events)
	}

	// 4. Базис меняет тот, кто от него же считал разницу. Проигравший инстанс
	// свою разницу выбрасывает: её увидел и записал победитель
	won, err := s.states.ReplaceBaseline(ctx, group, weekStart, prevHash, nextHash, events, now)
	if err != nil {
		return 0, err
	}
	if !won {
		op.Debug().Str("group", group).Str("week_start", weekStart).
			Msg("week baseline advanced by another instance")
		return 0, nil
	}

	// 5. Базиса не было - неделю просто засеяли. Появление расписания ловит
	// decideWeek, дублировать его нечем
	if len(baseline) == 0 {
		op.Debug().Str("group", group).Str("week_start", weekStart).
			Int("events", len(events)).Msg("week baseline seeded")
		return 0, nil
	}

	// 6. Прошедшее отбрасывается после смены базиса: иначе одно и то же
	// вчерашнее изменение находилось бы каждый прогон
	changes = dropPastChanges(changes, now)
	if len(changes) == 0 {
		return 0, nil
	}

	weekChanges := &domain.WeekChanges{
		Group:      group,
		WeekStart:  weekStart,
		DetectedAt: now,
		Changes:    changes,
	}

	if err := s.changes.Save(ctx, weekChanges); err != nil {
		// Базис уже сменился, повторить прогон нечем - изменение потеряно.
		// Логирует вызывающий: два op.Failed на один сбой дали бы два события
		return 0, fmt.Errorf("schedule changes detected but not saved: %w", err)
	}

	added, removed, changed, fields := changeSummary(changes)
	op.Info().Str("group", group).Str("week_start", weekStart).
		Int("added", added).Int("removed", removed).Int("changed", changed).
		Strs("fields", fields).Msg("schedule changed")

	return len(changes), nil
}

// changeSummary считает разницу по видам и собирает разошедшиеся поля. По этой
// строке лога видно, какие поля портал двигает сам по себе.
func changeSummary(changes []domain.EventChange) (added, removed, changed int, fields []string) {
	seen := make(map[string]bool)

	for _, change := range changes {
		switch change.Kind {
		case domain.ChangeAdded:
			added++
		case domain.ChangeRemoved:
			removed++
		case domain.ChangeChanged:
			changed++
		}

		for _, field := range change.Fields {
			if !seen[field] {
				seen[field] = true
				fields = append(fields, field)
			}
		}
	}

	return added, removed, changed, fields
}
