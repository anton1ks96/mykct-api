package service

import (
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
)

// Имена полей, по которым занятия расходятся. Уезжают в уведомление, поэтому
// короткие и машиночитаемые.
const (
	fieldDay      = "day"
	fieldStart    = "start"
	fieldEnd      = "end"
	fieldRoom     = "room"
	fieldTitle    = "title"
	fieldType     = "type"
	fieldSubGroup = "subgroup"
)

// eventFingerprint собирает отпечаток занятия из полей, расхождение в которых
// считается изменением расписания.
func eventFingerprint(e domain.Event) string {
	// Тема и цвет в отпечаток не входят: тему преподаватели дописывают почти на
	// каждой паре, и детект свёлся бы к ней одной
	return strings.Join([]string{
		e.Day, e.Start, e.End, e.Room, e.Title, e.Type, subGroupFingerprint(e.SubGroup),
	}, "|")
}

// subGroupFingerprint собирает отпечаток подгрупп занятия.
func subGroupFingerprint(subgroups []domain.SubGroup) string {
	if len(subgroups) == 0 {
		return ""
	}

	// Подгруппы сортируются: порядок портала не гарантирован. Идентификаторы
	// занятий подгрупп не берутся - портал мог пересоздать их, не меняя сути
	parts := make([]string, 0, len(subgroups))
	for _, sg := range subgroups {
		parts = append(parts, sg.SGrID+"~"+sg.SGCaID+"~"+sg.STitle)
	}
	sort.Strings(parts)

	return strings.Join(parts, ";")
}

// baselineEvents приводит занятия к виду, в котором они ложатся в базис.
func baselineEvents(events []domain.Event) []domain.Event {
	out := make([]domain.Event, 0, len(events))
	for _, event := range events {
		out = append(out, baselineEvent(event))
	}

	return out
}

// baselineEvent очищает поля, которых нет в отпечатке. Базис обновляется только
// при расхождении отпечатков, поэтому тема или цвет пролежали бы в нём
// устаревшими и уехали в уведомление как "было".
func baselineEvent(event domain.Event) domain.Event {
	event.Topic = ""
	event.Color = ""

	if len(event.SubGroup) == 0 {
		return event
	}

	// Подгруппы копируются: тот же слайс уходит в снимок расписания и в
	// новую сторону изменения, чистить его на месте нельзя
	subgroups := make([]domain.SubGroup, 0, len(event.SubGroup))
	for _, sg := range event.SubGroup {
		subgroups = append(subgroups, domain.SubGroup{
			SGrID:  sg.SGrID,
			SGCaID: sg.SGCaID,
			STitle: sg.STitle,
		})
	}
	event.SubGroup = subgroups

	return event
}

// eventsHash - отпечаток всей недели. Отпечатки занятий сортируются, поэтому
// перестановка занятий местами хэш не двигает. Пустая неделя - пустой хэш.
func eventsHash(events []domain.Event) string {
	if len(events) == 0 {
		return ""
	}

	prints := make([]string, 0, len(events))
	for _, e := range events {
		prints = append(prints, eventFingerprint(e))
	}
	sort.Strings(prints)

	h := fnv.New64a()
	for _, p := range prints {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}

	return strconv.FormatUint(h.Sum64(), 16)
}

// changedFields перечисляет поля, которыми занятия разошлись. Пустой результат
// означает, что для расписания занятия одинаковы.
func changedFields(before, after domain.Event) []string {
	pairs := []struct {
		name          string
		before, after string
	}{
		{fieldDay, before.Day, after.Day},
		{fieldStart, before.Start, after.Start},
		{fieldEnd, before.End, after.End},
		{fieldRoom, before.Room, after.Room},
		{fieldTitle, before.Title, after.Title},
		{fieldType, before.Type, after.Type},
		{fieldSubGroup, subGroupFingerprint(before.SubGroup), subGroupFingerprint(after.SubGroup)},
	}

	fields := make([]string, 0, len(pairs))
	for _, p := range pairs {
		if p.before != p.after {
			fields = append(fields, p.name)
		}
	}

	if len(fields) == 0 {
		return nil
	}

	return fields
}

