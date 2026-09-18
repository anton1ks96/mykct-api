package service

import (
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
)

// lesson собирает занятие: идентификатор, день, время и аудитория - всё, чем
// занятия различаются в тестах ниже.
func lesson(clid, day, start, end, room, title string) domain.Event {
	return domain.Event{
		ClID:  clid,
		Day:   day,
		Start: start,
		End:   end,
		Room:  room,
		Title: title,
	}
}

// week - неделя из двух занятий, опорный снимок для сравнений.
func week() []domain.Event {
	return []domain.Event{
		lesson("1", "2026-09-14", "09:00", "10:30", "404", "Математика"),
		lesson("2", "2026-09-15", "10:45", "12:15", "3-2", "АиСД"),
	}
}

// changeKinds - виды изменений разницы, по порядку.
func changeKinds(changes []domain.EventChange) []string {
	kinds := make([]string, 0, len(changes))
	for _, ch := range changes {
		kinds = append(kinds, ch.Kind)
	}

	return kinds
}

// TestDiffEventsDetectsChangedFields - правка занятия видна и названа полем.
func TestDiffEventsDetectsChangedFields(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(e *domain.Event)
		field string
	}{
		{"аудитория", func(e *domain.Event) { e.Room = "999" }, fieldRoom},
		{"время начала", func(e *domain.Event) { e.Start = "11:00" }, fieldStart},
		{"предмет", func(e *domain.Event) { e.Title = "Физика" }, fieldTitle},
		{"день", func(e *domain.Event) { e.Day = "2026-09-16" }, fieldDay},
		{"тип занятия", func(e *domain.Event) { e.Type = "2" }, fieldType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := week()
			tt.edit(&next[0])

			changes := diffEvents(week(), next)

			if len(changes) != 1 {
				t.Fatalf("diffEvents() = %d изменений, want 1: %+v", len(changes), changes)
			}
			if changes[0].Kind != domain.ChangeChanged {
				t.Errorf("Kind = %s, want %s", changes[0].Kind, domain.ChangeChanged)
			}
			if len(changes[0].Fields) != 1 || changes[0].Fields[0] != tt.field {
				t.Errorf("Fields = %v, want [%s]", changes[0].Fields, tt.field)
			}
			if changes[0].Before == nil || changes[0].After == nil {
				t.Error("у изменения должны быть обе стороны")
			}
		})
	}
}

// TestDiffEventsIgnoresNoise - поля, которые портал правит сам по себе, в
// разницу не идут: иначе детект сведётся к ним одним.
func TestDiffEventsIgnoresNoise(t *testing.T) {
	tests := []struct {
		name string
		edit func(e *domain.Event)
	}{
		{"тема занятия", func(e *domain.Event) { e.Topic = "Форсайт 2" }},
		{"цвет предмета", func(e *domain.Event) { e.Color = "crimson" }},
		{"тема подгруппы", func(e *domain.Event) { e.SubGroup[0].STopic = "Travelling" }},
		{"идентификатор подгруппы", func(e *domain.Event) { e.SubGroup[0].SClID = "777" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := week()
			prev[0].SubGroup = []domain.SubGroup{{SClID: "10", SGrID: "A0.11", SGCaID: "404", STitle: "АнглЯз"}}

			next := week()
			next[0].SubGroup = []domain.SubGroup{{SClID: "10", SGrID: "A0.11", SGCaID: "404", STitle: "АнглЯз"}}
			tt.edit(&next[0])

			if changes := diffEvents(prev, next); len(changes) != 0 {
				t.Errorf("diffEvents() = %+v, want пусто", changes)
			}
			if eventsHash(prev) != eventsHash(next) {
				t.Error("хэш сдвинулся, хотя разницы нет")
			}
		})
	}
}

// TestDiffEventsDetectsSubGroupRoom - смена аудитории у подгруппы это реальное
// изменение: половина группы идёт в другой кабинет.
func TestDiffEventsDetectsSubGroupRoom(t *testing.T) {
	prev := week()
	prev[0].SubGroup = []domain.SubGroup{{SGrID: "Подгр1", SGCaID: "2-5", STitle: "РазработкаПО"}}

	next := week()
	next[0].SubGroup = []domain.SubGroup{{SGrID: "Подгр1", SGCaID: "3-3", STitle: "РазработкаПО"}}

	changes := diffEvents(prev, next)

	if len(changes) != 1 || changes[0].Kind != domain.ChangeChanged {
		t.Fatalf("diffEvents() = %+v, want одно changed", changes)
	}
	if len(changes[0].Fields) != 1 || changes[0].Fields[0] != fieldSubGroup {
		t.Errorf("Fields = %v, want [%s]", changes[0].Fields, fieldSubGroup)
	}
}

