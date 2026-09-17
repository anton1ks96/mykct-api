package handler

import (
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// scheduleRequest - параметры запроса расписания из query-строки.
type scheduleRequest struct {
	Group           string `form:"group" binding:"required"`
	Start           string `form:"start" binding:"required,datetime=2006-01-02"`
	End             string `form:"end" binding:"required,datetime=2006-01-02"`
	Subgroup        string `form:"subgroup"`
	EnglishGroup    string `form:"english_group"`
	ProfileSubgroup string `form:"profile_subgroup"`
}

// classDetailsRequest - идентификатор занятия, совпадает с ClID в расписании.
type classDetailsRequest struct {
	ID string `form:"id" binding:"required"`
}

// scheduleSubGroup - подгруппа занятия. Имена полей заданы порталом, клиенты
// разбирают ответ по ним, менять раскладку регистра нельзя.
type scheduleSubGroup struct {
	SClID  string `json:"SClID"`
	SGrID  string `json:"SGrID"`
	SGCaID string `json:"SGCaID"`
	STopic string `json:"STopic"`
	STitle string `json:"STitle"`
}

// scheduleEvent - занятие в ответе.
type scheduleEvent struct {
	ClID     string             `json:"ClID"`
	Type     string             `json:"type,omitempty"`
	Day      string             `json:"Day"`
	Group    string             `json:"group"`
	Topic    string             `json:"topic"`
	Start    string             `json:"start"`
	End      string             `json:"end"`
	Room     string             `json:"room"`
	Color    string             `json:"color"`
	Title    string             `json:"title"`
	SubGroup []scheduleSubGroup `json:"SubGroup,omitempty"`
}

// scheduleResponse - расписание вместе с происхождением данных: по source и
// stale клиент понимает, что портал колледжа сейчас недоступен, а по fetched_at
// показывает время последней успешной связи с ним.
type scheduleResponse struct {
	Events    []scheduleEvent `json:"events"`
	Source    string          `json:"source"`
	FetchedAt string          `json:"fetched_at"`
	Stale     bool            `json:"stale"`
}

// newScheduleEvents переводит доменные занятия в DTO ответа. Нулевой слайс
// сохраняется: клиенты рассчитывают на такой ответ по пустому периоду.
func newScheduleEvents(events []domain.Event) []scheduleEvent {
	if events == nil {
		return nil
	}

	out := make([]scheduleEvent, 0, len(events))
	for _, e := range events {
		event := scheduleEvent{
			ClID:  e.ClID,
			Type:  e.Type,
			Day:   e.Day,
			Group: e.Group,
			Topic: e.Topic,
			Start: e.Start,
			End:   e.End,
			Room:  e.Room,
			Color: e.Color,
			Title: e.Title,
		}
		if len(e.SubGroup) > 0 {
			event.SubGroup = make([]scheduleSubGroup, 0, len(e.SubGroup))
			for _, sg := range e.SubGroup {
				event.SubGroup = append(event.SubGroup, scheduleSubGroup{
					SClID:  sg.SClID,
					SGrID:  sg.SGrID,
					SGCaID: sg.SGCaID,
					STopic: sg.STopic,
					STitle: sg.STitle,
				})
			}
		}
		out = append(out, event)
	}

	return out
}

// formatFetchedAt переводит время получения данных в RFC3339 UTC.
func formatFetchedAt(fetchedAt time.Time) string {
	return fetchedAt.UTC().Format(time.RFC3339)
}

// nextWeekRequest - параметры запроса состояния следующей недели.
type nextWeekRequest struct {
	Group string `form:"group" binding:"required"`
}

// nextWeekResponse - состояние следующей недели. published_at == null при
// published == true означает, что неделя была заполнена ещё до того, как сервис
// начал следить за группой: уведомление по ней не рассылалось.
type nextWeekResponse struct {
	Group       string  `json:"group"`
	WeekStart   string  `json:"week_start"`
	WeekEnd     string  `json:"week_end"`
	Published   bool    `json:"published"`
	PublishedAt *string `json:"published_at"`
	EventsCount int     `json:"events_count"`
	CheckedAt   string  `json:"checked_at"`
}

// newNextWeekResponse собирает ответ из состояния недели.
func newNextWeekResponse(state *domain.WeekState) nextWeekResponse {
	out := nextWeekResponse{
		Group:       state.Group,
		WeekStart:   state.WeekStart,
		WeekEnd:     state.WeekEnd,
		Published:   state.Published,
		EventsCount: state.EventsCount,
		CheckedAt:   formatFetchedAt(state.LastCheckedAt),
	}

	if state.PublishedAt != nil {
		publishedAt := formatFetchedAt(*state.PublishedAt)
		out.PublishedAt = &publishedAt
	}

	return out
}
