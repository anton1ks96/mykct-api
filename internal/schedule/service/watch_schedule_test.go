package service

import (
	"context"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
)

// publishedState - состояние недели, по которой расписание уже появилось.
func publishedState() *domain.WeekState {
	at := time.Date(2026, time.September, 19, 10, 0, 0, 0, time.UTC)

	return &domain.WeekState{
		Group:       "ИТ25-11",
		WeekStart:   "2026-09-21",
		WeekEnd:     "2026-09-27",
		Published:   true,
		EventsCount: 30,
		PublishedAt: &at,
	}
}

// trackingState - состояние недели, по которой расписания ещё нет.
func trackingState() *domain.WeekState {
	return &domain.WeekState{
		Group:     "ИТ25-11",
		WeekStart: "2026-09-21",
		WeekEnd:   "2026-09-27",
	}
}

// nextWeek - неделя, на которой ловится появление расписания.
func nextWeek() watchedWeek {
	return watchedWeek{start: "2026-09-21", end: "2026-09-27", detectPublish: true}
}

// currentWeek - текущая неделя, публикация на ней не детектится.
func currentWeek() watchedWeek {
	return watchedWeek{start: "2026-09-14", end: "2026-09-20"}
}

// TestDecideWeek закрепляет все переходы детекта: заведение, холодный старт,
// смену недели у знакомой группы и монотонность признака публикации.
func TestDecideWeek(t *testing.T) {
	tests := []struct {
		name        string
		state       *domain.WeekState
		week        watchedWeek
		groupKnown  bool
		eventsCount int
		want        weekAction
	}{
		{"новая группа, расписания нет", nil, nextWeek(), false, 0, actionCreateTracking},
		{"новая группа, расписание есть", nil, nextWeek(), false, 5, actionCreateBaseline},
		{"знакомая группа, расписания нет", nil, nextWeek(), true, 0, actionCreateTracking},
		{"знакомая группа, новая неделя с расписанием", nil, nextWeek(), true, 5, actionCreatePublished},
		{"ждём расписание, его всё нет", trackingState(), nextWeek(), true, 0, actionTouch},
		{"ждём расписание, оно появилось", trackingState(), nextWeek(), true, 5, actionPublish},
		{"опубликованная неделя опустела", publishedState(), nextWeek(), true, 0, actionTouch},
		{"опубликованная неделя на месте", publishedState(), nextWeek(), true, 30, actionTouch},
		{"текущая неделя у знакомой группы", nil, currentWeek(), true, 5, actionCreateBaseline},
		{"текущая неделя заполнилась", trackingState(), currentWeek(), true, 5, actionTouch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideWeek(tt.state, tt.week, tt.groupKnown, tt.eventsCount); got != tt.want {
				t.Errorf("decideWeek() = %v, want %v", got, tt.want)
			}
		})
	}
}

// watchConfig - настройки воркера с днями по умолчанию.
func watchConfig() config.ScheduleWatchConfig {
	return config.ScheduleWatchConfig{
		ActiveInterval: 15 * time.Minute,
		IdleInterval:   3 * time.Hour,
		ActiveDays: map[time.Weekday]bool{
			time.Friday: true, time.Saturday: true, time.Sunday: true,
		},
	}
}