// TestDiffEventsAddedAndRemoved - занятие появилось и занятие пропало.
func TestDiffEventsAddedAndRemoved(t *testing.T) {
	added := lesson("3", "2026-09-16", "13:00", "14:30", "405", "История")

	changes := diffEvents(week(), append(week(), added))
	if len(changes) != 1 || changes[0].Kind != domain.ChangeAdded {
		t.Fatalf("diffEvents() = %+v, want одно added", changes)
	}
	if changes[0].Before != nil || changes[0].After == nil {
		t.Error("у появившегося занятия есть только новая сторона")
	}

	changes = diffEvents(week(), week()[:1])
	if len(changes) != 1 || changes[0].Kind != domain.ChangeRemoved {
		t.Fatalf("diffEvents() = %+v, want одно removed", changes)
	}
	if changes[0].Before == nil || changes[0].After != nil {
		t.Error("у пропавшего занятия есть только старая сторона")
	}
}

// TestDiffEventsReplacedLesson - портал пересоздал пару с новым ClID в том же
// слоте: для студента это замена предмета, а не пара removed+added.
func TestDiffEventsReplacedLesson(t *testing.T) {
	next := week()
	next[0] = lesson("77", "2026-09-14", "09:00", "10:30", "404", "Физика")

	changes := diffEvents(week(), next)

	if len(changes) != 1 {
		t.Fatalf("diffEvents() = %d изменений, want 1: %+v", len(changes), changes)
	}
	if changes[0].Kind != domain.ChangeChanged {
		t.Errorf("Kind = %s, want %s", changes[0].Kind, domain.ChangeChanged)
	}
}

// TestDiffEventsMovedLessonSplits - новый ClID и другое время сопоставлять не
// по чему: это пропавшая пара и появившаяся.
func TestDiffEventsMovedLessonSplits(t *testing.T) {
	next := week()
	next[0] = lesson("77", "2026-09-17", "16:45", "18:15", "404", "Физика")

	changes := diffEvents(week(), next)

	if got := changeKinds(changes); len(got) != 2 {
		t.Fatalf("diffEvents() = %v, want removed и added", got)
	}
	if changes[0].Kind != domain.ChangeRemoved || changes[1].Kind != domain.ChangeAdded {
		t.Errorf("diffEvents() = %v, want [removed added]", changeKinds(changes))
	}
}

// TestDiffEventsIgnoresOrder - портал волен вернуть занятия в другом порядке.
func TestDiffEventsIgnoresOrder(t *testing.T) {
	next := []domain.Event{week()[1], week()[0]}

	if changes := diffEvents(week(), next); len(changes) != 0 {
		t.Errorf("diffEvents() = %+v, want пусто", changes)
	}
	if eventsHash(week()) != eventsHash(next) {
		t.Error("перестановка занятий сдвинула хэш недели")
	}
}

// TestEventsHashMatchesDiff - главный инвариант: хэши равны тогда и только
// тогда, когда разницы нет. Разойдутся - детект начнёт терять изменения.
func TestEventsHashMatchesDiff(t *testing.T) {
	tests := []struct {
		name string
		next []domain.Event
	}{
		{"без правок", week()},
		{"другая аудитория", func() []domain.Event { w := week(); w[0].Room = "999"; return w }()},
		{"другая тема", func() []domain.Event { w := week(); w[0].Topic = "Практика"; return w }()},
		{"занятие пропало", week()[:1]},
		{"занятие появилось", append(week(), lesson("3", "2026-09-16", "13:00", "14:30", "1", "X"))},
		{"неделя опустела", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sameHash := eventsHash(week()) == eventsHash(tt.next)
			noDiff := len(diffEvents(week(), tt.next)) == 0

			if sameHash != noDiff {
				t.Errorf("хэши равны = %v, разница пуста = %v - должны совпадать", sameHash, noDiff)
			}
		})
	}
}

