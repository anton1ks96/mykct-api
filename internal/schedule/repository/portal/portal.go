// Package portal реализует клиент портала колледжа - первоисточника расписания.
package portal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/internal/schedule/repository"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// Соответствие интерфейсу проверяется на этапе компиляции.
var _ repository.Portal = (*Client)(nil)

var log = logger.ComponentLogger("schedule.portal")

// Пути сервисов портала.
const (
	schedulePath     = "/Services/schedule25.php"
	classDetailsPath = "/Services/classdetails25.php"
)

// allSubgroups - портал всегда запрашивается со всеми подгруппами, отбор нужной
// делает сервис. Благодаря этому ответ зависит только от группы и периода.
const allSubgroups = "*"

// scheduleRequest - тело запроса расписания к порталу.
type scheduleRequest struct {
	DStart   string `json:"d_start"`
	DEnd     string `json:"d_end"`
	Group    string `json:"group"`
	Subgroup string `json:"subgroup"`
}

// portalSubGroup - подгруппа занятия в ответе портала.
type portalSubGroup struct {
	SClID  string `json:"SClID"`
	SGrID  string `json:"SGrID"`
	SGCaID string `json:"SGCaID"`
	STopic string `json:"STopic"`
	STitle string `json:"STitle"`
}

// portalEvent - занятие в ответе портала.
type portalEvent struct {
	ClID     string           `json:"ClID"`
	Type     string           `json:"type"`
	Day      string           `json:"Day"`
	Group    string           `json:"group"`
	Topic    string           `json:"topic"`
	Start    string           `json:"start"`
	End      string           `json:"end"`
	Room     string           `json:"room"`
	Color    string           `json:"color"`
	Title    string           `json:"title"`
	SubGroup []portalSubGroup `json:"SubGroup"`
}

// toDomain переводит занятие портала в доменную модель, попутно приводя время к
// единому виду.
func (e *portalEvent) toDomain() domain.Event {
	event := domain.Event{
		ClID:  e.ClID,
		Type:  e.Type,
		Day:   e.Day,
		Group: e.Group,
		Topic: e.Topic,
		Start: normalizeTime(e.Start),
		End:   normalizeTime(e.End),
		Room:  e.Room,
		Color: e.Color,
		Title: e.Title,
	}

	if len(e.SubGroup) > 0 {
		event.SubGroup = make([]domain.SubGroup, 0, len(e.SubGroup))
		for _, sg := range e.SubGroup {
			event.SubGroup = append(event.SubGroup, domain.SubGroup{
				SClID:  sg.SClID,
				SGrID:  sg.SGrID,
				SGCaID: sg.SGCaID,
				STopic: sg.STopic,
				STitle: sg.STitle,
			})
		}
	}

	return event
}

// Client ходит за расписанием на портал колледжа.
type Client struct {
	client  *http.Client
	baseURL string
}

// NewClient создаёт клиент портала. Сертификат портала не проверяется, как и в
// сервисе, из которого перенесено расписание.
func NewClient(cfg config.ScheduleConfig) *Client {
	return &Client{
		client: &http.Client{
			Timeout: cfg.PortalTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		baseURL: strings.TrimSuffix(cfg.PortalURL, "/"),
	}
}

// FetchSchedule возвращает занятия группы за период без фильтрации по подгруппам.
func (c *Client) FetchSchedule(ctx context.Context, group, start, end string) ([]domain.Event, error) {
	op := logger.NewLogOp(ctx, log, "FetchSchedule")

	var portalEvents []portalEvent
	err := c.post(ctx, schedulePath, scheduleRequest{
		DStart:   start,
		DEnd:     end,
		Group:    group,
		Subgroup: allSubgroups,
	}, &portalEvents)
	if err != nil {
		op.Warn().Err(err).Str("group", group).Msg("failed to fetch schedule from portal")
		return nil, err
	}

	// Нулевой слайс сохраняем как есть: портал так отвечает на пустой период
	var events []domain.Event
	if portalEvents != nil {
		events = make([]domain.Event, 0, len(portalEvents))
		for i := range portalEvents {
			events = append(events, portalEvents[i].toDomain())
		}
	}

	op.Completed().Str("group", group).Int("events", len(events)).
		Msg("schedule fetched from portal")

	return events, nil
}

// FetchClassDetails возвращает произвольный JSON с деталями занятия.
func (c *Client) FetchClassDetails(ctx context.Context, clid string) (map[string]any, error) {
	op := logger.NewLogOp(ctx, log, "FetchClassDetails")

	var details map[string]any
	if err := c.post(ctx, classDetailsPath, map[string]string{"clid": clid}, &details); err != nil {
		op.Warn().Err(err).Str("clid", clid).Msg("failed to fetch class details from portal")
		return nil, err
	}

	op.Completed().Str("clid", clid).Msg("class details fetched from portal")

	return details, nil
}

// post отправляет JSON на сервис портала и разбирает ответ в dst. Любой сбой
// транспорта или разбора превращается в ErrPortalUnavailable: причина остаётся в
// логе, наружу уходит только сентинел.
func (c *Client) post(ctx context.Context, path string, payload, dst any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal portal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build portal request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrPortalUnavailable, err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrPortalUnavailable, err)
	}

	return nil
}

// normalizeTime приводит время портала к ЧЧ:ММ: портал отдаёт то
// "2026-09-01 9:00", то "9:00". Непонятное значение возвращается как есть.
func normalizeTime(value string) string {
	value = strings.TrimSpace(value)

	// Дата и время приходят одной строкой, нужна только вторая половина
	if index := strings.LastIndex(value, " "); index >= 0 {
		value = value[index+1:]
	}

	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return value
	}

	hour, minute := parts[0], parts[1]
	if len(hour) == 1 {
		hour = "0" + hour
	}
	if len(minute) == 1 {
		minute = "0" + minute
	}

	return hour + ":" + minute
}
