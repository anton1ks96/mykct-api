package portal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/performance/domain"
	"github.com/anton1ks96/mykct-api/internal/platform/config"
)

// wrongData - ответ портала на неизвестного студента: 200 и отладочный текст.
const wrongData = "    \narray(1) {\n  [\"Semestre\"]=>\n  string(4) \"test\"\n}\nWrong data:"

// newTestClient поднимает HTTPS-портал с заданным обработчиком и клиент к нему.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)

	return NewClient(config.PerformanceConfig{PortalURL: srv.URL + "/", PortalTimeout: 5 * time.Second})
}

// respond отвечает телом как портал: с пробелами перед JSON и text/html.
func respond(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	_, _ = w.Write([]byte(body))
}

// checkLogin проверяет, что студент передан в cookie сессии.
func checkLogin(t *testing.T, r *http.Request) {
	t.Helper()

	cookie, err := r.Cookie("session")
	if err != nil {
		t.Errorf("expected session cookie: %v", err)
		return
	}
	if cookie.Value != "STDNT-login-user=i24s0291" {
		t.Errorf("unexpected session cookie %q", cookie.Value)
	}
}

// TestFetchSubjects - предметы запрашиваются GET от имени студента и разбираются.
func TestFetchSubjects(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != subjectsPath {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		checkLogin(t, r)
		respond(w, "    \n"+`[{"SuIDcrc":"subjb4253892","SuID":"СГ.02","Title":"Иностранный язык"}]`)
	})

	subjects, err := client.FetchSubjects(context.Background(), "i24s0291")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := domain.Subject{SuIDcrc: "subjb4253892", SuID: "СГ.02", Title: "Иностранный язык"}
	if len(subjects) != 1 || subjects[0] != want {
		t.Errorf("expected %+v, got %+v", want, subjects)
	}
}

// TestFetchSubjectsNull - null от портала - пустой список, а не nil.
func TestFetchSubjectsNull(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, "null")
	})

	subjects, err := client.FetchSubjects(context.Background(), "i24s0291")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subjects == nil || len(subjects) != 0 {
		t.Errorf("expected empty non-nil slice, got %#v", subjects)
	}
}

// TestFetchScores - оценки запрашиваются POST с периодом, а null в дате
// разбирается пустой строкой.
func TestFetchScores(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != scorePath {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content type %q", ct)
		}
		checkLogin(t, r)

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		want := map[string]string{"SuID": "СГ.02", "datastart": "2025-09-01", "dataend": "2026-06-30"}
		for key, value := range want {
			if body[key] != value {
				t.Errorf("expected %s=%q, got %q", key, value, body[key])
			}
		}

		respond(w, "    \n"+`{"subjb4253892":{"Intro":[`+
			`{"DateF":"2025-09-12","DateP":"2025-09-12","Score":"10","MaxScore":20,"Description":"Introduction"},`+
			`{"DateF":null,"DateP":"2025-10-03","Score":"","MaxScore":20,"Description":"Gadgets"}]}}`)
	})

	scores, err := client.FetchScores(context.Background(), "i24s0291", "СГ.02", "2025-09-01", "2026-06-30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lesson := scores["subjb4253892"]["Intro"]
	if len(lesson) != 2 {
		t.Fatalf("expected 2 scores in lesson, got %+v", scores)
	}
	want := domain.Score{DateF: "2025-09-12", DateP: "2025-09-12", Score: "10", MaxScore: 20, Description: "Introduction"}
	if lesson[0] != want {
		t.Errorf("expected %+v, got %+v", want, lesson[0])
	}
	if lesson[1].DateF != "" || lesson[1].DateP != "2025-10-03" || lesson[1].Score != "" {
		t.Errorf("expected empty DateF and Score, got %+v", lesson[1])
	}
}

// TestFetchScoresEmpty - пустую карту PHP отдаёт массивом, это не ошибка.
func TestFetchScoresEmpty(t *testing.T) {
	for _, body := range []string{"    \n[]", "[ ]"} {
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			respond(w, body)
		})

		scores, err := client.FetchScores(context.Background(), "i24s0291", "СГ.02", "2026-09-01", "2026-12-31")
		if err != nil {
			t.Fatalf("body %q: unexpected error: %v", body, err)
		}
		if scores == nil || len(scores) != 0 {
			t.Errorf("body %q: expected empty non-nil map, got %#v", body, scores)
		}
	}
}

// TestPortalTextResponse - текст вместо JSON означает, что портал не отдал
// успеваемость.
func TestPortalTextResponse(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, wrongData)
	})

	if _, err := client.FetchSubjects(context.Background(), "nobody"); !errors.Is(err, domain.ErrPortalUnavailable) {
		t.Errorf("subjects: expected ErrPortalUnavailable, got %v", err)
	}
	if _, err := client.FetchScores(context.Background(), "nobody", "СГ.02", "2025-09-01", "2026-06-30"); !errors.Is(err, domain.ErrPortalUnavailable) {
		t.Errorf("scores: expected ErrPortalUnavailable, got %v", err)
	}
}

// TestPortalUnreachable - недоступный портал - ErrPortalUnavailable.
func TestPortalUnreachable(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	srv.Close()
	client := NewClient(config.PerformanceConfig{PortalURL: srv.URL, PortalTimeout: time.Second})

	if _, err := client.FetchSubjects(context.Background(), "i24s0291"); !errors.Is(err, domain.ErrPortalUnavailable) {
		t.Errorf("subjects: expected ErrPortalUnavailable, got %v", err)
	}
	if _, err := client.FetchScores(context.Background(), "i24s0291", "СГ.02", "2025-09-01", "2026-06-30"); !errors.Is(err, domain.ErrPortalUnavailable) {
		t.Errorf("scores: expected ErrPortalUnavailable, got %v", err)
	}
}