// TestDropPastChanges - в уведомление идёт только то, что ещё впереди.
func TestDropPastChanges(t *testing.T) {
	now := time.Date(2026, time.September, 16, 13, 0, 0, 0, collegetime.TZ())

	tests := []struct {
		name   string
		change domain.EventChange
		keep   bool
	}{
		{"вчерашняя пара", changed(lesson("1", "2026-09-15", "09:00", "10:30", "1", "X")), false},
		{"сегодня, уже кончилась", changed(lesson("1", "2026-09-16", "10:45", "12:15", "1", "X")), false},
		{"сегодня, идёт прямо сейчас", changed(lesson("1", "2026-09-16", "13:00", "14:30", "1", "X")), true},
		{"сегодня, ещё впереди", changed(lesson("1", "2026-09-16", "14:45", "16:15", "1", "X")), true},
		{"завтрашняя пара", changed(lesson("1", "2026-09-17", "09:00", "10:30", "1", "X")), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dropPastChanges([]domain.EventChange{tt.change}, now)

			if keep := len(got) == 1; keep != tt.keep {
				t.Errorf("оставлено = %v, want %v", keep, tt.keep)
			}
		})
	}
}

// TestDropPastChangesKeepsMovedForward - пара уехала из прошлого в будущее:
// старая сторона прошла, новая нет, изменение остаётся.
func TestDropPastChangesKeepsMovedForward(t *testing.T) {
	now := time.Date(2026, time.September, 16, 13, 0, 0, 0, collegetime.TZ())

	change := domain.EventChange{
		Kind:   domain.ChangeChanged,
		Fields: []string{fieldDay},
		Before: ptr(lesson("1", "2026-09-16", "09:00", "10:30", "1", "X")),
		After:  ptr(lesson("1", "2026-09-18", "09:00", "10:30", "1", "X")),
	}

	if got := dropPastChanges([]domain.EventChange{change}, now); len(got) != 1 {
		t.Error("перенос вперёд выбросили, а он ещё впереди")
	}
}

// TestDropPastChangesUsesCollegeTZ - день кончается по времени колледжа: в
// 19:30 UTC в Екатеринбурге уже завтра.
func TestDropPastChangesUsesCollegeTZ(t *testing.T) {
	now := time.Date(2026, time.September, 16, 19, 30, 0, 0, time.UTC)

	change := changed(lesson("1", "2026-09-16", "14:45", "16:15", "1", "X"))

	if got := dropPastChanges([]domain.EventChange{change}, now); len(got) != 0 {
		t.Error("оставили пару прошедшего по времени колледжа дня")
	}
}

// changed - изменение занятия, у которого обе стороны в одном слоте.
func changed(event domain.Event) domain.EventChange {
	after := event
	after.Room = "999"

	return domain.EventChange{
		Kind:   domain.ChangeChanged,
		Fields: []string{fieldRoom},
		Before: &event,
		After:  &after,
	}
}

// ptr - адрес занятия для сторон изменения.
func ptr(event domain.Event) *domain.Event {
	return &event
}

// TestBaselineEventsKeepComparedFields - в базис кладётся ровно то, что
// сравнивается: очистка не двигает отпечаток и не рождает разницу.
func TestBaselineEventsKeepComparedFields(t *testing.T) {
	full := week()
	full[0].Topic = "Форсайт 2"
	full[0].Color = "crimson"
	full[0].SubGroup = []domain.SubGroup{
		{SClID: "10", SGrID: "Подгр1", SGCaID: "2-5", STopic: "Практика", STitle: "РазработкаПО"},
	}

	stripped := baselineEvents(full)

	if eventsHash(stripped) != eventsHash(full) {
		t.Error("очистка базиса сдвинула отпечаток недели")
	}
	if changes := diffEvents(stripped, full); len(changes) != 0 {
		t.Errorf("базис разошёлся с ответом портала: %+v", changes)
	}
	if stripped[0].Topic != "" || stripped[0].Color != "" || stripped[0].SubGroup[0].STopic != "" {
		t.Errorf("несравниваемые поля остались в базисе: %+v", stripped[0])
	}
	if stripped[0].ClID == "" || stripped[0].Room == "" || stripped[0].SubGroup[0].SGCaID == "" {
		t.Errorf("из базиса пропало нужное поле: %+v", stripped[0])
	}
}

// TestBaselineEventsDoNotMutateSource - исходные занятия уходят в снимок и в
// новую сторону изменения, чистить их на месте нельзя.
func TestBaselineEventsDoNotMutateSource(t *testing.T) {
	full := week()
	full[0].Topic = "Практика"
	full[0].SubGroup = []domain.SubGroup{{SClID: "10", SGrID: "Подгр1", SGCaID: "2-5", STopic: "Тема"}}

	baselineEvents(full)

	if full[0].Topic != "Практика" || full[0].SubGroup[0].STopic != "Тема" {
		t.Errorf("исходное занятие испортили: %+v", full[0])
	}
}
