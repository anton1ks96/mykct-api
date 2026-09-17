package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// portalFailureLimit - сколько отказов портала подряд терпит прогон, прежде чем
// бросить его целиком: лежачий портал лежит сразу для всех групп.
const portalFailureLimit = 3

// minTick - нижняя граница паузы между прогонами, страховка от busy loop.
const minTick = time.Minute

// startupJitter - разброс первого прогона, чтобы перезапуски не били по порталу разом.
const startupJitter = 30 * time.Second

// weekAction - что сделать с состоянием недели по результату опроса.
type weekAction int

const (
	// actionTouch - обновить отметку опроса, ничего не меняя по сути.
	actionTouch weekAction = iota
	// actionCreateTracking - завести состояние: расписания на неделю ещё нет.
	actionCreateTracking
	// actionCreateBaseline - завести уже опубликованным, но без повода уведомлять.
	actionCreateBaseline
	// actionCreatePublished - завести опубликованным: расписание появилось при нас.
	actionCreatePublished
	// actionPublish - зафиксировать появление расписания на заведённом состоянии.
	actionPublish
)

// decideWeek решает судьбу недели по её состоянию, знакомству с группой и числу
// занятий. Чистая функция: детект проверяется тестами без портала и MongoDB.
func decideWeek(state *domain.WeekState, groupKnown bool, eventsCount int) weekAction {
	if state == nil {
		switch {
		case eventsCount == 0:
			return actionCreateTracking
		case groupKnown:
			// Группу уже вели, а неделя новая: расписание по ней выложили при нас
			return actionCreatePublished
		default:
			// Группу видим впервые: неделя была заполнена до нас, уведомлять не о чем
			return actionCreateBaseline
		}
	}

	if !state.Published && eventsCount > 0 {
		return actionPublish
	}

	return actionTouch
}

// RunNextWeekWatcher опрашивает портал, пока не отменят контекст. Интервал
// плавающий, поэтому не тикер: пауза считается перед каждым ожиданием.
func (s *Service) RunNextWeekWatcher(ctx context.Context) {
	log.Info().Dur("active_interval", s.watch.ActiveInterval).
		Dur("idle_interval", s.watch.IdleInterval).Msg("next week schedule watcher started")

	// Первый прогон разносится случайной паузой: иначе перезапуск или раскатка
	// нескольких инстансов бьёт по порталу всеми группами разом
	if !sleep(ctx, time.Duration(rand.Int64N(int64(startupJitter)))) {
		log.Info().Msg("next week schedule watcher stopped")
		return
	}

	for {
		if err := s.CheckNextWeek(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Warn().Err(err).Msg("next week check failed")
		}

		if !sleep(ctx, nextTick(time.Now(), s.watch)) {
			log.Info().Msg("next week schedule watcher stopped")
			return
		}
	}
}

// CheckNextWeek опрашивает портал по всем живым группам и фиксирует появление
// расписания на следующую неделю. Один прогон, без цикла.
func (s *Service) CheckNextWeek(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "CheckNextWeek")

	groups, err := s.groups.ActiveAcademicGroups(ctx)
	if err != nil {
		return fmt.Errorf("failed to list groups to watch: %w", err)
	}
	if len(groups) == 0 {
		op.Debug().Msg("no active groups to watch")
		return nil
	}

	// Знакомство с группой берётся одним запросом на весь прогон: оно нужно
	// только чтобы отличить новую группу от смены недели у знакомой
	knownGroups, err := s.states.KnownGroups(ctx)
	if err != nil {
		return fmt.Errorf("failed to list known groups: %w", err)
	}
	known := make(map[string]bool, len(knownGroups))
	for _, group := range knownGroups {
		known[group] = true
	}

	weekStart, weekEnd := collegetime.NextWeek(time.Now())
	op.Started().Strs("groups", groups).Str("week_start", weekStart).
		Str("week_end", weekEnd).Msg("checking next week schedule")

	var published, failures int
	for i, group := range groups {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i > 0 && !sleep(ctx, s.watch.GroupDelay) {
			return ctx.Err()
		}

		appeared, err := s.checkGroupWeek(ctx, group, weekStart, weekEnd, known[group])
		switch {
		case err == nil:
			failures = 0
			if appeared {
				published++
			}
		case errors.Is(err, context.Canceled):
			return err
		case errors.Is(err, domain.ErrPortalUnavailable):
			failures++
			if failures >= portalFailureLimit {
				op.Warn().Int("failures", failures).
					Msg("college portal unavailable, aborting next week check")
				return nil
			}
		default:
			// Счётчик отказов портала не сбрасываем: посторонняя ошибка не
			// означает, что портал ожил
			op.Failed(err).Str("group", group).Msg("failed to check next week schedule")
		}
	}

	op.Completed().Int("groups", len(groups)).Int("published", published).
		Msg("next week schedule checked")

	return nil
}

