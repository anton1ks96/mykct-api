package service

// GetScoresInput - студент, предмет и период для выдачи оценок.
type GetScoresInput struct {
	Login string // Логин студента из токена: i24s0291
	SuID  string // Код предмета: СГ.02
	Start string // Начало периода, ГГГГ-ММ-ДД
	End   string // Конец периода, ГГГГ-ММ-ДД
}
