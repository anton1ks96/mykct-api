// Package middleware предоставляет HTTP middleware: логирование, recovery,
// CORS и rate limiting.
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var log = logger.ComponentLogger("middleware.http")

// Logging логирует запросы и кладёт request_id в контекст и заголовок ответа.
func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		requestID := uuid.New().String()
		c.Header("X-Request-ID", requestID)

		// В контекст запроса - чтобы нижележащие слои писали тот же request_id
		ctx := context.WithValue(c.Request.Context(), logger.RequestIDKey, requestID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		log.Info().
			Str("request_id", requestID).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Str("client_ip", c.ClientIP()).
			Msg("http request")
	}
}

// Recovery перехватывает паники, логирует их и отвечает 500.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Error().
					Interface("panic", err).
					Str("method", c.Request.Method).
					Str("path", c.Request.URL.Path).
					Msg("panic recovered")

				c.AbortWithStatusJSON(http.StatusInternalServerError, httpapi.APIError{
					Code:    http.StatusInternalServerError,
					Message: "internal server error",
				})
			}
		}()
		c.Next()
	}
}
