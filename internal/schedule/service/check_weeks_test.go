package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
)

// fakePortal - подставной портал: отдаёт заготовленный ответ или отказ.
type fakePortal struct {
	events    []domain.Event
	err       error
	calls     int
	lastStart string
	lastEnd   string
}

func (f *fakePortal) FetchSchedule(_ context.Context, _, start, end string) ([]domain.Event, error) {
	f.calls++
	f.lastStart, f.lastEnd = start, end

	if f.err != nil {
		return nil, f.err
	}

	return f.events, nil
}

func (f *fakePortal) FetchClassDetails(context.Context, string) (map[string]any, error) {
	return nil, domain.ErrClassDetailsUnavailable
}

// fakeGroups - живые группы колледжа.
type fakeGroups struct {
	groups []string
}

func (f *fakeGroups) ActiveAcademicGroups(context.Context) ([]string, error) {
	return f.groups, nil
}

// fakeTracked - отметки о слежении: запоминает, кого отметили.
type fakeTracked struct {
	known   []string
	tracked []string
}

func (f *fakeTracked) All(context.Context) ([]string, error) {
	return f.known, nil
}

func (f *fakeTracked) Track(_ context.Context, group string, _ time.Time) error {
	f.tracked = append(f.tracked, group)
	return nil
}

// fakeSnapshots - хранилище снимков, считает сохранённые недели.
type fakeSnapshots struct {
	saved []*domain.Snapshot
}

func (f *fakeSnapshots) SaveSchedule(_ context.Context, snapshot *domain.Snapshot) error {
	f.saved = append(f.saved, snapshot)
	return nil
}

func (f *fakeSnapshots) FindSchedule(context.Context, string, string, string) (*domain.Snapshot, error) {
	return nil, domain.ErrScheduleUnavailable
}

func (f *fakeSnapshots) SaveClassDetails(context.Context, *domain.ClassDetails) error {
	return nil
}

func (f *fakeSnapshots) FindClassDetails(context.Context, string) (*domain.ClassDetails, error) {
	return nil, domain.ErrClassDetailsUnavailable
}

// twoWeekAnswer - ответ портала за обе отслеживаемые недели по сегодняшнему дню.
func twoWeekAnswer() (events []domain.Event, currentStart, nextStart string) {
	currentStart, _ = collegetime.CurrentWeek(time.Now())
	nextStart, _ = collegetime.NextWeek(time.Now())

	return []domain.Event{
		lesson("1", currentStart, "09:00", "10:30", "404", "Математика"),
		lesson("2", currentStart, "10:45", "12:15", "3-2", "АиСД"),
		lesson("3", nextStart, "09:00", "10:30", "405", "История"),
	}, currentStart, nextStart
}

// watcherService собирает сервис воркера на подставных хранилищах.
func watcherService(portal *fakePortal, states *fakeWeekStates, tracked *fakeTracked) *Service {
	return &Service{
		portal:    portal,
		snapshots: &fakeSnapshots{},
		states:    states,
		tracked:   tracked,
		changes:   &fakeChanges{},
		groups:    &fakeGroups{groups: []string{"ИТ25-11"}},
		watch:     config.ScheduleWatchConfig{ActiveDays: map[time.Weekday]bool{}},
	}
}

