// Package repository объявляет интерфейсы хранилищ модуля успеваемости.
package repository

import (
	"context"

	"github.com/anton1ks96/mykct-api/internal/performance/domain"
)

// Portal - портал колледжа, первоисточник успеваемости.
type Portal interface {
	// FetchSubjects возвращает предметы студента в текущем семестре.
	FetchSubjects(ctx context.Context, login string) ([]domain.Subject, error)
	// FetchScores возвращает оценки студента по предмету за период.
	FetchScores(ctx context.Context, login, suID, start, end string) (domain.Scores, error)
}
