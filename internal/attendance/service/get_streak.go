package service

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// participantRefreshTimeout - предел на запись строки рейтинга. Считается уже
// после ответа клиенту, поэтому короткий.
const participantRefreshTimeout = 5 * time.Second

// GetStreak возвращает серию посещений студента с начала учебного года по сегодня
// и попутно освежает его строку в рейтинге.
func (s *Service) GetStreak(ctx context.Context, input GetStreakInput) (*domain.Streak, error) {
	op := logger.NewLogOp(ctx, log, "GetStreak")

	start, end := academicYearPeriod(time.Now())

	records, err := s.fetchAttendance(ctx, op, input.Login, start, end)
	if err != nil {
		return nil, err
	}

	streak := calculateStreak(records, start, end)

	op.Completed().Str("login", input.Login).Int("current_streak", streak.CurrentStreak).
		Int("school_days", streak.TotalSchoolDays).Msg("attendance streak calculated")

	s.refreshParticipant(ctx, input, streak, len(records))

	return &streak, nil
}

// refreshParticipant освежает строку студента в рейтинге его собственным
// запросом: поход на портал уже оплачен, второй раз за тем же ходить незачем.
// Ошибка здесь запрос не роняет - рейтинг это побочный эффект выдачи серии.
func (s *Service) refreshParticipant(
	ctx context.Context,
	input GetStreakInput,
	streak domain.Streak,
	records int,
) {
	if !s.cfg.Enabled {
		return
	}

	// Пустой курс - это преподаватель или студент без группы в токене: в
	// рейтинге таким участвовать не с кем
	course := courseFromGroup(input.AcademicGroup)
	if course == "" {
		return
	}

	// Пустой ответ портала неотличим от сбоя: он отвечает 200 с текстом, и
	// валидный пустой список приходит так же. Серию по нему не переписываем,
	// иначе лежащий портал обнулил бы её всем, кто открыл приложение
	if records == 0 {
		return
	}

	// Клиент мог отвалиться, но серия уже посчитана: контекст запроса для записи
	// не годится, иначе результат пропадёт вместе с соединением
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), participantRefreshTimeout)
	defer cancel()

	op := logger.NewLogOp(ctx, log, "refreshParticipant")

	participant := domain.Participant{
		Login:         input.Login,
		AcademicGroup: strings.TrimSpace(input.AcademicGroup),
		Course:        course,
	}
	if err := s.leaderboard.Register(ctx, &participant); err != nil {
		op.Warn().Err(err).Str("login", input.Login).Msg("failed to register leaderboard participant")
		return
	}

	if err := s.leaderboard.SaveStreak(ctx, input.Login, streak, time.Now()); err != nil {
		op.Warn().Err(err).Str("login", input.Login).Msg("failed to refresh leaderboard streak")
		return
	}

	op.Debug().Str("login", input.Login).Msg("leaderboard row refreshed")
}

// academicYearPeriod возвращает период с 1 сентября текущего учебного года по
// сегодняшний день по времени колледжа, в формате ГГГГ-ММ-ДД.
func academicYearPeriod(now time.Time) (start, end string) {
	now = now.In(collegetime.TZ())

	year := now.Year()
	if now.Month() < time.September {
		year--
	}

	return time.Date(year, time.September, 1, 0, 0, 0, 0, collegetime.TZ()).Format(time.DateOnly),
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
