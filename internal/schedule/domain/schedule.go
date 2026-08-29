// Package domain содержит доменные модели и ошибки модуля расписания.
package domain

import "time"

// Источники расписания в ответе API.
const (
	// SourceLive - расписание только что получено от портала колледжа.
	SourceLive = "live"
	// SourceCache - портал недоступен, отдан сохранённый снимок.
	SourceCache = "cache"
)

// SubGroup - подгруппа занятия: у одной пары бывает несколько вариантов для
// разных подгрупп, английских групп или спортивных секций.
type SubGroup struct {
	SClID  string // Идентификатор занятия подгруппы
	SGrID  string // Имя подгруппы: Подгр1, A0.11, ФизраКол
	SGCaID string // Аудитория подгруппы
	STopic string // Тема занятия подгруппы
	STitle string // Название предмета для подгруппы
}

// Event - занятие в расписании группы.
type Event struct {
	ClID     string     // Идентификатор занятия, по нему запрашиваются детали
	Type     string     // Тип занятия, портал заполняет не всегда
	Day      string     // Дата занятия, ГГГГ-ММ-ДД
	Group    string     // Академическая группа
	Topic    string     // Тема занятия
	Start    string     // Время начала, ЧЧ:ММ
	End      string     // Время окончания, ЧЧ:ММ
	Room     string     // Аудитория
	Color    string     // Цвет предмета, #RRGGBB
	Title    string     // Название предмета
	SubGroup []SubGroup // Варианты занятия по подгруппам
}

// Snapshot - сохранённый ответ портала за период. Портал всегда запрашивается с
// subgroup=*, поэтому снимок зависит только от группы и периода и обслуживает
// все подгруппы этой группы.
type Snapshot struct {
	Group     string    // Академическая группа
	Start     string    // Начало периода, ГГГГ-ММ-ДД
	End       string    // Конец периода, ГГГГ-ММ-ДД
	Events    []Event   // Занятия без фильтрации по подгруппам
	FetchedAt time.Time // Когда снимок получен от портала
}

// ClassDetails - сохранённые детали занятия. Портал отдаёт произвольный JSON,
// поэтому схема не фиксируется.
type ClassDetails struct {
	ClID      string         // Идентификатор занятия
	Details   map[string]any // Тело ответа портала как есть
	FetchedAt time.Time      // Когда детали получены от портала
}
