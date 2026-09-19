package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// GetLeaderboard возвращает анонимный рейтинг курса студента и его собственную
// строку. Логин в логах этого юзкейса не появляется: событие со связкой логина и
// псевдонима восстанавливало бы таблицу соответствий из журнала.
func (s *Service) GetLeaderboard(ctx context.Context, input GetLeaderboardInput) (*domain.Leaderboard, error) {
	op := logger.NewLogOp(ctx, log, "GetLeaderboard")

	course := courseFromGroup(input.AcademicGroup)
	if course == "" {
		op.Debug().Msg("leaderboard requested without a course")
		return nil, domain.ErrLeaderboardForbidden
	}

	// Заход в рейтинг заводит участника: иначе новичок не нашёл бы в таблице
	// собственную строку, пока его не подберёт воркер
	participant := domain.Participant{
		Login:         input.Login,
		AcademicGroup: strings.TrimSpace(input.AcademicGroup),
		Course:        course,
	}
	if err := s.leaderboard.Register(ctx, &participant); err != nil {
		op.Failed(err).Str("course", course).Msg("failed to register leaderboard participant")
		return nil, fmt.Errorf("failed to register leaderboard participant: %w", err)
	}

	participants, err := s.leaderboard.ByCourse(ctx, course)
	if err != nil {
		op.Failed(err).Str("course", course).Msg("failed to load course participants")
		return nil, fmt.Errorf("failed to load course participants: %w", err)
	}

	// Порог участников - защита от деанонимизации: в выборке из нескольких
	// человек псевдонимы разбираются с первого взгляда
	if len(participants) < s.cfg.MinParticipants {
		op.Debug().Str("course", course).Msg("leaderboard cohort is too small")
		return nil, domain.ErrLeaderboardTooSmall
	}

	board := rankCohort(participants, input.Login, newAliasMaker(s.aliasSecret, course), s.cfg.TopSize)

	op.Completed().Str("course", course).Int("participants", board.Participants).
		Int("top", len(board.Top)).Msg("leaderboard built")

	return &board, nil
}

// courseFromGroup выводит курс-поток из академической группы: ИТ25-11 -> ИТ25.
// Год набора общий у всех групп курса, поэтому перевод между группами псевдоним
// студента не меняет.
func courseFromGroup(group string) string {
	normalized := strings.ToUpper(strings.TrimSpace(group))

	prefix, _, found := strings.Cut(normalized, "-")
	if !found || prefix == "" {
		return ""
	}

	return prefix
}

// rankCohort строит рейтинг курса: сортирует когорту, раздаёт места и переводит
// её в анонимные строки. Ранжирование делается здесь, а не запросом в базу,
// потому что порядок внутри равных серий считается тем же HMAC, что и псевдоним.
func rankCohort(
	participants []domain.Participant,
	me string,
	aliases aliasMaker,
	topSize int,
) domain.Leaderboard {
	// Равные серии упорядочиваются псевдослучайно. Порядок по логину выдал бы
	// номер студенческого, то есть год и очерёдность зачисления
	sorted := slices.Clone(participants)
	slices.SortFunc(sorted, func(a, b domain.Participant) int {
		if a.CurrentStreak != b.CurrentStreak {
			return cmp.Compare(b.CurrentStreak, a.CurrentStreak)
		}
		return cmp.Compare(aliases.seed(a.Login), aliases.seed(b.Login))
	})

	entries := make([]domain.Entry, 0, len(sorted))
	rank := 0
	for i, participant := range sorted {
		// Места спортивные: равные серии делят одно место, иначе сквозная
		// нумерация выдавала бы случайный порядок за осмысленный
		if i == 0 || participant.CurrentStreak != sorted[i-1].CurrentStreak {
			rank = i + 1
		}
		entries = append(entries, domain.Entry{
			Rank:          rank,
			Alias:         aliases.alias(participant.Login),
			CurrentStreak: participant.CurrentStreak,
			IsMe:          participant.Login == me,
		})
	}

	resolveAliasCollisions(entries)

	board := domain.Leaderboard{Participants: len(entries)}

	// Собственная строка отдаётся всегда, даже когда студент далеко за топом
	for _, entry := range entries {
		if entry.IsMe {
			board.Me = entry
			break
		}
	}

	board.Top = make([]domain.Entry, 0, min(topSize, len(entries)))
	board.Top = append(board.Top, entries[:min(topSize, len(entries))]...)

	return board
}
