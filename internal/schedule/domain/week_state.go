package domain

import "time"

// WeekState - состояние недели расписания для группы: появились ли занятия и
// разослано ли уведомление об этом.
type WeekState struct {
	Group       string // Академическая группа: ИТ25-11
	WeekStart   string // Понедельник недели, ГГГГ-ММ-ДД
	WeekEnd     string // Воскресенье недели, ГГГГ-ММ-ДД
	Published   bool   // Расписание на неделю появилось
	EventsCount int    // Сколько занятий видел последний опрос
	// PublishedAt - когда зафиксирован переход пусто -> непусто. nil при
	// Published: неделю заполнили до того, как мы начали следить за группой
	PublishedAt *time.Time
	// NotifiedAt - когда разослано уведомление. nil - ждёт рассылки
	NotifiedAt    *time.Time
	LastCheckedAt time.Time // Последний успешный опрос портала
}
