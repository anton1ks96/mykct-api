package collegetime

import (
	"testing"
	"time"
)

// at собирает момент по времени колледжа из даты и часа.
func at(year int, month time.Month, day, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, tz)
}

// TestNextWeekWholeWeek - вся опорная неделя указывает на одну и ту же
// следующую: и понедельник, и воскресенье, на которых чаще всего ошибаются.
func TestNextWeekWholeWeek(t *testing.T) {
	const (
		wantStart = "2026-09-21"
		wantEnd   = "2026-09-27"
	)

	for day := 14; day <= 20; day++ {
		start, end := NextWeek(at(2026, time.September, day, 12))
		if start != wantStart || end != wantEnd {
			t.Errorf("NextWeek(2026-09-%02d) = %s..%s, want %s..%s",
				day, start, end, wantStart, wantEnd)
		}
	}
}

// TestNextWeekCrossesYearAndMonth - арифметика не ломается на границах.
func TestNextWeekCrossesYearAndMonth(t *testing.T) {
	tests := []struct {
		name       string
		now        time.Time
		start, end string
	}{
		{"конец года", at(2026, time.December, 28, 12), "2027-01-04", "2027-01-10"},
		{"конец месяца", at(2026, time.August, 31, 12), "2026-09-07", "2026-09-13"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := NextWeek(tt.now)
			if start != tt.start || end != tt.end {
				t.Errorf("NextWeek() = %s..%s, want %s..%s", start, end, tt.start, tt.end)
			}
		})
	}
}

// TestNextWeekUsesCollegeTZ - неделя считается по времени колледжа, а не по UTC:
// воскресенье 19:30 UTC - это уже понедельник 00:30 в Екатеринбурге.
func TestNextWeekUsesCollegeTZ(t *testing.T) {
	now := time.Date(2026, time.September, 20, 19, 30, 0, 0, time.UTC)

	start, end := NextWeek(now)
	if start != "2026-09-28" || end != "2026-10-04" {
		t.Errorf("NextWeek() = %s..%s, want 2026-09-28..2026-10-04", start, end)
	}
}

// TestNextMondayIsMidnight - граница недели всегда полночь понедельника.
func TestNextMondayIsMidnight(t *testing.T) {
	monday := NextMonday(at(2026, time.September, 18, 23))

	if monday.Weekday() != time.Monday {
		t.Errorf("NextMonday().Weekday() = %v, want Monday", monday.Weekday())
	}
	if h, m, s := monday.Clock(); h != 0 || m != 0 || s != 0 {
		t.Errorf("NextMonday() = %v, want midnight", monday)
	}
	if !monday.After(at(2026, time.September, 18, 23)) {
		t.Errorf("NextMonday() = %v, want a moment in the future", monday)
	}
}

// TestCurrentWeekWholeWeek - вся неделя указывает на саму себя, включая
// понедельник: на нём NextMonday даёт следующую неделю, а CurrentMonday - эту.
func TestCurrentWeekWholeWeek(t *testing.T) {
	const (
		wantStart = "2026-09-14"
		wantEnd   = "2026-09-20"
	)

	for day := 14; day <= 20; day++ {
		start, end := CurrentWeek(at(2026, time.September, day, 12))
		if start != wantStart || end != wantEnd {
			t.Errorf("CurrentWeek(2026-09-%02d) = %s..%s, want %s..%s",
				day, start, end, wantStart, wantEnd)
		}
	}
}

// TestCurrentWeekCrossesYearAndMonth - арифметика не ломается на границах.
func TestCurrentWeekCrossesYearAndMonth(t *testing.T) {
	tests := []struct {
		name       string
		now        time.Time
		start, end string
	}{
		{"конец года", at(2027, time.January, 1, 12), "2026-12-28", "2027-01-03"},
		{"конец месяца", at(2026, time.September, 1, 12), "2026-08-31", "2026-09-06"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := CurrentWeek(tt.now)
			if start != tt.start || end != tt.end {
				t.Errorf("CurrentWeek() = %s..%s, want %s..%s", start, end, tt.start, tt.end)
			}
		})
	}
}

// TestCurrentWeekUsesCollegeTZ - воскресенье 19:30 UTC это уже понедельник в
// Екатеринбурге, то есть началась следующая неделя.
func TestCurrentWeekUsesCollegeTZ(t *testing.T) {
	now := time.Date(2026, time.September, 20, 19, 30, 0, 0, time.UTC)

	start, end := CurrentWeek(now)
	if start != "2026-09-21" || end != "2026-09-27" {
		t.Errorf("CurrentWeek() = %s..%s, want 2026-09-21..2026-09-27", start, end)
	}
}

// TestCurrentWeekMeetsNextWeek - недели стыкуются без зазора и нахлёста.
func TestCurrentWeekMeetsNextWeek(t *testing.T) {
	now := at(2026, time.September, 16, 12)

	_, currentEnd := CurrentWeek(now)
	nextStart, _ := NextWeek(now)

	if want := CurrentMonday(now).AddDate(0, 0, 7).Format(time.DateOnly); nextStart != want {
		t.Errorf("NextWeek() start = %s, want %s", nextStart, want)
	}
	if currentEnd >= nextStart {
		t.Errorf("недели пересекаются: %s >= %s", currentEnd, nextStart)
	}
}

// TestToday - дата берётся по времени колледжа, а не по UTC.
func TestToday(t *testing.T) {
	now := time.Date(2026, time.September, 20, 19, 30, 0, 0, time.UTC)

	if got := Today(now); got != "2026-09-21" {
		t.Errorf("Today() = %s, want 2026-09-21", got)
	}
}
