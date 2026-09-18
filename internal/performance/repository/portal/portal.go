// Package portal реализует клиент портала колледжа - первоисточника успеваемости.
package portal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/anton1ks96/mykct-api/internal/performance/domain"
	"github.com/anton1ks96/mykct-api/internal/performance/repository"
	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// Соответствие интерфейсу проверяется на этапе компиляции.
var _ repository.Portal = (*Client)(nil)

var log = logger.ComponentLogger("performance.portal")

// Пути сервисов портала.
const (
	subjectsPath = "/Services/subjectsP-25.php"
	scorePath    = "/Services/scoreP-25.php"
)

// scoreRequest - тело запроса оценок к порталу.
type scoreRequest struct {
	SuID      string `json:"SuID"`
	DataStart string `json:"datastart"`
	DataEnd   string `json:"dataend"`
}

// portalSubject - предмет в ответе портала.
type portalSubject struct {
	SuIDcrc string `json:"SuIDcrc"`
	SuID    string `json:"SuID"`
	Title   string `json:"Title"`
}

// portalScore - оценка в ответе портала. Даты портал отдаёт и как null: в
// строковое поле null разбирается пустой строкой.
type portalScore struct {
	DateF       string `json:"DateF"`
	DateP       string `json:"DateP"`
	Score       string `json:"Score"`
	MaxScore    int    `json:"MaxScore"`
	Description string `json:"Description"`
}

// Client ходит за успеваемостью на портал колледжа.
type Client struct {
	client  *http.Client
	baseURL string
}

// NewClient создаёт клиент портала. Сертификат портала не проверяется, как и в
// клиенте расписания.
func NewClient(cfg config.PerformanceConfig) *Client {
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

// FetchSubjects возвращает предметы студента в текущем семестре.
func (c *Client) FetchSubjects(ctx context.Context, login string) ([]domain.Subject, error) {
	op := logger.NewLogOp(ctx, log, "FetchSubjects")

	var portalSubjects []portalSubject
	if err := c.do(ctx, http.MethodGet, subjectsPath, login, nil, &portalSubjects); err != nil {
		op.Warn().Err(err).Str("login", login).Msg("failed to fetch subjects from portal")
		return nil, err
	}

	subjects := make([]domain.Subject, 0, len(portalSubjects))
	for _, s := range portalSubjects {
		subjects = append(subjects, domain.Subject{SuIDcrc: s.SuIDcrc, SuID: s.SuID, Title: s.Title})
	}

	op.Completed().Str("login", login).Int("subjects", len(subjects)).
		Msg("subjects fetched from portal")

	return subjects, nil
}

// FetchScores возвращает оценки студента по предмету за период.
func (c *Client) FetchScores(ctx context.Context, login, suID, start, end string) (domain.Scores, error) {
	op := logger.NewLogOp(ctx, log, "FetchScores")

	var raw json.RawMessage
	err := c.do(ctx, http.MethodPost, scorePath, login, scoreRequest{
		SuID:      suID,
		DataStart: start,
		DataEnd:   end,
	}, &raw)
	var scores domain.Scores
	if err == nil {
		scores, err = decodeScores(raw)
	}
	if err != nil {
		op.Warn().Err(err).Str("login", login).Str("suid", suID).
			Msg("failed to fetch scores from portal")
		return nil, err
	}

	op.Completed().Str("login", login).Str("suid", suID).Int("subjects", len(scores)).
		Msg("scores fetched from portal")

	return scores, nil
}

// decodeScores разбирает оценки портала. Пустую карту PHP кодирует массивом [],
// поэтому пустой массив - это «оценок нет», а не сбой.
func decodeScores(raw json.RawMessage) (domain.Scores, error) {
	var empty []json.RawMessage
	if json.Unmarshal(raw, &empty) == nil && len(empty) == 0 {
		return domain.Scores{}, nil
	}

	var portalScores map[string]map[string][]portalScore
	if err := json.Unmarshal(raw, &portalScores); err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrPortalUnavailable, err)
	}

	scores := make(domain.Scores, len(portalScores))
	for suIDcrc, lessons := range portalScores {
		byLesson := make(map[string][]domain.Score, len(lessons))
		for lesson, items := range lessons {
			out := make([]domain.Score, 0, len(items))
			for _, s := range items {
				out = append(out, domain.Score{
					DateF:       s.DateF,
					DateP:       s.DateP,
					Score:       s.Score,
					MaxScore:    s.MaxScore,
					Description: s.Description,
				})
			}
			byLesson[lesson] = out
		}
		scores[suIDcrc] = byLesson
	}

	return scores, nil
}

// do отправляет запрос на сервис портала от имени студента и разбирает ответ в
// dst. Портал узнаёт студента по логину в cookie сессии, пароль не нужен. Сбой
// транспорта или разбора превращается в ErrPortalUnavailable: на ошибку портал
// отвечает 200 с текстом вместо JSON.
func (c *Client) do(ctx context.Context, method, path, login string, payload, dst any) error {
	var body io.Reader = http.NoBody
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal portal request: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("failed to build portal request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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
