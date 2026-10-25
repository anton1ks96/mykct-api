package middleware

import (
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
)

// FeatureGate отвечает 503 FEATURE_DISABLED с message, если фича выключена.
// Маршрут остаётся зарегистрированным, чтобы клиент отличал выключение от 404.
func FeatureGate(enabled bool, message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				httpapi.NewError(httpapi.CodeFeatureDisabled, message))
			return
		}
		c.Next()
	}
}
