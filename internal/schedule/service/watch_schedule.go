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

// maxChangeLag - интервал опроса, выше которого детект изменений внутри недели
// отстаёт настолько, что уведомление приходит уже после занятия.
const maxChangeLag = time.Hour

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

// decideWeek решает судьбу недели по её состоянию, флагу детекта публикации,
// знакомству с группой и числу занятий.
func decideWeek(state *domain.WeekState, week watchedWeek, groupKnown bool, eventsCount int) weekAction {
	// Появление расписания - событие следующей недели. На текущей оно не имеет
	// смысла, поэтому она всегда заводится baseline и уведомлений не даёт
	publish := week.detectPublish && groupKnown

	if state == nil {
		switch {
		case eventsCount == 0:
			return actionCreateTracking
		case publish:
			// Группу уже вели, а неделя новая: расписание по ней выложили при нас
			return actionCreatePublished
		default:
			// Группу видим впервые: неделя была заполнена до нас, уведомлять не о чем
			return actionCreateBaseline
		}
	}

	if week.detectPublish && !state.Published && eventsCount > 0 {
		return actionPublish
	}

	return actionTouch
}

// RunScheduleWatcher опрашивает портал, пока не отменят контекст. Интервал
// плавающий, поэтому не тикер: пауза считается перед каждым ожиданием.
func (s *Service) RunScheduleWatcher(ctx context.Context) {
	log.Info().Dur("active_interval", s.watch.ActiveInterval).
		Dur("idle_interval", s.watch.IdleInterval).Msg("schedule watcher started")

	// Дефолт поменялся на 30m, но окружение, собранное по старому примеру,
	// пришло со своим значением и молча оставит детект изменений отставать
	if s.watch.IdleInterval > maxChangeLag {
		log.Warn().Dur("idle_interval", s.watch.IdleInterval).Dur("advised", maxChangeLag).
			Msg("SCHEDULE_WATCH_IDLE_INTERVAL is too rare for schedule change detection")
	}

	// Первый прогон разносится случайной паузой: иначе перезапуск или раскатка
	// нескольких инстансов бьёт по порталу всеми группами разом
	if !sleep(ctx, time.Duration(rand.Int64N(int64(startupJitter)))) {
		log.Info().Msg("schedule watcher stopped")
		return
	}

	for {
		if err := s.CheckWeeks(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Warn().Err(err).Msg("schedule check failed")
		}

		if !sleep(ctx, nextTick(time.Now(), s.watch)) {
			log.Info().Msg("schedule watcher stopped")
			return
		}
	}
}

// watchedWeek - неделя, за которой следит воркер.
type watchedWeek struct {
	start, end string
	// detectPublish - ловить ли на этой неделе появление расписания. Для
	// текущей недели такого события нет: она либо заполнена, либо каникулы
	detectPublish bool
}

// watchedWeeks - недели одного прогона: текущая и следующая. Чистая функция.
func watchedWeeks(now time.Time) []watchedWeek {
	currentStart, currentEnd := collegetime.CurrentWeek(now)
	nextStart, nextEnd := collegetime.NextWeek(now)

	return []watchedWeek{
		{start: currentStart, end: currentEnd},
		{start: nextStart, end: nextEnd, detectPublish: true},
	}
}

// eventsWithin оставляет занятия, попавшие в период. Ответ портала за две
// недели делится по датам, а не по порядку: порядок портала нам ничего не
// гарантирует, а даты есть у каждого занятия.
func eventsWithin(events []domain.Event, start, end string) []domain.Event {
	out := make([]domain.Event, 0, len(events))
	for _, event := range events {
		if event.Day >= start && event.Day <= end {
			out = append(out, event)
		}
	}

	return out
}

