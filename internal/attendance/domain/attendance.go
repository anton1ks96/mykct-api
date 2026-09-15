// Package domain содержит доменные модели и ошибки модуля посещаемости.
package domain

// Отметки посещаемости портала. Любое другое значение, включая 0, - пропуск
// без уважительной причины.
const (
	// StatusExcused - пропуск по уважительной причине.
	StatusExcused = 1
	// StatusPresent - студент был на занятии.
	StatusPresent = 2
)

// SubGroup - вариант занятия для подгруппы студента.
type SubGroup struct {
	SClID  int    // Идентификатор занятия подгруппы, по нему запрашиваются детали
	SCaID  string // Аудитория подгруппы
	STopic string // Тема занятия подгруппы
	STitle string // Название предмета для подгруппы
}

// Record - занятие студента с отметкой посещаемости.
type Record struct {
	ClID     int        // Идентификатор занятия
	Day      string     // Дата занятия, ГГГГ-ММ-ДД
	Topic    string     // Тема занятия
	Start    string     // Время начала, ЧЧ:ММ
	End      string     // Время окончания, ЧЧ:ММ
	Room     string     // Аудитория
	Status   int        // Отметка: StatusExcused, StatusPresent или пропуск
	Title    string     // Название предмета
	Color    string     // Цвет отметки: green, red
	Type     string     // Тип занятия, портал заполняет не всегда
	SubGroup []SubGroup // Варианты занятия по подгруппам
}

// Streak - серия посещений и сводка за период. Учебный день засчитан, если
// студент был хотя бы на одном занятии; дни только с уважительными пропусками
// не учитываются вовсе.
type Streak struct {
	CurrentStreak     int     // Дней подряд с посещением, считая от последнего учебного дня
	LongestStreak     int     // Самая длинная серия за период
	TotalDaysAttended int     // Учебных дней с посещением
	TotalSchoolDays   int     // Учебных дней за период
	AttendanceRate    float64 // Доля дней с посещением, от 0 до 1
	LastAttendedDate  string  // Последний день с посещением, ГГГГ-ММ-ДД; пусто, если не было
	PeriodStart       string  // Начало периода, ГГГГ-ММ-ДД
	PeriodEnd         string  // Конец периода, ГГГГ-ММ-ДД
}
