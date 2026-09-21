package domain

import "time"

// Виды изменений в расписании недели.
const (
	// ChangeAdded - занятие появилось.
	ChangeAdded = "added"
	// ChangeRemoved - занятие пропало.
	ChangeRemoved = "removed"
	// ChangeChanged - занятие осталось, но его подробности разошлись.
	ChangeChanged = "changed"
)

// EventChange - одно изменение в расписании недели.
type EventChange struct {
	Kind   string   // Вид изменения: added, removed или changed
	Fields []string // Разошедшиеся поля, только у changed: room, start, ...
	Before *Event   // Занятие до изменения, nil у added
	After  *Event   // Занятие после изменения, nil у removed
}

// WeekChanges - разница в расписании недели, зафиксированная одним прогоном
// воркера. Один документ - одно будущее уведомление.
type WeekChanges struct {
	ID         string        // Идентификатор документа, пустой до сохранения
	Group      string        // Академическая группа
	WeekStart  string        // Понедельник недели, ГГГГ-ММ-ДД
	DetectedAt time.Time     // Когда воркер заметил разницу
	Changes    []EventChange // Сами изменения
	// NotifiedAt - когда разослано уведомление. nil - ждёт рассылки
	NotifiedAt *time.Time
}
