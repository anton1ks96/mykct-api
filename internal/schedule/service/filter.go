package service

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// englishRe распознаёт подгруппу английского языка: A0.11, B1.21.
var englishRe = regexp.MustCompile(`^(A0|A1|A2|B1)\.\d{2}$`)

// sportSubgroups - спортивные секции, они идут всем и никогда не отбрасываются.
var sportSubgroups = []string{"ФизраКол", "БрайтФит", "БаскетКол"}

// anySubgroup сообщает, что отбор по этому значению не нужен.
func anySubgroup(value string) bool {
	return value == "" || value == "*"
}

// hasSelection сообщает, что выдачу сузили хотя бы одним параметром.
func hasSelection(input GetScheduleInput) bool {
	return !anySubgroup(input.Subgroup) ||
		!anySubgroup(input.EnglishGroup) ||
		!anySubgroup(input.ProfileSubgroup)
}

// selectEvents отбирает занятия под выбранные подгруппы, схлопывает одиночную
// подгруппу в само занятие и сортирует результат по дате и времени начала.
func selectEvents(events []domain.Event, input GetScheduleInput) []domain.Event {
	result := filterBySelection(events, input)

	// Когда после отбора осталась одна подгруппа, занятие описывается ею самой
	if hasSelection(input) {
		for i := range result {
			if len(result[i].SubGroup) != 1 {
				continue
			}

			sg := result[i].SubGroup[0]
			if sg.SClID != "" {
				result[i].ClID = sg.SClID
			}
			result[i].Title = sg.STitle
			if result[i].Topic == "" {
				result[i].Topic = sg.STopic
			}
			if result[i].Room == "" {
				result[i].Room = sg.SGCaID
			}
			result[i].SubGroup = nil
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Day != result[j].Day {
			return result[i].Day < result[j].Day
		}
		return startMinutes(result[i].Start) < startMinutes(result[j].Start)
	})

	return result
}

// filterBySelection оставляет занятия, подходящие выбранным подгруппам. Занятие
// без подгрупп идёт всем; занятие, у которого не выжила ни одна подгруппа,
// выпадает целиком.
func filterBySelection(events []domain.Event, input GetScheduleInput) []domain.Event {
	if !hasSelection(input) {
		return events
	}

	out := make([]domain.Event, 0, len(events))

	for _, ev := range events {
		if len(ev.SubGroup) == 0 {
			out = append(out, ev)
			continue
		}

		filtered := make([]domain.SubGroup, 0, len(ev.SubGroup))
		for _, sg := range ev.SubGroup {
			if matchesSelection(sg.SGrID, input) {
				filtered = append(filtered, sg)
			}
		}

		if len(filtered) == 0 {
			continue
		}

		ev.SubGroup = filtered
		out = append(out, ev)
	}

	return out
}

// matchesSelection решает, подходит ли подгруппа занятия выбору пользователя.
func matchesSelection(subgroupID string, input GetScheduleInput) bool {
	// 1. Спортивные секции идут всем
	for _, sport := range sportSubgroups {
		if strings.EqualFold(subgroupID, sport) {
			return true
		}
	}

	// 2. Точное совпадение с выбранной подгруппой или профилем
	if strings.EqualFold(subgroupID, input.Subgroup) {
		return true
	}

	// 3. Английский отбирается своей подгруппой
	if englishRe.MatchString(subgroupID) {
		return anySubgroup(input.EnglishGroup) || strings.EqualFold(subgroupID, input.EnglishGroup)
	}

	// 4. Деление на подгруппы: у старших курсов выбран профиль, а дробится
	// занятие по подгруппам внутри него
	if strings.HasPrefix(subgroupID, "Подгр") {
		mainSubgroup := input.ProfileSubgroup
		if strings.HasPrefix(input.Subgroup, "Подгр") {
			mainSubgroup = input.Subgroup
		}

		// Только здесь "Все" тоже означает отсутствие отбора
		if anySubgroup(mainSubgroup) || strings.EqualFold(mainSubgroup, "Все") {
			return true
		}

		return strings.EqualFold(subgroupID, mainSubgroup)
	}

	// 5. Профиль и всё прочее отбирается выбранной подгруппой
	return anySubgroup(input.Subgroup)
}

// startMinutes переводит "09:00" в минуты от полуночи. Непонятное время уходит
// в конец дня, чтобы не путать порядок занятий.
func startMinutes(value string) int {
	hour, minute, found := strings.Cut(value, ":")
	if !found {
		return math.MaxInt
	}

	h, err := strconv.Atoi(hour)
	if err != nil {
		return math.MaxInt
	}
	m, err := strconv.Atoi(minute)
	if err != nil {
		return math.MaxInt
	}

	return h*60 + m
}
