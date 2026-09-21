// Package repository объявляет интерфейсы хранилищ модуля расписания.
package repository

import (
	"context"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// Portal - портал колледжа, первоисточник расписания.
type Portal interface {
	// FetchSchedule возвращает занятия группы за период без фильтрации по подгруппам.
	FetchSchedule(ctx context.Context, group, start, end string) ([]domain.Event, error)
	// FetchClassDetails возвращает произвольный JSON с деталями занятия.
	FetchClassDetails(ctx context.Context, clid string) (map[string]any, error)
}

// SnapshotRepository - кэш ответов портала, из которого расписание отдаётся,
// пока портал недоступен.
type SnapshotRepository interface {
	// SaveSchedule сохраняет снимок расписания, затирая предыдущий за тот же период.
	SaveSchedule(ctx context.Context, snapshot *domain.Snapshot) error
	// FindSchedule возвращает последний снимок расписания за период.
	FindSchedule(ctx context.Context, group, start, end string) (*domain.Snapshot, error)
	// SaveClassDetails сохраняет детали занятия, затирая предыдущие.
	SaveClassDetails(ctx context.Context, details *domain.ClassDetails) error
	// FindClassDetails возвращает последние сохранённые детали занятия.
	FindClassDetails(ctx context.Context, clid string) (*domain.ClassDetails, error)
}

// WeekStateRepository - состояния недель расписания: по ним ловится появление
// расписания и отсюда же берутся неразосланные уведомления.
type WeekStateRepository interface {
	// Find возвращает состояние недели группы вместе с базовым снимком.
	Find(ctx context.Context, group, weekStart string) (*domain.WeekState, error)
	// FindStatus возвращает состояние недели без базового снимка.
	FindStatus(ctx context.Context, group, weekStart string) (*domain.WeekState, error)
	// Create заводит состояние недели; уже заведённое - ErrWeekStateExists.
	Create(ctx context.Context, state *domain.WeekState) error
	// MarkPublished фиксирует появление расписания одной операцией и сообщает,
	// этот ли вызов зафиксировал переход.
	MarkPublished(ctx context.Context, group, weekStart string, eventsCount int, at time.Time) (bool, error)
	// Touch отмечает, что неделю опросили, не трогая признак публикации.
	Touch(ctx context.Context, group, weekStart string, eventsCount int, at time.Time) error

	// ReplaceBaseline меняет базовый снимок недели, только если он всё ещё тот,
	// от которого считали разницу. false - снимок успел сменить другой инстанс.
	ReplaceBaseline(ctx context.Context, group, weekStart, prevHash, nextHash string,
		events []domain.Event) (bool, error)

	// PendingPublished возвращает недели, опубликованные не раньше since, о
	// которых ещё не уведомляли. Базовый снимок не читается.
	PendingPublished(ctx context.Context, since time.Time) ([]*domain.WeekState, error)
	// MarkNotified отмечает рассылку по неделе и сообщает, этот ли вызов её отметил.
	MarkNotified(ctx context.Context, group, weekStart string, at time.Time) (bool, error)
}

// ChangeRepository - замеченные изменения расписания внутри недели. Источник
// будущей рассылки: неразосланное ищется по notified_at:null.
type ChangeRepository interface {
	// Save записывает разницу, замеченную одним прогоном воркера.
	Save(ctx context.Context, changes *domain.WeekChanges) error
	// Pending возвращает разницы, замеченные не раньше since, о которых ещё не
	// уведомляли, от старых к новым.
	Pending(ctx context.Context, since time.Time) ([]*domain.WeekChanges, error)
	// MarkNotified отмечает рассылку разницы и сообщает, этот ли вызов её отметил.
	MarkNotified(ctx context.Context, id string, at time.Time) (bool, error)
}

// TrackedGroupRepository - отметки о том, с какого момента за группой следят.
// Хранятся без TTL: по ним новая группа отличается от смены недели у знакомой.
type TrackedGroupRepository interface {
	// All возвращает группы, за которыми уже следят.
	All(ctx context.Context) ([]string, error)
	// Track отмечает начало слежения; повторный вызов момент не сдвигает.
	Track(ctx context.Context, group string, at time.Time) error
}
