package service

// GetAttendanceInput - студент и период для выдачи посещаемости.
type GetAttendanceInput struct {
	Login string // Логин студента из токена: i24s0291
	Start string // Начало периода, ГГГГ-ММ-ДД
	End   string // Конец периода, ГГГГ-ММ-ДД
}

// GetStreakInput - студент, чью серию считать. Группа нужна не для расчёта, а
// чтобы попутно освежить его строку в рейтинге; у не-студента она пустая.
type GetStreakInput struct {
	Login         string // Логин студента из токена: i24s0291
	AcademicGroup string // Академическая группа из токена: ИТ25-11
}

// GetLeaderboardInput - студент, которому отдаётся рейтинг его курса.
type GetLeaderboardInput struct {
	Login         string // Логин студента из токена: i24s0291
	AcademicGroup string // Академическая группа из токена: ИТ25-11
}
