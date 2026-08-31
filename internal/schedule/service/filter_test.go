package service

import (
	"testing"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// englishEvent - занятие английского: портал делит его на четыре подгруппы,
// у каждой своя аудитория и свой идентификатор для деталей.
func englishEvent() domain.Event {
	return domain.Event{
		ClID: "6953", Day: "2026-04-06", Start: "09:00", End: "10:30", Title: "АнглЯз",
		SubGroup: []domain.SubGroup{
			{SClID: "8653", SGrID: "A0.11", SGCaID: "404", STitle: "АнглЯз", STopic: "ВПР!"},
			{SClID: "8654", SGrID: "A1.11", SGCaID: "405", STitle: "АнглЯз"},
			{SClID: "8655", SGrID: "A2.11", SGCaID: "403", STitle: "АнглЯз"},
			{SClID: "8656", SGrID: "B1.11", SGCaID: "410", STitle: "АнглЯз"},
		},
	}
}

// splitEvent - занятие, поделённое на подгруппы группы или профиля.
func splitEvent() domain.Event {
	return domain.Event{
		ClID: "6969", Day: "2026-04-07", Start: "14:45", End: "16:15", Title: "ВведСпец",
		SubGroup: []domain.SubGroup{
			{SClID: "8671", SGrID: "Подгр1", SGCaID: "2-4", STitle: "ВведСпец"},
			{SClID: "8672", SGrID: "Подгр2", SGCaID: "2-5", STitle: "ВведСпец"},
		},
	}
}

// sportEvent - физкультура: секции идут всем и не отбрасываются.
func sportEvent() domain.Event {
	return domain.Event{
		ClID: "6958", Day: "2026-04-07", Start: "10:45", End: "12:15", Title: "Физкульт",
		SubGroup: []domain.SubGroup{
			{SClID: "8660", SGrID: "БрайтФит", STitle: "Физкульт"},
			{SClID: "8661", SGrID: "ФизраКол", STitle: "Физкульт"},
		},
	}
}

// profileEvent - занятие профиля: чужому профилю оно не показывается.
func profileEvent() domain.Event {
	return domain.Event{
		ClID: "7100", Day: "2026-04-08", Start: "09:00", End: "10:30", Title: "ПрофПредмет",
		SubGroup: []domain.SubGroup{
			{SClID: "9100", SGrID: "CD", SGCaID: "3-1", STitle: "Дизайн"},
		},
	}
}

// plainEvent - занятие без подгрупп, идёт всей группе.
func plainEvent() domain.Event {
	return domain.Event{
		ClID: "6954", Day: "2026-04-06", Start: "10:45", End: "12:15",
		Title: "Математика", Room: "3-2",
	}
}

// findEvent ищет занятие по названию, чтобы тесты не зависели от порядка.
func findEvent(events []domain.Event, title string) *domain.Event {
	for i := range events {
		if events[i].Title == title {
			return &events[i]
		}
	}
	return nil
}

// subgroupIDs собирает имена оставшихся подгрупп занятия.
func subgroupIDs(event *domain.Event) []string {
	if event == nil {
		return nil
	}

	ids := make([]string, 0, len(event.SubGroup))
	for _, sg := range event.SubGroup {
		ids = append(ids, sg.SGrID)
	}

	return ids
}

func TestSelectEventsWithoutSelection(t *testing.T) {
	events := []domain.Event{englishEvent(), splitEvent(), plainEvent(), profileEvent()}

	result := selectEvents(events, GetScheduleInput{Group: "ИТ25-11"})

	if len(result) != 4 {
		t.Fatalf("expected all 4 events, got %d", len(result))
	}
	if got := subgroupIDs(findEvent(result, "АнглЯз")); len(got) != 4 {
		t.Errorf("expected all english subgroups, got %v", got)
	}
	if english := findEvent(result, "АнглЯз"); english.ClID != "6953" {
		t.Errorf("expected parent ClID 6953, got %s", english.ClID)
	}
}

func TestSelectEventsByEnglishGroupOnly(t *testing.T) {
	events := []domain.Event{englishEvent(), splitEvent(), plainEvent()}

	result := selectEvents(events, GetScheduleInput{Group: "ИТ25-11", EnglishGroup: "B1.11"})

	if len(result) != 3 {
		t.Fatalf("expected 3 events, got %d", len(result))
	}

	english := findEvent(result, "АнглЯз")
	if len(english.SubGroup) != 0 {
		t.Errorf("expected collapsed english event, got %v", subgroupIDs(english))
	}
	if english.Room != "410" {
		t.Errorf("expected room 410, got %q", english.Room)
	}
	if english.ClID != "8656" {
		t.Errorf("expected subgroup SClID 8656, got %s", english.ClID)
	}

	// Деление группы остаётся нетронутым: подгруппу пользователь не выбирал
	if got := subgroupIDs(findEvent(result, "ВведСпец")); len(got) != 2 {
		t.Errorf("expected both subgroups, got %v", got)
	}
}

func TestSelectEventsBySubgroupAndEnglishGroup(t *testing.T) {
	events := []domain.Event{englishEvent(), splitEvent(), sportEvent()}

	result := selectEvents(events, GetScheduleInput{
		Group: "ИТ25-11", Subgroup: "Подгр1", EnglishGroup: "A0.11",
	})

	english := findEvent(result, "АнглЯз")
	if english.ClID != "8653" || english.Room != "404" || english.Topic != "ВПР!" {
		t.Errorf("unexpected english event: clid=%s room=%s topic=%s",
			english.ClID, english.Room, english.Topic)
	}

	split := findEvent(result, "ВведСпец")
	if split.ClID != "8671" || split.Room != "2-4" {
		t.Errorf("unexpected split event: clid=%s room=%s", split.ClID, split.Room)
	}

	// Секции физкультуры идут всем, поэтому их две и занятие не схлопывается
	if got := subgroupIDs(findEvent(result, "Физкульт")); len(got) != 2 {
		t.Errorf("expected both sport sections, got %v", got)
	}
}

func TestSelectEventsKeepsParentClIDWithoutSClID(t *testing.T) {
	event := englishEvent()
	for i := range event.SubGroup {
		event.SubGroup[i].SClID = ""
	}

	result := selectEvents([]domain.Event{event}, GetScheduleInput{
		Group: "ИТ25-11", EnglishGroup: "B1.11",
	})

	if got := result[0].ClID; got != "6953" {
		t.Errorf("expected parent ClID 6953, got %s", got)
	}
}

func TestSelectEventsByProfile(t *testing.T) {
	events := []domain.Event{profileEvent(), plainEvent()}

	// Схлопнутое занятие описывается подгруппой, поэтому название - её
	own := selectEvents(events, GetScheduleInput{Group: "ИТ24-14", Subgroup: "CD"})
	if profile := findEvent(own, "Дизайн"); profile == nil || profile.ClID != "9100" {
		t.Errorf("expected own profile event with subgroup ClID, got %v", own)
	}

	foreign := selectEvents(events, GetScheduleInput{Group: "ИТ24-14", Subgroup: "BE"})
	if len(foreign) != 1 {
		t.Errorf("expected foreign profile event to be dropped, got %v", foreign)
	}
	if findEvent(foreign, "Математика") == nil {
		t.Error("expected event without subgroups to stay")
	}
}

func TestSelectEventsByProfileSubgroup(t *testing.T) {
	events := []domain.Event{splitEvent()}

	result := selectEvents(events, GetScheduleInput{
		Group: "ИТ24-14", Subgroup: "CD", ProfileSubgroup: "Подгр2",
	})

	if len(result) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result))
	}
	if result[0].Room != "2-5" || result[0].ClID != "8672" {
		t.Errorf("unexpected event: clid=%s room=%s", result[0].ClID, result[0].Room)
	}
}

func TestSelectEventsSortsByDayAndStart(t *testing.T) {
	events := []domain.Event{
		{ClID: "3", Day: "2026-04-07", Start: "09:00", Title: "Третье"},
		{ClID: "2", Day: "2026-04-06", Start: "13:00", Title: "Второе"},
		{ClID: "1", Day: "2026-04-06", Start: "9:00", Title: "Первое"},
	}

	result := selectEvents(events, GetScheduleInput{Group: "ИТ25-11"})

	for i, want := range []string{"Первое", "Второе", "Третье"} {
		if result[i].Title != want {
			t.Errorf("position %d: expected %s, got %s", i, want, result[i].Title)
		}
	}
}
