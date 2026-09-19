// Package repository объявляет интерфейсы хранилищ модуля посещаемости.
package repository

import (
	"context"
	"time"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
)

// Portal - портал колледжа, первоисточник посещаемости.
type Portal interface {
	// FetchAttendance возвращает занятия студента за период с отметками посещаемости.
	FetchAttendance(ctx context.Context, login, start, end string) ([]domain.Record, error)
}

// Leaderboard - постоянный реестр участников рейтинга и их посчитанные серии.
// Логин из реестра не выпадает никогда, даже когда протухла refresh-сессия.
type Leaderboard interface {
	// Register заводит участника и освежает его группу, не трогая посчитанную
	// серию и отметку последнего пересчёта.
	Register(ctx context.Context, participant *domain.Participant) error
	// SaveStreak записывает пересчитанную серию и снимает отметку простоя.
	SaveStreak(ctx context.Context, login string, streak domain.Streak, at time.Time) error
	// MarkEmpty считает пустые ответы портала и гасит участника, когда их
	// накопилось limit подряд.
	MarkEmpty(ctx context.Context, login string, limit int) error
	// ByCourse возвращает живых участников курса, от длинной серии к короткой.
	ByCourse(ctx context.Context, course string) ([]domain.Participant, error)
	// Stale возвращает участников, чью серию не пересчитывали дольше срока.
	Stale(ctx context.Context, olderThan time.Time, limit int) ([]domain.Participant, error)
}
