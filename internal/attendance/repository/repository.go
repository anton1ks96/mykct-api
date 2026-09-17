// Package repository объявляет интерфейсы хранилищ модуля посещаемости.
package repository

import (
	"context"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
)

// Portal - портал колледжа, первоисточник посещаемости.
type Portal interface {
	// FetchAttendance возвращает занятия студента за период с отметками посещаемости.
	FetchAttendance(ctx context.Context, login, start, end string) ([]domain.Record, error)
}