// TestCheckWeeksSplitsPortalAnswer - двухнедельная пачка портала запрашивается
// одним походом и раскладывается по неделям.
func TestCheckWeeksSplitsPortalAnswer(t *testing.T) {
	events, currentStart, nextStart := twoWeekAnswer()

	portal := &fakePortal{events: events}
	states := &fakeWeekStates{}
	snapshots := &fakeSnapshots{}

	svc := watcherService(portal, states, &fakeTracked{})
	svc.snapshots = snapshots

	if err := svc.CheckWeeks(context.Background()); err != nil {
		t.Fatalf("CheckWeeks() error = %v", err)
	}

	if portal.calls != 1 {
		t.Errorf("походов в портал = %d, want 1", portal.calls)
	}
	if portal.lastStart != currentStart {
		t.Errorf("запрошено с %s, want %s", portal.lastStart, currentStart)
	}

	if len(states.createdAll) != 2 {
		t.Fatalf("заведено состояний = %d, want 2: %+v", len(states.createdAll), states.createdAll)
	}
	if got := states.createdAll[0]; got.WeekStart != currentStart || got.EventsCount != 2 {
		t.Errorf("текущая неделя = %s с %d занятиями, want %s с 2",
			got.WeekStart, got.EventsCount, currentStart)
	}
	if got := states.createdAll[1]; got.WeekStart != nextStart || got.EventsCount != 1 {
		t.Errorf("следующая неделя = %s с %d занятиями, want %s с 1",
			got.WeekStart, got.EventsCount, nextStart)
	}
	if len(snapshots.saved) != 2 {
		t.Errorf("сохранено снимков = %d, want 2", len(snapshots.saved))
	}
}

// TestCheckWeeksTracksGroupAfterWeeks - группа отмечается, когда решения по
// неделям записаны.
func TestCheckWeeksTracksGroupAfterWeeks(t *testing.T) {
	events, _, _ := twoWeekAnswer()

	tracked := &fakeTracked{}
	svc := watcherService(&fakePortal{events: events}, &fakeWeekStates{}, tracked)

	if err := svc.CheckWeeks(context.Background()); err != nil {
		t.Fatalf("CheckWeeks() error = %v", err)
	}

	if len(tracked.tracked) != 1 || tracked.tracked[0] != "ИТ25-11" {
		t.Errorf("отмечены группы %v, want [ИТ25-11]", tracked.tracked)
	}
}

// TestCheckWeeksSkipsTrackWhenWeekFailed - группа без записанных состояний не
// должна считаться знакомой: иначе следующий прогон объявит уже выложенную
// неделю только что появившейся и разошлёт ложное уведомление.
func TestCheckWeeksSkipsTrackWhenWeekFailed(t *testing.T) {
	events, _, _ := twoWeekAnswer()

	tracked := &fakeTracked{}
	states := &fakeWeekStates{findErr: errors.New("mongo is down")}
	svc := watcherService(&fakePortal{events: events}, states, tracked)

	if err := svc.CheckWeeks(context.Background()); err != nil {
		t.Fatalf("CheckWeeks() error = %v", err)
	}

	if len(tracked.tracked) != 0 {
		t.Errorf("отмечены группы %v, want пусто: состояния недель не записаны", tracked.tracked)
	}
}

// TestCheckWeeksSurvivesPortalFailure - лежачий портал прогон не роняет и
// группу знакомой не делает.
func TestCheckWeeksSurvivesPortalFailure(t *testing.T) {
	tracked := &fakeTracked{}
	portal := &fakePortal{err: domain.ErrPortalUnavailable}
	states := &fakeWeekStates{}

	if err := watcherService(portal, states, tracked).CheckWeeks(context.Background()); err != nil {
		t.Fatalf("CheckWeeks() error = %v", err)
	}

	if len(tracked.tracked) != 0 {
		t.Errorf("отмечены группы %v, want пусто", tracked.tracked)
	}
	if len(states.createdAll) != 0 {
		t.Errorf("заведено состояний %d, want 0", len(states.createdAll))
	}
}

// TestCheckWeeksStopsOnCanceledContext - отмена контекста прекращает прогон, не
// доходя до портала.
func TestCheckWeeksStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	portal := &fakePortal{}
	err := watcherService(portal, &fakeWeekStates{}, &fakeTracked{}).CheckWeeks(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("CheckWeeks() error = %v, want context.Canceled", err)
	}
	if portal.calls != 0 {
		t.Errorf("походов в портал = %d, want 0", portal.calls)
	}
}
