package service

import (
	"context"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
)

// fakeChanges - подставное хранилище изменений: запоминает записанное.
type fakeChanges struct {
	saved    []*domain.WeekChanges
	pending  []*domain.WeekChanges
	notified map[string]bool
}

func (f *fakeChanges) Pending(context.Context, time.Time) ([]*domain.WeekChanges, error) {
	return f.pending, nil
}

// MarkNotified отдаёт отметку один раз, как условный апдейт в Mongo.
func (f *fakeChanges) MarkNotified(_ context.Context, id string, _ time.Time) (bool, error) {
	if f.notified == nil {
		f.notified = map[string]bool{}
	}
	if f.notified[id] {
		return false, nil
	}
	f.notified[id] = true
	return true, nil
}

func (f *fakeChanges) Save(_ context.Context, changes *domain.WeekChanges) error {
	f.saved = append(f.saved, changes)
	return nil
}

// baseline - состояние недели с уже засеянным базовым снимком.
func baseline(events []domain.Event) *domain.WeekState {
	return &domain.WeekState{
		Group:      "ИТ25-11",
		WeekStart:  "2026-09-14",
		Events:     events,
		EventsHash: eventsHash(events),
	}
}

// detect прогоняет детект через сервис с подставными хранилищами.
func detect(
	t *testing.T,
	states *fakeWeekStates,
	changes *fakeChanges,
	state *domain.WeekState,
	events []domain.Event,
) int {
	t.Helper()

	// Утро понедельника: занятия опорной недели ещё впереди
	now := time.Date(2026, time.September, 14, 8, 0, 0, 0, collegetime.TZ())

	svc := &Service{states: states, changes: changes}
	count, err := svc.detectWeekChanges(context.Background(), "ИТ25-11", "2026-09-14", state, events, now)
	if err != nil {
		t.Fatalf("detectWeekChanges() error = %v", err)
	}

	return count
}

// TestDetectWeekChangesWritesDiff - изменение недели уезжает в хранилище вместе
// с обеими сторонами.
func TestDetectWeekChangesWritesDiff(t *testing.T) {
	next := week()
	next[0].Room = "999"

	states := &fakeWeekStates{baselineWon: true}
	changes := &fakeChanges{}

	if count := detect(t, states, changes, baseline(week()), next); count != 1 {
		t.Fatalf("detectWeekChanges() = %d, want 1", count)
	}

	if len(changes.saved) != 1 {
		t.Fatalf("записано %d разниц, want 1", len(changes.saved))
	}
	saved := changes.saved[0]
	if saved.Group != "ИТ25-11" || saved.WeekStart != "2026-09-14" {
		t.Errorf("разница записана как %s %s, want ИТ25-11 2026-09-14", saved.Group, saved.WeekStart)
	}
	if saved.NotifiedAt != nil {
		t.Error("свежая разница не может быть разосланной")
	}
	if len(saved.Changes) != 1 || saved.Changes[0].Kind != domain.ChangeChanged {
		t.Errorf("разница = %+v, want одно changed", saved.Changes)
	}
	if states.baselinePrev != eventsHash(week()) || states.baselineNext != eventsHash(next) {
		t.Error("базис меняли не от того отпечатка, от которого считали разницу")
	}
}

// TestDetectWeekChangesKeepsBaselineOnEmptyWeek - пустой ответ портала это чаще
// сбой, чем отменённая неделя: базис остаётся на месте.
func TestDetectWeekChangesKeepsBaselineOnEmptyWeek(t *testing.T) {
	states := &fakeWeekStates{baselineWon: true}
	changes := &fakeChanges{}

	if count := detect(t, states, changes, baseline(week()), nil); count != 0 {
		t.Errorf("detectWeekChanges() = %d, want 0", count)
	}
	if states.baselineNext != "" {
		t.Error("базис снесли пустым ответом портала")
	}
	if len(changes.saved) != 0 {
		t.Error("пустой ответ портала записали как пропажу занятий")
	}
}

// TestDetectWeekChangesSeedsBaseline - первый снимок недели сравнивать не с чем,
// появление расписания ловит decideWeek.
func TestDetectWeekChangesSeedsBaseline(t *testing.T) {
	states := &fakeWeekStates{baselineWon: true}
	changes := &fakeChanges{}

	if count := detect(t, states, changes, nil, week()); count != 0 {
		t.Errorf("detectWeekChanges() = %d, want 0", count)
	}
	if states.baselineNext != eventsHash(week()) {
		t.Error("базис не засеяли")
	}
	if len(changes.saved) != 0 {
		t.Error("засев базиса записали как изменения")
	}
}

// TestDetectWeekChangesSkipsWhenBaselineTaken - гонку инстансов выигрывает один,
// он же и уведомляет.
func TestDetectWeekChangesSkipsWhenBaselineTaken(t *testing.T) {
	next := week()
	next[0].Room = "999"

	changes := &fakeChanges{}

	if count := detect(t, &fakeWeekStates{baselineWon: false}, changes, baseline(week()), next); count != 0 {
		t.Errorf("detectWeekChanges() = %d, want 0", count)
	}
	if len(changes.saved) != 0 {
		t.Error("проигравший гонку инстанс записал свою разницу")
	}
}

// TestDetectWeekChangesSkipsUnchangedWeek - неделя не менялась, базис трогать
// незачем: так проходит подавляющее большинство опросов.
func TestDetectWeekChangesSkipsUnchangedWeek(t *testing.T) {
	states := &fakeWeekStates{baselineWon: true}
	changes := &fakeChanges{}

	if count := detect(t, states, changes, baseline(week()), week()); count != 0 {
		t.Errorf("detectWeekChanges() = %d, want 0", count)
	}
	if states.baselineNext != "" {
		t.Error("базис переписали, хотя неделя не менялась")
	}
	if len(changes.saved) != 0 {
		t.Error("записали разницу там, где её нет")
	}
}

// TestDetectWeekChangesDropsPastButKeepsBaseline - прошедшее изменение не
// уведомление, но базис двигает: иначе его находили бы каждый прогон.
func TestDetectWeekChangesDropsPastButKeepsBaseline(t *testing.T) {
	next := week()
	next[0].Room = "999"

	states := &fakeWeekStates{baselineWon: true}
	changes := &fakeChanges{}

	// Пятница: обе пары опорной недели давно прошли
	now := time.Date(2026, time.September, 18, 12, 0, 0, 0, collegetime.TZ())

	svc := &Service{states: states, changes: changes}
	count, err := svc.detectWeekChanges(context.Background(), "ИТ25-11", "2026-09-14",
		baseline(week()), next, now)
	if err != nil {
		t.Fatalf("detectWeekChanges() error = %v", err)
	}

	if count != 0 {
		t.Errorf("detectWeekChanges() = %d, want 0", count)
	}
	if len(changes.saved) != 0 {
		t.Error("прошедшее изменение записали в уведомления")
	}
	if states.baselineNext != eventsHash(next) {
		t.Error("базис не сдвинули, прошедшее изменение будет находиться снова")
	}
}
