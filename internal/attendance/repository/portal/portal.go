// Package portal реализует клиент портала колледжа - первоисточника посещаемости.
package portal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/internal/attendance/repository"
	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// Соответствие интерфейсу проверяется на этапе компиляции.
var _ repository.Portal = (*Client)(nil)

var log = logger.ComponentLogger("attendance.portal")

// attendancePath - сервис посещаемости на портале.
const attendancePath = "/Services/attnP25.php"

// attendanceRequest - тело запроса посещаемости к порталу.
type attendanceRequest struct {
	DStart string `json:"d_start"`
	DEnd   string `json:"d_end"`
}

// portalSubGroup - подгруппа занятия в ответе портала.
type portalSubGroup struct {
	SClID  int    `json:"SClID"`
	SCaID  string `json:"SCaID"`
	STopic string `json:"STopic"`
	STitle string `json:"STitle"`
}

// portalRecord - занятие с отметкой посещаемости в ответе портала.
type portalRecord struct {
	ClID     int              `json:"ClID"`
	Day      string           `json:"Day"`
	Topic    string           `json:"topic"`
	Start    string           `json:"start"`
	End      string           `json:"end"`
	Room     string           `json:"room"`
	Status   int              `json:"status"`
	Title    string           `json:"title"`
	Color    string           `json:"color"`
	Type     string           `json:"type"`
	SubGroup []portalSubGroup `json:"SubGroup"`
}

// toDomain переводит занятие портала в доменную модель, попутно приводя время к
// единому виду.
func (r *portalRecord) toDomain() domain.Record {
	record := domain.Record{
		ClID:   r.ClID,
		Day:    r.Day,
		Topic:  r.Topic,
		Start:  normalizeTime(r.Start),
		End:    normalizeTime(r.End),
		Room:   r.Room,
		Status: r.Status,
		Title:  r.Title,
		Color:  r.Color,
		Type:   r.Type,
	}

	if len(r.SubGroup) > 0 {
		record.SubGroup = make([]domain.SubGroup, 0, len(r.SubGroup))
		for _, sg := range r.SubGroup {
			record.SubGroup = append(record.SubGroup, domain.SubGroup{
				SClID:  sg.SClID,
				SCaID:  sg.SCaID,
				STopic: sg.STopic,
				STitle: sg.STitle,
			})
		}
	}

	return record
}

// Client ходит за посещаемостью на портал колледжа.
type Client struct {
	client  *http.Client
	baseURL string
}

// NewClient создаёт клиент портала. Сертификат портала не проверяется, как и в
// клиенте расписания.
func NewClient(cfg config.AttendanceConfig) *Client {
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

// FetchAttendance возвращает занятия студента за период с отметками посещаемости.
func (c *Client) FetchAttendance(ctx context.Context, login, start, end string) ([]domain.Record, error) {
	op := logger.NewLogOp(ctx, log, "FetchAttendance")

	var portalRecords []portalRecord
	err := c.post(ctx, login, attendanceRequest{DStart: start, DEnd: end}, &portalRecords)
	if err != nil {
		op.Warn().Err(err).Str("login", login).Msg("failed to fetch attendance from portal")
		return nil, err
	}

	var records []domain.Record
	if portalRecords != nil {
		records = make([]domain.Record, 0, len(portalRecords))
		for i := range portalRecords {
			records = append(records, portalRecords[i].toDomain())
		}
	}

	op.Completed().Str("login", login).Int("records", len(records)).
		Msg("attendance fetched from portal")

	return records, nil
}

// post отправляет JSON на сервис посещаемости от имени студента и разбирает ответ
// в dst. Портал узнаёт студента по логину в cookie сессии, пароль не нужен. Сбой
// транспорта или разбора превращается в ErrPortalUnavailable: на ошибку портал
// отвечает 200 с текстом вместо JSON.
func (c *Client) post(ctx context.Context, login string, payload, dst any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal portal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+attendancePath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build portal request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "STDNT-login-user=" + login})

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
