package handler

import "github.com/anton1ks96/mykct-api/internal/performance/domain"

// scoreRequest - предмет и период оценок. Имена полей заданы college-app-core,
// клиенты шлют именно их.
type scoreRequest struct {
	SuID      string `json:"SuID" binding:"required"`
	DataStart string `json:"datastart" binding:"required,datetime=2006-01-02"`
	DataEnd   string `json:"dataend" binding:"required,datetime=2006-01-02"`
}

// scoreRequestFields - имена полей scoreRequest в JSON для details ошибки валидации.
var scoreRequestFields = map[string]string{
	"SuID":      "SuID",
	"DataStart": "datastart",
	"DataEnd":   "dataend",
}

// subjectResponse - предмет в ответе. Имена полей заданы порталом, клиенты
// разбирают ответ по ним.
type subjectResponse struct {
	SuIDcrc string `json:"SuIDcrc"`
	SuID    string `json:"SuID"`
	Title   string `json:"Title"`
}

// scoreResponse - оценка в ответе. Незаполненная дата уходит как null, как у
// портала: клиент тогда берёт другую дату.
type scoreResponse struct {
	DateF       *string `json:"DateF"`
	DateP       *string `json:"DateP"`
	Score       string  `json:"Score"`
	MaxScore    int     `json:"MaxScore"`
	Description string  `json:"Description"`
}

// newSubjects переводит предметы в DTO ответа. Ответ - голый массив, поэтому
// пустой список отдаётся как [], а не null.
func newSubjects(subjects []domain.Subject) []subjectResponse {
	out := make([]subjectResponse, 0, len(subjects))
	for _, s := range subjects {
		out = append(out, subjectResponse{SuIDcrc: s.SuIDcrc, SuID: s.SuID, Title: s.Title})
	}

	return out
}

// newScores переводит оценки в DTO ответа: хэш предмета -> занятие -> оценки.
// Нет оценок - {}, а не null.
func newScores(scores domain.Scores) map[string]map[string][]scoreResponse {
	out := make(map[string]map[string][]scoreResponse, len(scores))
	for suIDcrc, lessons := range scores {
		byLesson := make(map[string][]scoreResponse, len(lessons))
		for lesson, items := range lessons {
			responses := make([]scoreResponse, 0, len(items))
			for _, s := range items {
				responses = append(responses, scoreResponse{
					DateF:       optional(s.DateF),
					DateP:       optional(s.DateP),
					Score:       s.Score,
					MaxScore:    s.MaxScore,
					Description: s.Description,
				})
			}
			byLesson[lesson] = responses
		}
		out[suIDcrc] = byLesson
	}

	return out
}

// optional превращает пустую строку в null ответа.
func optional(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}
