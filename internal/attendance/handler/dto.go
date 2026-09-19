package handler

import "github.com/anton1ks96/mykct-api/internal/attendance/domain"

// attendanceRequest - период посещаемости из query-строки.
type attendanceRequest struct {
	Start string `form:"start" binding:"required,datetime=2006-01-02"`
	End   string `form:"end" binding:"required,datetime=2006-01-02"`
}

// attendanceSubGroup - подгруппа занятия. Имена полей заданы порталом, клиенты
// разбирают ответ по ним, менять раскладку регистра нельзя.
type attendanceSubGroup struct {
	SClID  int    `json:"SClID"`
	SCaID  string `json:"SCaID"`
	STopic string `json:"STopic"`
	STitle string `json:"STitle"`
}

// attendanceRecord - занятие с отметкой посещаемости в ответе.
type attendanceRecord struct {
	ClID     int                  `json:"ClID"`
	Day      string               `json:"Day"`
	Topic    string               `json:"topic"`
	Start    string               `json:"start"`
	End      string               `json:"end"`
	Room     string               `json:"room"`
	Status   int                  `json:"status"`
	Title    string               `json:"title"`
	Color    string               `json:"color"`
	Type     string               `json:"type,omitempty"`
	SubGroup []attendanceSubGroup `json:"SubGroup,omitempty"`
}

// streakResponse - серия посещений и сводка с начала учебного года.
type streakResponse struct {
	CurrentStreak     int     `json:"current_streak"`
	LongestStreak     int     `json:"longest_streak"`
	TotalDaysAttended int     `json:"total_days_attended"`
	TotalSchoolDays   int     `json:"total_school_days"`
	AttendanceRate    float64 `json:"attendance_rate"`
	LastAttendedDate  string  `json:"last_attended_date,omitempty"`
	PeriodStart       string  `json:"period_start"`
	PeriodEnd         string  `json:"period_end"`
}

// newAttendanceRecords переводит доменные занятия в DTO ответа. Ответ - голый
// массив, поэтому пустой период отдаётся как [], а не null.
func newAttendanceRecords(records []domain.Record) []attendanceRecord {
	out := make([]attendanceRecord, 0, len(records))
	for _, r := range records {
		record := attendanceRecord{
			ClID:   r.ClID,
			Day:    r.Day,
			Topic:  r.Topic,
			Start:  r.Start,
			End:    r.End,
			Room:   r.Room,
			Status: r.Status,
			Title:  r.Title,
			Color:  r.Color,
			Type:   r.Type,
		}
		if len(r.SubGroup) > 0 {
			record.SubGroup = make([]attendanceSubGroup, 0, len(r.SubGroup))
			for _, sg := range r.SubGroup {
				record.SubGroup = append(record.SubGroup, attendanceSubGroup{
					SClID:  sg.SClID,
					SCaID:  sg.SCaID,
					STopic: sg.STopic,
					STitle: sg.STitle,
				})
			}
		}
		out = append(out, record)
	}

	return out
}

// newStreakResponse переводит серию посещений в DTO ответа.
func newStreakResponse(s *domain.Streak) streakResponse {
	return streakResponse{
		CurrentStreak:     s.CurrentStreak,
		LongestStreak:     s.LongestStreak,
		TotalDaysAttended: s.TotalDaysAttended,
		TotalSchoolDays:   s.TotalSchoolDays,
		AttendanceRate:    s.AttendanceRate,
		LastAttendedDate:  s.LastAttendedDate,
		PeriodStart:       s.PeriodStart,
		PeriodEnd:         s.PeriodEnd,
	}
}

// leaderboardEntry - строка анонимного рейтинга. Здесь не должно появиться ни
// логина, ни ФИО, ни группы, ни дат: каждое такое поле - зацепка, по которой
// псевдоним сопоставляется с человеком, а одной даты последнего посещения
// хватает, чтобы разобрать половину таблицы.
type leaderboardEntry struct {
	Rank          int    `json:"rank"` // Место, спортивное: равные серии делят одно
	Alias         string `json:"alias"`
	CurrentStreak int    `json:"current_streak"`
	IsMe          bool   `json:"is_me"`
}

// leaderboardResponse - топ курса, собственная строка студента и размер когорты.
type leaderboardResponse struct {
	Top          []leaderboardEntry `json:"top"`
	Me           leaderboardEntry   `json:"me"`
	Participants int                `json:"participants"`
}

// newLeaderboardEntry переводит строку рейтинга в DTO ответа.
func newLeaderboardEntry(e domain.Entry) leaderboardEntry {
	return leaderboardEntry{
		Rank:          e.Rank,
		Alias:         e.Alias,
		CurrentStreak: e.CurrentStreak,
		IsMe:          e.IsMe,
	}
}

// newLeaderboardResponse переводит рейтинг в DTO ответа. Топ - массив, поэтому
// пустой отдаётся как [], а не null.
func newLeaderboardResponse(board *domain.Leaderboard) leaderboardResponse {
	top := make([]leaderboardEntry, 0, len(board.Top))
	for _, entry := range board.Top {
		top = append(top, newLeaderboardEntry(entry))
	}

	return leaderboardResponse{
		Top:          top,
		Me:           newLeaderboardEntry(board.Me),
		Participants: board.Participants,
	}
}