// checkGroupWeek опрашивает портал по одной группе и приводит состояние недели в
// соответствие с ответом. true - расписание появилось именно на этом опросе.
func (s *Service) checkGroupWeek(
	ctx context.Context,
	group, weekStart, weekEnd string,
	groupKnown bool,
) (bool, error) {
	op := logger.NewLogOp(ctx, log, "checkGroupWeek")

	events, err := s.portal.FetchSchedule(ctx, group, weekStart, weekEnd)
	if err != nil {
		if errors.Is(err, domain.ErrPortalUnavailable) && ctx.Err() == nil {
			op.Warn().Err(err).Str("group", group).Msg("college portal did not answer")
		}
		return false, err
	}

	now := time.Now().UTC()

	// Снимок сохраняется только непустой: пустой заставил бы выдачу расписания
	// отдавать при упавшем портале пустой кэш вместо честного отказа
	if len(events) > 0 {
		s.saveSchedule(ctx, op, GetScheduleInput{Group: group, Start: weekStart, End: weekEnd}, events, now)
	}

	state, err := s.states.Find(ctx, group, weekStart)
	if err != nil && !errors.Is(err, domain.ErrWeekStateNotFound) {
		return false, err
	}
	if errors.Is(err, domain.ErrWeekStateNotFound) {
		state = nil
	}

	if state != nil && state.Published && len(events) == 0 {
		// Признак публикации не сбрасываем: пустой ответ бывает и при сбое
		// портала, а откат дал бы второе уведомление об одной неделе
		op.Warn().Str("group", group).Str("week_start", weekStart).
			Msg("published week came back empty, keeping the published flag")
	}

	return s.applyWeekAction(ctx, decideWeek(state, groupKnown, len(events)),
		group, weekStart, weekEnd, len(events), now)
}

// applyWeekAction записывает решение по неделе в хранилище.
func (s *Service) applyWeekAction(
	ctx context.Context,
	action weekAction,
	group, weekStart, weekEnd string,
	eventsCount int,
	now time.Time,
) (bool, error) {
	op := logger.NewLogOp(ctx, log, "applyWeekAction")

	var published bool

	switch action {
	case actionTouch:
		if err := s.states.Touch(ctx, group, weekStart, eventsCount, now); err != nil {
			return false, err
		}

	case actionPublish:
		marked, err := s.states.MarkPublished(ctx, group, weekStart, eventsCount, now)
		if err != nil {
			return false, err
		}
		published = marked

	case actionCreateTracking, actionCreateBaseline, actionCreatePublished:
		state := &domain.WeekState{
			Group:         group,
			WeekStart:     weekStart,
			WeekEnd:       weekEnd,
			Published:     action != actionCreateTracking,
			EventsCount:   eventsCount,
			LastCheckedAt: now,
		}
		if action == actionCreatePublished {
			state.PublishedAt = &now
		}

		if err := s.states.Create(ctx, state); err != nil {
			// Состояние завёл другой инстанс, он же и уведомит
			if errors.Is(err, domain.ErrWeekStateExists) {
				return false, nil
			}
			return false, err
		}
		published = action == actionCreatePublished

	default:
		return false, fmt.Errorf("unknown week action %d", action)
	}

	if published {
		op.Info().Str("group", group).Str("week_start", weekStart).
			Int("events", eventsCount).Msg("next week schedule published")
	}

	return published, nil
}

// watchInterval выбирает интервал опроса: в дни, когда выкладывают расписание,
// часто, в остальные - редко.
func watchInterval(now time.Time, cfg config.ScheduleWatchConfig) time.Duration {
	if cfg.ActiveDays[now.In(collegetime.TZ()).Weekday()] {
		return cfg.ActiveInterval
	}

	return cfg.IdleInterval
}

// nextTick - пауза до следующего прогона: интервал, обрезанный полуночью
// понедельника, где меняется ключ недели.
func nextTick(now time.Time, cfg config.ScheduleWatchConfig) time.Duration {
	interval := watchInterval(now, cfg)

	if untilMonday := collegetime.NextMonday(now).Sub(now); untilMonday < interval {
		interval = untilMonday
	}
	if interval < minTick {
		interval = minTick
	}

	return interval
}

// sleep ждёт паузу или отмену контекста. false - контекст отменён.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}

	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
