package service

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// GetStreak возвращает серию посещений студента с начала учебного года по сегодня.
func (s *Service) GetStreak(ctx context.Context, login string) (*domain.Streak, error) {
	op := logger.NewLogOp(ctx, log, "GetStreak")

	start, end := academicYearPeriod(time.Now())

	records, err := s.fetchAttendance(ctx, op, login, start, end)
	if err != nil {
		return nil, err
	}

	streak := calculateStreak(records, start, end)

	op.Completed().Str("login", login).Int("current_streak", streak.CurrentStreak).
		Int("school_days", streak.TotalSchoolDays).Msg("attendance streak calculated")

	return &streak, nil
}

// academicYearPeriod возвращает период с 1 сентября текущего учебного года по
// сегодняшний день по времени колледжа, в формате ГГГГ-ММ-ДД.
func academicYearPeriod(now time.Time) (start, end string) {
	now = now.In(collegetime.TZ)

	year := now.Year()
	if now.Month() < time.September {
		year--
	}

	return time.Date(year, time.September, 1, 0, 0, 0, 0, collegetime.TZ).Format(time.DateOnly),
		now.Format(time.DateOnly)
}

// calculateStreak считает серию посещений по отметкам за период. День засчитан,
// если студент был хотя бы на одном занятии; уважительные пропуски не считаются,
// и день только с ними выпадает из подсчёта.
func calculateStreak(records []domain.Record, periodStart, periodEnd string) domain.Streak {
	streak := domain.Streak{PeriodStart: periodStart, PeriodEnd: periodEnd}

	// 1. Отметки сводятся к учебным дням
	attended := make(map[string]bool)
	for _, r := range records {
		if r.Status == domain.StatusExcused {
			continue
		}
		attended[r.Day] = attended[r.Day] || r.Status == domain.StatusPresent
	}
	// Сегодня без посещения ещё не итог: неотмеченная пара приходит как пропуск,
	// и серия обнулялась бы до отметки преподавателя. Прогул засчитается завтра
	if present, ok := attended[periodEnd]; ok && !present {
		delete(attended, periodEnd)
	}
	if len(attended) == 0 {
		return streak
	}

	// 2. Даты ГГГГ-ММ-ДД сортируются как строки. К концу прохода от старых дней к
	// новым текущая серия и есть серия от последнего учебного дня
	days := slices.Sorted(maps.Keys(attended))

	current := 0
	for _, day := range days {
		if !attended[day] {
			current = 0
			continue
		}
		current++
		streak.TotalDaysAttended++
		streak.LastAttendedDate = day
		streak.LongestStreak = max(streak.LongestStreak, current)
	}

	streak.CurrentStreak = current
	streak.TotalSchoolDays = len(days)
	streak.AttendanceRate = float64(streak.TotalDaysAttended) / float64(len(days))

	return streak
}
