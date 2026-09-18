// Package domain содержит доменные модели и ошибки модуля успеваемости.
package domain

// Subject - предмет студента в текущем семестре.
type Subject struct {
	SuIDcrc string // Хэш предмета: subjb4253892, им подписаны оценки в ответе портала
	SuID    string // Код предмета: СГ.02, по нему запрашиваются оценки
	Title   string // Название предмета
}

// Score - оценка за работу на занятии.
type Score struct {
	DateF       string // Фактическая дата оценки, ГГГГ-ММ-ДД; пусто, если портал не заполнил
	DateP       string // Плановая дата работы, ГГГГ-ММ-ДД; пусто, если портал не заполнил
	Score       string // Балл строкой, пусто - оценки ещё нет
	MaxScore    int    // Максимальный балл за работу
	Description string // Описание работы
}

// Scores - оценки за период: хэш предмета (SuIDcrc) -> тема занятия -> оценки.
type Scores map[string]map[string][]Score
