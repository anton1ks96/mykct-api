package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// portalFailureLimit - сколько отказов портала подряд терпит прогон, прежде чем
// бросить его целиком: лежачий портал лежит сразу для всех студентов.
const portalFailureLimit = 3

// minRefreshTick - нижняя граница паузы между прогонами, страховка от busy loop.
const minRefreshTick = time.Minute

// refreshStartupJitter - разброс первого прогона, чтобы перезапуски не били по
// порталу разом.
const refreshStartupJitter = 30 * time.Second

// refreshAction - что сделать со строкой участника по ответу портала.
type refreshAction int

const (
	// refreshSkip - портал не ответил: строку не трогаем.
	refreshSkip refreshAction = iota
	// refreshEmpty - портал ответил пусто: возможно, логин погас.
	refreshEmpty
	// refreshSave - отметки есть: пересчитываем серию.
	refreshSave
)

// decideRefresh решает судьбу строки участника по ответу портала. Пустой ответ
// не переписывает серию: портал отвечает на ошибку кодом 200 с текстом, поэтому
// сбой и честно пустой список неотличимы, а сохранение нуля обнулило бы серии
// всему курсу разом.
func decideRefresh(records []domain.Record, err error) refreshAction {
	if err != nil {
		return refreshSkip
	}
	if len(records) == 0 {
		return refreshEmpty
	}

	return refreshSave
}

// RunLeaderboardWorker пересчитывает серии участников рейтинга, пока не отменят
// контекст. Прогон берёт только залежавшиеся строки, поэтому нагрузка на портал
// ограничена размером выборки, а не размером реестра.
func (s *Service) RunLeaderboardWorker(ctx context.Context) {
	log.Info().Dur("interval", s.cfg.RefreshInterval).Dur("refresh_ttl", s.cfg.RefreshTTL).
		Int("batch_size", s.cfg.BatchSize).Msg("leaderboard worker started")

	// Первый прогон разносится случайной паузой: иначе перезапуск или раскатка
	// нескольких инстансов бьёт по порталу всеми участниками разом
	if !refreshSleep(ctx, time.Duration(rand.Int64N(int64(refreshStartupJitter)))) {
		log.Info().Msg("leaderboard worker stopped")
		return
	}

	for {
		if err := s.RefreshParticipants(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Warn().Err(err).Msg("leaderboard refresh failed")
		}

		if !refreshSleep(ctx, nextRefreshTick(s.cfg.RefreshInterval)) {
			log.Info().Msg("leaderboard worker stopped")
			return
		}
	}
}

// RefreshParticipants - один прогон воркера: подбирает в реестр студентов,
// пользующихся приложением, и пересчитывает самые залежавшиеся серии.
func (s *Service) RefreshParticipants(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "RefreshParticipants")
	op.Started().Msg("leaderboard refresh started")

	if err := s.registerActiveStudents(ctx); err != nil {
		// Реестр постоянный, поэтому пополнение не обязано удаваться: уже
		// известных участников пересчитываем в любом случае
		op.Warn().Err(err).Msg("failed to register active students")
	}

	participants, err := s.leaderboard.Stale(ctx, time.Now().Add(-s.cfg.RefreshTTL), s.cfg.BatchSize)
	if err != nil {
		op.Failed(err).Msg("failed to list stale participants")
		return fmt.Errorf("failed to list stale participants: %w", err)
	}
	if len(participants) == 0 {
		op.Completed().Msg("no stale participants to refresh")
		return nil
	}

	start, end := academicYearPeriod(time.Now())

	var refreshed, failures int
	for _, participant := range participants {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		records, err := s.portal.FetchAttendance(ctx, participant.Login, start, end)

		switch decideRefresh(records, err) {
		case refreshSkip:
			failures++
			op.Warn().Err(err).Str("login", participant.Login).
				Msg("failed to fetch attendance for participant")
			// Лежачий портал лежит для всех, дальше идти смысла нет
			if failures >= portalFailureLimit {
				op.Failed(err).Int("refreshed", refreshed).
					Msg("leaderboard refresh aborted, portal keeps failing")
				return fmt.Errorf("portal failed %d times in a row: %w", failures, err)
			}
		case refreshEmpty:
			failures = 0
			if err := s.leaderboard.MarkEmpty(ctx, participant.Login, s.cfg.EmptyRunsLimit); err != nil {
				op.Warn().Err(err).Str("login", participant.Login).
					Msg("failed to mark empty portal answer")
			}
		case refreshSave:
			failures = 0
			streak := calculateStreak(records, start, end)
			if err := s.leaderboard.SaveStreak(ctx, participant.Login, streak, time.Now()); err != nil {
				op.Warn().Err(err).Str("login", participant.Login).Msg("failed to save participant streak")
				continue
			}
			refreshed++
		}

		if !refreshSleep(ctx, s.cfg.StudentDelay) {
			return ctx.Err()
		}
	}

	op.Completed().Int("participants", len(participants)).Int("refreshed", refreshed).
		Msg("leaderboard refresh completed")

	return nil
}

// registerActiveStudents подбирает в реестр студентов с живыми сессиями. Реестр
// постоянный: попавший в него логин не выпадает, когда сессия протухнет.
func (s *Service) registerActiveStudents(ctx context.Context) error {
	students, err := s.students.ActiveStudents(ctx)
	if err != nil {
		return fmt.Errorf("failed to list active students: %w", err)
	}

	for _, student := range students {
		course := courseFromGroup(student.AcademicGroup)
		if course == "" {
			continue
		}

		participant := domain.Participant{
			Login:         student.UserID,
			AcademicGroup: student.AcademicGroup,
			Course:        course,
		}
		if err := s.leaderboard.Register(ctx, &participant); err != nil {
			return fmt.Errorf("failed to register participant: %w", err)
		}
	}

	return nil
}

// nextRefreshTick - пауза до следующего прогона, не короче нижней границы.
func nextRefreshTick(interval time.Duration) time.Duration {
	if interval < minRefreshTick {
		return minRefreshTick
	}

	return interval
}

// refreshSleep ждёт паузу или отмену контекста. false - контекст отменён.
func refreshSleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}

	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