// diffEvents сравнивает базовый снимок недели со свежим ответом портала.
func diffEvents(prev, next []domain.Event) []domain.EventChange {
	usedPrev := make([]bool, len(prev))
	usedNext := make([]bool, len(next))

	changes := make([]domain.EventChange, 0)

	// 1. Занятие остаётся собой, пока портал держит за ним ClID
	changes = matchEvents(changes, prev, next, usedPrev, usedNext, func(e domain.Event) string {
		return e.ClID
	})

	// 2. Остатки сопоставляются по слоту: портал мог пересоздать пару с новым
	// ClID, но для студента это та же пара в то же время
	changes = matchEvents(changes, prev, next, usedPrev, usedNext, func(e domain.Event) string {
		return e.Day + " " + e.Start
	})

	// 3. Что не с чем сопоставлять - пропало или появилось
	for i := range prev {
		if !usedPrev[i] {
			before := prev[i]
			changes = append(changes, domain.EventChange{Kind: domain.ChangeRemoved, Before: &before})
		}
	}
	for j := range next {
		if !usedNext[j] {
			after := next[j]
			changes = append(changes, domain.EventChange{Kind: domain.ChangeAdded, After: &after})
		}
	}

	sortChanges(changes)

	return changes
}

// matchEvents сопоставляет ещё не разобранные занятия по ключу и дописывает
// расхождения. Занятия, совпавшие по ключу и по сути, просто помечаются
// разобранными.
func matchEvents(
	changes []domain.EventChange,
	prev, next []domain.Event,
	usedPrev, usedNext []bool,
	key func(domain.Event) string,
) []domain.EventChange {
	index := make(map[string]int, len(next))
	for j, e := range next {
		k := key(e)
		if usedNext[j] || k == "" {
			continue
		}
		// Ключ-дубль разбирает первое занятие, остальные уходят дальше по цепочке
		if _, ok := index[k]; !ok {
			index[k] = j
		}
	}

	for i, before := range prev {
		k := key(before)
		if usedPrev[i] || k == "" {
			continue
		}

		j, ok := index[k]
		if !ok || usedNext[j] {
			continue
		}

		usedPrev[i], usedNext[j] = true, true

		fields := changedFields(before, next[j])
		if fields == nil {
			continue
		}

		after := next[j]
		changes = append(changes, domain.EventChange{
			Kind:   domain.ChangeChanged,
			Fields: fields,
			Before: &before,
			After:  &after,
		})
	}

	return changes
}

// sortChanges приводит разницу к устойчивому порядку: по дню и времени занятия,
// затем по виду и идентификатору.
func sortChanges(changes []domain.EventChange) {
	sort.SliceStable(changes, func(i, j int) bool {
		a, b := changeEvent(changes[i]), changeEvent(changes[j])

		if a.Day != b.Day {
			return a.Day < b.Day
		}
		if am, bm := startMinutes(a.Start), startMinutes(b.Start); am != bm {
			return am < bm
		}
		if changes[i].Kind != changes[j].Kind {
			return changes[i].Kind < changes[j].Kind
		}

		return a.ClID < b.ClID
	})
}

// changeEvent - занятие, которым изменение представляется в сортировке и
// отборе: новое состояние, а для пропавшего занятия - старое.
func changeEvent(change domain.EventChange) domain.Event {
	if change.After != nil {
		return *change.After
	}
	if change.Before != nil {
		return *change.Before
	}

	return domain.Event{}
}

// dropPastChanges выбрасывает изменения, которые уже никому не пригодятся:
// отменённая утром пара к вечеру того же дня - такой же мусор, как вчерашняя.
func dropPastChanges(changes []domain.EventChange, now time.Time) []domain.EventChange {
	collegeNow := now.In(collegetime.TZ())
	today := collegetime.Today(now)
	minutes := collegeNow.Hour()*60 + collegeNow.Minute()

	out := make([]domain.EventChange, 0, len(changes))
	for _, change := range changes {
		if eventPassed(change.Before, today, minutes) && eventPassed(change.After, today, minutes) {
			continue
		}
		out = append(out, change)
	}

	return out
}

// eventPassed сообщает, что занятие уже кончилось. Отсутствующая сторона
// изменения удержать его не может, поэтому считается прошедшей.
func eventPassed(event *domain.Event, today string, minutes int) bool {
	if event == nil {
		return true
	}
	if event.Day != today {
		return event.Day < today
	}

	return startMinutes(event.End) <= minutes
}
