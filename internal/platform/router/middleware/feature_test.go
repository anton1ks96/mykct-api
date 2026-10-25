package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFeatureGate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		enabled bool
		status  int
	}{
		{enabled: true, status: http.StatusOK},
		{enabled: false, status: http.StatusServiceUnavailable},
	} {
		r := gin.New()
		r.GET("/x", FeatureGate(tc.enabled, "Отключено"), func(c *gin.Context) { c.Status(http.StatusOK) })

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

		if w.Code != tc.status {
			t.Fatalf("enabled=%v: status %d, want %d", tc.enabled, w.Code, tc.status)
		}
		if !tc.enabled && !strings.Contains(w.Body.String(), `"code":"FEATURE_DISABLED"`) {
			t.Fatalf("unexpected body: %s", w.Body.String())
		}
	}
}