// CheckWeeks опрашивает портал по всем живым группам и приводит состояния
// отслеживаемых недель в соответствие с ответом. Один прогон, без цикла.
func (s *Service) CheckWeeks(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "CheckWeeks")

	groups, err := s.groups.ActiveAcademicGroups(ctx)
	if err != nil {
		return fmt.Errorf("failed to list groups to watch: %w", err)
	}
	if len(groups) == 0 {
		op.Debug().Msg("no active groups to watch")
		return nil
	}

	// Отметки слежения берутся одним запросом на весь прогон: они нужны только
	// чтобы отличить новую группу от смены недели у знакомой
	trackedGroups, err := s.tracked.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tracked groups: %w", err)
	}
	tracked := make(map[string]bool, len(trackedGroups))
	for _, group := range trackedGroups {
		tracked[group] = true
	}

	// Момент берётся один на прогон: иначе прогон, начатый под полночь
	// понедельника, обработал бы часть групп уже в другой паре недель
	now := time.Now().UTC()

	weeks := watchedWeeks(now)
	op.Started().Strs("groups", groups).Str("from", weeks[0].start).
		Str("to", weeks[len(weeks)-1].end).Msg("checking schedule weeks")

	var published, changed, failures int
	for i, group := range groups {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i > 0 && !sleep(ctx, s.watch.GroupDelay) {
			return ctx.Err()
		}

		// Обе недели берутся одним запросом: на двухнедельный период портал
		// отвечает ровно тем же, чем на два недельных
		events, err := s.portal.FetchSchedule(ctx, group, weeks[0].start, weeks[len(weeks)-1].end)
		if err != nil {
			switch {
			case errors.Is(err, context.Canceled):
				return err
			case errors.Is(err, domain.ErrPortalUnavailable):
				failures++
				if ctx.Err() == nil {
					op.Warn().Err(err).Str("group", group).Msg("college portal did not answer")
				}
				if failures >= portalFailureLimit {
					op.Warn().Int("failures", failures).
						Msg("college portal unavailable, aborting schedule check")
					return nil
				}
			default:
				// Счётчик отказов портала не сбрасываем: посторонняя ошибка не
				// означает, что портал ожил
				op.Failed(err).Str("group", group).Msg("failed to fetch schedule")
			}
			continue
		}
		failures = 0

		var placed int
		weeksDone := true
		for _, week := range weeks {
			weekEvents := eventsWithin(events, week.start, week.end)
			placed += len(weekEvents)

			appeared, weekChanges, err := s.checkGroupWeek(ctx, group, week, weekEvents,
				tracked[group], now)

			// Счётчики снимаются до разбора ошибки: публикация уже записана в
			// хранилище, даже если следом упал детект изменений
			if appeared {
				published++
			}
			changed += weekChanges

			if err != nil {
				op.Failed(err).Str("group", group).Str("week_start", week.start).
					Msg("failed to check schedule week")
				weeksDone = false
			}
		}

		// Занятие без даты не попадает ни в одну неделю и молча теряется
		if placed != len(events) {
			op.Warn().Str("group", group).Int("dropped", len(events)-placed).
				Msg("portal returned events outside the watched weeks")
		}

		// Отметка ставится только когда решения по неделям записаны: иначе
		// группа станет знакомой без состояний, и следующий прогон объявит уже
		// выложенную неделю только что появившейся
		if !weeksDone {
			continue
		}
		if err := s.tracked.Track(ctx, group, now); err != nil {
			op.Failed(err).Str("group", group).Msg("failed to mark group as tracked")
		}
	}

	op.Completed().Int("groups", len(groups)).Int("published", published).
		Int("changed", changed).Msg("schedule weeks checked")

	return nil
}

// checkGroupWeek приводит состояние одной недели группы в соответствие с
// ответом портала: ловит появление расписания и разницу с прошлым опросом.
// Первое значение - расписание появилось именно на этом опросе, второе - сколько
// изменений записано.
func (s *Service) checkGroupWeek(
	ctx context.Context,
	group string,
	week watchedWeek,
	events []domain.Event,
	groupKnown bool,
	now time.Time,
) (bool, int, error) {
	op := logger.NewLogOp(ctx, log, "checkGroupWeek")

	// Снимок сохраняется только непустой: пустой заставил бы выдачу расписания
	// отдавать при упавшем портале пустой кэш вместо честного отказа
	if len(events) > 0 {
		s.saveSchedule(ctx, op, GetScheduleInput{Group: group, Start: week.start, End: week.end}, events, now)
	}

	state, err := s.states.Find(ctx, group, week.start)
	if err != nil && !errors.Is(err, domain.ErrWeekStateNotFound) {
		return false, 0, err
	}
	if errors.Is(err, domain.ErrWeekStateNotFound) {
		state = nil
	}

	if state != nil && state.Published && len(events) == 0 {
		// Признак публикации не сбрасываем: пустой ответ бывает и при сбое
		// портала, а откат дал бы второе уведомление об одной неделе
		op.Warn().Str("group", group).Str("week_start", week.start).
			Msg("published week came back empty, keeping the published flag")
	}

	appeared, err := s.applyWeekAction(ctx, decideWeek(state, week, groupKnown, len(events)),
		group, week.start, week.end, len(events), now)
	if err != nil {
		return false, 0, err
	}

	// Разница считается после решения о публикации: состояние недели к этому
	// моменту уже заведено, и базису есть где лежать
	changed, err := s.detectWeekChanges(ctx, group, week.start, state, events, now)
	if err != nil {
		return appeared, 0, err
	}

	return appeared, changed, nil
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
			Int("events", eventsCount).Msg("week schedule published")
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
