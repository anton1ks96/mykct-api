package domain

import "time"

// Participant - строка постоянного реестра рейтинга. Логин и группа - служебные
// поля расчёта: наружу из них не уходит ничего.
type Participant struct {
	Login             string // Логин студента: i24s0291
	AcademicGroup     string // Академическая группа: ИТ25-11
	Course            string // Курс-поток, префикс группы до дефиса: ИТ25
	CurrentStreak     int    // Текущая серия посещений
	LongestStreak     int    // Самая длинная серия за учебный год
	TotalDaysAttended int    // Дней с посещением
	AttendanceRate    float64
	LastAttendedDate  string
	EmptyRuns         int       // Пустых ответов портала подряд
	Inactive          bool      // Логин погас: из выдачи убран, из реестра нет
	RegisteredAt      time.Time // Когда участник попал в реестр
	UpdatedAt         time.Time // Когда серию считали в последний раз
}

// Entry - строка рейтинга наружу. Больше здесь появиться не должно ничего:
// каждое дополнительное поле - ещё одна зацепка для сопоставления псевдонима с
// человеком. Даты последнего посещения в одиночку хватает, чтобы разобрать
// половину таблицы.
type Entry struct {
	Rank          int    // Место, спортивное: равные серии делят одно
	Alias         string // Псевдоним, выведенный из логина и секрета
	CurrentStreak int
	IsMe          bool // Строка самого запрашивающего
}

// Leaderboard - топ курса и собственная строка студента, даже если он вне топа.
type Leaderboard struct {
	Top          []Entry
	Me           Entry
	Participants int // Размер когорты курса, не размер топа
}
