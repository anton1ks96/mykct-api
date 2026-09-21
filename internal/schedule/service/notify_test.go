package service

import (
	"context"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// fakeNotifier запоминает разосланное.
type fakeNotifier struct {
	sent []string // группа|тип|текст
}

func (f *fakeNotifier) NotifyGroup(_ context.Context, group, _, body string, data map[string]string) error {
	f.sent = append(f.sent, group+"|"+data["type"]+"|"+body)
	return nil
}

// TestNotifyPendingSendsOnce - неразосланное уходит одним пушем, повторный
// прогон его уже не трогает.
func TestNotifyPendingSendsOnce(t *testing.T) {
	math := lesson("1", "2026-09-22", "10:40", "12:10", "404", "Математика")
	states := &fakeWeekStates{pending: []*domain.WeekState{
		{Group: "ИТ25-11", WeekStart: "2026-09-21", WeekEnd: "2026-09-27"},
	}}
	changes := &fakeChanges{pending: []*domain.WeekChanges{
		{ID: "a", Group: "ИТ25-12", WeekStart: "2026-09-21",
			Changes: []domain.EventChange{{Kind: domain.ChangeRemoved, Before: &math}}},
	}}
	notifier := &fakeNotifier{}
	svc := &Service{states: states, changes: changes, notifier: notifier}

	svc.NotifyPending(context.Background(), time.Now().UTC())
	svc.NotifyPending(context.Background(), time.Now().UTC())

	want := []string{
		"ИТ25-11|schedule_published|Выложено расписание на 21.09 - 27.09",
		"ИТ25-12|schedule_changed|Отменена пара: Математика, вт 22.09 в 10:40",
	}
	if len(notifier.sent) != len(want) {
		t.Fatalf("sent %d pushes, want %d: %v", len(notifier.sent), len(want), notifier.sent)
	}
	for i := range want {
		if notifier.sent[i] != want[i] {
			t.Errorf("push %d = %q, want %q", i, notifier.sent[i], want[i])
		}
	}
}

// TestChangesMessage - одно изменение описывается целиком, несколько - сводкой.
func TestChangesMessage(t *testing.T) {
	before := lesson("1", "2026-09-22", "10:40", "12:10", "404", "Математика")
	after := lesson("1", "2026-09-22", "12:20", "13:50", "305", "Математика")
	other := lesson("2", "2026-09-24", "09:00", "10:30", "101", "История")

	moved := &domain.WeekChanges{Changes: []domain.EventChange{
		{Kind: domain.ChangeChanged, Fields: []string{fieldStart, fieldEnd, fieldRoom}, Before: &before, After: &after},
	}}
	if _, body := changesMessage(moved); body != "Математика, вт 22.09 в 12:20: изменились время, аудитория" {
		t.Errorf("single change body = %q", body)
	}

	many := &domain.WeekChanges{Changes: []domain.EventChange{
		{Kind: domain.ChangeAdded, After: &other},
		{Kind: domain.ChangeChanged, Fields: []string{fieldRoom}, Before: &before, After: &after},
	}}
	if _, body := changesMessage(many); body != "Изменений: 2 - вт 22.09, чт 24.09" {
		t.Errorf("summary body = %q", body)
	}
}
