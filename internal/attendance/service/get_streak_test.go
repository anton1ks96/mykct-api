package service

import (
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
)

// statusAbsent - пропуск без уважительной причины.
const statusAbsent = 0

// mark - отметка посещаемости занятия в указанный день.
func mark(day string, status int) domain.Record {
	return domain.Record{Day: day, Status: status}
}

// TestCalculateStreakEmpty - без отметок заполнен только период.
func TestCalculateStreakEmpty(t *testing.T) {
	got := calculateStreak(nil, "2026-09-01", "2026-09-15")

	want := domain.Streak{PeriodStart: "2026-09-01", PeriodEnd: "2026-09-15"}
	if got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}

// TestCalculateStreakCurrentAndLongest - серии считаются по дням в порядке дат,
// даже если портал прислал отметки вразнобой.
func TestCalculateStreakCurrentAndLongest(t *testing.T) {
	records := []domain.Record{
		mark("2026-09-08", domain.StatusPresent),
		mark("2026-09-01", domain.StatusPresent),
		mark("2026-09-02", domain.StatusPresent),
		mark("2026-09-03", domain.StatusPresent),
		mark("2026-09-04", statusAbsent),
		mark("2026-09-07", domain.StatusPresent),
	}

	got := calculateStreak(records, "2026-09-01", "2026-09-08")

	if got.CurrentStreak != 2 {
		t.Errorf("expected current streak 2, got %d", got.CurrentStreak)
	}
	if got.LongestStreak != 3 {
		t.Errorf("expected longest streak 3, got %d", got.LongestStreak)
	}
	if got.TotalDaysAttended != 5 || got.TotalSchoolDays != 6 {
		t.Errorf("expected 5 of 6 days, got %d of %d", got.TotalDaysAttended, got.TotalSchoolDays)
	}
	if got.AttendanceRate != 5.0/6.0 {
		t.Errorf("expected rate %v, got %v", 5.0/6.0, got.AttendanceRate)
	}
	if got.LastAttendedDate != "2026-09-08" {
		t.Errorf("expected last attended 2026-09-08, got %q", got.LastAttendedDate)
	}
}

// TestCalculateStreakBrokenByLastDay - пропуск в последний учебный день обнуляет
// текущую серию, но не последнюю дату посещения.
func TestCalculateStreakBrokenByLastDay(t *testing.T) {
	records := []domain.Record{
		mark("2026-09-01", domain.StatusPresent),
		mark("2026-09-02", statusAbsent),
	}

	got := calculateStreak(records, "2026-09-01", "2026-09-02")

	if got.CurrentStreak != 0 {
		t.Errorf("expected current streak 0, got %d", got.CurrentStreak)
	}
	if got.LastAttendedDate != "2026-09-01" {
		t.Errorf("expected last attended 2026-09-01, got %q", got.LastAttendedDate)
	}
}

// TestCalculateStreakAnyPresenceCounts - день засчитан, если студент был хотя бы
// на одном занятии.
func TestCalculateStreakAnyPresenceCounts(t *testing.T) {
	records := []domain.Record{
		mark("2026-09-01", statusAbsent),
		mark("2026-09-01", domain.StatusPresent),
		mark("2026-09-01", statusAbsent),
	}

	got := calculateStreak(records, "2026-09-01", "2026-09-01")

	if got.TotalDaysAttended != 1 || got.TotalSchoolDays != 1 {
		t.Errorf("expected 1 of 1 days, got %d of %d", got.TotalDaysAttended, got.TotalSchoolDays)
	}
}

// TestCalculateStreakSkipsExcused - день только с уважительными пропусками не
// рвёт серию и не считается учебным, а вместе с прогулом день не засчитан.
func TestCalculateStreakSkipsExcused(t *testing.T) {
	records := []domain.Record{
		mark("2026-09-01", domain.StatusPresent),
		mark("2026-09-02", domain.StatusExcused),
		mark("2026-09-03", domain.StatusPresent),
		mark("2026-09-04", domain.StatusExcused),
		mark("2026-09-04", statusAbsent),
	}

	got := calculateStreak(records, "2026-09-01", "2026-09-04")

	if got.LongestStreak != 2 {
		t.Errorf("expected longest streak 2, got %d", got.LongestStreak)
	}
	if got.CurrentStreak != 0 {
		t.Errorf("expected current streak 0, got %d", got.CurrentStreak)
	}
	if got.TotalSchoolDays != 3 {
		t.Errorf("expected 3 school days, got %d", got.TotalSchoolDays)
	}
}

// TestAcademicYearPeriod - учебный год начинается 1 сентября, а сегодняшний день
// берётся по времени колледжа.
func TestAcademicYearPeriod(t *testing.T) {
	cases := []struct {
		now        time.Time
		start, end string
	}{
		{time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC), "2026-09-01", "2026-09-15"},
		{time.Date(2027, time.March, 10, 12, 0, 0, 0, time.UTC), "2026-09-01", "2027-03-10"},
		// 31 августа 20:00 UTC - в Екатеринбурге уже 1 сентября
		{time.Date(2026, time.August, 31, 20, 0, 0, 0, time.UTC), "2026-09-01", "2026-09-01"},
	}

	for _, tc := range cases {
		start, end := academicYearPeriod(tc.now)
		if start != tc.start || end != tc.end {
			t.Errorf("for %s expected %s..%s, got %s..%s", tc.now, tc.start, tc.end, start, end)
		}
	}
}
