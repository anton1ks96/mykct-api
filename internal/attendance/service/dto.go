package service

// GetAttendanceInput - студент и период для выдачи посещаемости.
type GetAttendanceInput struct {
	Login string // Логин студента из токена: i24s0291
	Start string // Начало периода, ГГГГ-ММ-ДД
	End   string // Конец периода, ГГГГ-ММ-ДД
}