// TestWatchInterval - опрос учащается в дни, когда выкладывают расписание.
func TestWatchInterval(t *testing.T) {
	cfg := watchConfig()

	tests := []struct {
		name string
		now  time.Time
		want time.Duration
	}{
		{"пятница", time.Date(2026, time.September, 18, 12, 0, 0, 0, collegetime.TZ()), cfg.ActiveInterval},
		{"суббота", time.Date(2026, time.September, 19, 12, 0, 0, 0, collegetime.TZ()), cfg.ActiveInterval},
		{"воскресенье", time.Date(2026, time.September, 20, 12, 0, 0, 0, collegetime.TZ()), cfg.ActiveInterval},
		{"среда", time.Date(2026, time.September, 16, 12, 0, 0, 0, collegetime.TZ()), cfg.IdleInterval},
		// По UTC ещё четверг, по времени колледжа уже пятница
		{"четверг по UTC", time.Date(2026, time.September, 17, 19, 30, 0, 0, time.UTC), cfg.ActiveInterval},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := watchInterval(tt.now, cfg); got != tt.want {
				t.Errorf("watchInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNextTickStopsAtWeekBoundary - пауза не переносится через полночь
// понедельника: там меняется ключ недели, и его надо завести сразу.
func TestNextTickStopsAtWeekBoundary(t *testing.T) {
	cfg := watchConfig()

	// Воскресенье 23:50: до смены недели десять минут, активный интервал - пятнадцать
	now := time.Date(2026, time.September, 20, 23, 50, 0, 0, collegetime.TZ())
	if got := nextTick(now, cfg); got != 10*time.Minute {
		t.Errorf("nextTick() = %v, want %v", got, 10*time.Minute)
	}

	// То же воскресенье на холостом интервале: до смены недели два часа, интервал - три
	idle := cfg
	idle.ActiveDays = map[time.Weekday]bool{}
	evening := time.Date(2026, time.September, 20, 22, 0, 0, 0, collegetime.TZ())
	if got := nextTick(evening, idle); got != 2*time.Hour {
		t.Errorf("nextTick() = %v, want %v", got, 2*time.Hour)
	}

	// Середина недели - обычный интервал, граница далеко
	wednesday := time.Date(2026, time.September, 16, 12, 0, 0, 0, collegetime.TZ())
	if got := nextTick(wednesday, cfg); got != cfg.IdleInterval {
		t.Errorf("nextTick() = %v, want %v", got, cfg.IdleInterval)
	}
}

// TestNextTickNeverBusyLoops - пауза всегда положительная, в том числе за
// секунды до смены недели.
func TestNextTickNeverBusyLoops(t *testing.T) {
	cfg := watchConfig()
	now := time.Date(2026, time.September, 20, 23, 59, 59, 0, collegetime.TZ())

	if got := nextTick(now, cfg); got < minTick {
		t.Errorf("nextTick() = %v, want at least %v", got, minTick)
	}
}

// fakeWeekStates - подставное хранилище состояний: запоминает, что записали.
type fakeWeekStates struct {
	created    *domain.WeekState
	touched    bool
	markCalled bool
	markResult bool
	createErr  error
	// baselineWon - чем ответит смена базового снимка: выиграна ли гонка
	baselineWon  bool
	baselinePrev string
	baselineNext string
}

func (f *fakeWeekStates) Find(context.Context, string, string) (*domain.WeekState, error) {
	return nil, domain.ErrWeekStateNotFound
}

func (f *fakeWeekStates) FindStatus(context.Context, string, string) (*domain.WeekState, error) {
	return nil, domain.ErrWeekStateNotFound
}

func (f *fakeWeekStates) Create(_ context.Context, state *domain.WeekState) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = state
	return nil
}

func (f *fakeWeekStates) MarkPublished(context.Context, string, string, int, time.Time) (bool, error) {
	f.markCalled = true
	return f.markResult, nil
}

func (f *fakeWeekStates) Touch(context.Context, string, string, int, time.Time) error {
	f.touched = true
	return nil
}

func (f *fakeWeekStates) ReplaceBaseline(
	_ context.Context,
	_, _, prevHash, nextHash string,
	_ []domain.Event,
) (bool, error) {
	f.baselinePrev, f.baselineNext = prevHash, nextHash
	return f.baselineWon, nil
}

// applyAction прогоняет решение через сервис с подставным хранилищем.
func applyAction(t *testing.T, states *fakeWeekStates, action weekAction) bool {
	t.Helper()

	svc := &Service{states: states}
	appeared, err := svc.applyWeekAction(context.Background(), action,
		"ИТ25-11", "2026-09-21", "2026-09-27", 5, time.Now().UTC())
	if err != nil {
		t.Fatalf("applyWeekAction() returned error: %v", err)
	}

	return appeared
}

// TestApplyWeekActionCreateBaseline - неделя, заполненная до начала слежения,
// заводится опубликованной, но без момента публикации: уведомлять не о чем.
func TestApplyWeekActionCreateBaseline(t *testing.T) {
	states := &fakeWeekStates{}

	if appeared := applyAction(t, states, actionCreateBaseline); appeared {
		t.Error("applyWeekAction() reported an appearance for a baseline week")
	}
	if states.created == nil {
		t.Fatal("applyWeekAction() did not create the state")
	}
	if !states.created.Published {
		t.Error("baseline state must be published")
	}
	if states.created.PublishedAt != nil {
		t.Errorf("baseline state must not carry PublishedAt, got %v", states.created.PublishedAt)
	}
}

// TestApplyWeekActionCreatePublished - неделя, появившаяся при нас, заводится
// с моментом публикации: именно её подхватит рассылка.
func TestApplyWeekActionCreatePublished(t *testing.T) {
	states := &fakeWeekStates{}

	if appeared := applyAction(t, states, actionCreatePublished); !appeared {
		t.Error("applyWeekAction() did not report the appearance")
	}
	if states.created == nil {
		t.Fatal("applyWeekAction() did not create the state")
	}
	if !states.created.Published || states.created.PublishedAt == nil {
		t.Errorf("published state must carry PublishedAt, got published=%v at=%v",
			states.created.Published, states.created.PublishedAt)
	}
	if states.created.NotifiedAt != nil {
		t.Error("fresh state must wait for notification, NotifiedAt is set")
	}
}

// TestApplyWeekActionCreateTracking - пустая неделя заводится неопубликованной.
func TestApplyWeekActionCreateTracking(t *testing.T) {
	states := &fakeWeekStates{}

	if appeared := applyAction(t, states, actionCreateTracking); appeared {
		t.Error("applyWeekAction() reported an appearance for an empty week")
	}
	if states.created == nil {
		t.Fatal("applyWeekAction() did not create the state")
	}
	if states.created.Published || states.created.PublishedAt != nil {
		t.Error("tracking state must be neither published nor stamped")
	}
}

// TestApplyWeekActionPublishPassesThroughResult - переход засчитывается только
// тому вызову, за которым его признало хранилище.
func TestApplyWeekActionPublishPassesThroughResult(t *testing.T) {
	won := &fakeWeekStates{markResult: true}
	if appeared := applyAction(t, won, actionPublish); !appeared {
		t.Error("applyWeekAction() lost the appearance reported by the storage")
	}

	lost := &fakeWeekStates{markResult: false}
	if appeared := applyAction(t, lost, actionPublish); appeared {
		t.Error("applyWeekAction() reported an appearance the storage did not confirm")
	}
	if !lost.markCalled {
		t.Error("applyWeekAction() did not go to MarkPublished")
	}
}

// TestApplyWeekActionTouch - обычный опрос только обновляет отметку.
func TestApplyWeekActionTouch(t *testing.T) {
	states := &fakeWeekStates{}

	if appeared := applyAction(t, states, actionTouch); appeared {
		t.Error("applyWeekAction() reported an appearance for a touch")
	}
	if !states.touched {
		t.Error("applyWeekAction() did not touch the state")
	}
	if states.created != nil {
		t.Error("applyWeekAction() created a state instead of touching it")
	}
}

// TestApplyWeekActionExistingStateIsNotAnAppearance - состояние завёл другой
// инстанс, он же и уведомит: второго уведомления быть не должно.
func TestApplyWeekActionExistingStateIsNotAnAppearance(t *testing.T) {
	states := &fakeWeekStates{createErr: domain.ErrWeekStateExists}

	if appeared := applyAction(t, states, actionCreatePublished); appeared {
		t.Error("applyWeekAction() reported an appearance for a state created elsewhere")
	}
}

// TestApplyWeekActionRejectsUnknown - новое действие не должно молча уходить в
// ветку создания состояния.
func TestApplyWeekActionRejectsUnknown(t *testing.T) {
	svc := &Service{states: &fakeWeekStates{}}

	_, err := svc.applyWeekAction(context.Background(), weekAction(42),
		"ИТ25-11", "2026-09-21", "2026-09-27", 5, time.Now().UTC())
	if err == nil {
		t.Error("applyWeekAction() accepted an unknown action")
	}
}

// TestWatchedWeeks - прогон ведёт текущую и следующую неделю, и появление
// расписания ловится только на следующей.
func TestWatchedWeeks(t *testing.T) {
	weeks := watchedWeeks(time.Date(2026, time.September, 16, 12, 0, 0, 0, collegetime.TZ()))

	if len(weeks) != 2 {
		t.Fatalf("watchedWeeks() = %d недель, want 2", len(weeks))
	}
	if weeks[0].start != "2026-09-14" || weeks[0].end != "2026-09-20" {
		t.Errorf("текущая неделя = %s..%s, want 2026-09-14..2026-09-20", weeks[0].start, weeks[0].end)
	}
	if weeks[1].start != "2026-09-21" || weeks[1].end != "2026-09-27" {
		t.Errorf("следующая неделя = %s..%s, want 2026-09-21..2026-09-27", weeks[1].start, weeks[1].end)
	}
	if weeks[0].detectPublish {
		t.Error("на текущей неделе публикацию ловить нечего")
	}
	if !weeks[1].detectPublish {
		t.Error("на следующей неделе публикация должна детектиться")
	}
}

// TestWatchedWeeksUsesCollegeTZ - воскресенье 19:30 UTC это уже понедельник в
// Екатеринбурге: недели сдвинулись.
func TestWatchedWeeksUsesCollegeTZ(t *testing.T) {
	weeks := watchedWeeks(time.Date(2026, time.September, 20, 19, 30, 0, 0, time.UTC))

	if weeks[0].start != "2026-09-21" || weeks[1].start != "2026-09-28" {
		t.Errorf("watchedWeeks() = %s и %s, want 2026-09-21 и 2026-09-28",
			weeks[0].start, weeks[1].start)
	}
}

// TestEventsWithin - двухнедельная пачка портала делится по датам ровно на две
// недели, а занятие без даты не попадает никуда.
func TestEventsWithin(t *testing.T) {
	events := []domain.Event{
		{ClID: "1", Day: "2026-09-14"},
		{ClID: "2", Day: "2026-09-20"},
		{ClID: "3", Day: "2026-09-21"},
		{ClID: "4", Day: "2026-09-27"},
		{ClID: "5", Day: ""},
	}

	current := eventsWithin(events, "2026-09-14", "2026-09-20")
	next := eventsWithin(events, "2026-09-21", "2026-09-27")

	if len(current) != 2 || current[0].ClID != "1" || current[1].ClID != "2" {
		t.Errorf("текущая неделя = %+v, want занятия 1 и 2", current)
	}
	if len(next) != 2 || next[0].ClID != "3" || next[1].ClID != "4" {
		t.Errorf("следующая неделя = %+v, want занятия 3 и 4", next)
	}
	if len(current)+len(next) == len(events) {
		t.Error("занятие без даты попало в неделю")
	}
}
