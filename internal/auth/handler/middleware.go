package handler

import (
	"net/http"
	"strings"

	"github.com/anton1ks96/mykct-api/internal/auth/authctx"
	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
)

// bearerPrefix - схема заголовка Authorization.
const bearerPrefix = "Bearer "

// Auth проверяет access-токен из заголовка Authorization и кладёт профиль
// пользователя в контекст запроса. Вешается на маршруты других модулей,
// профиль читается через authctx.From.
func (h *Handler) Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpapi.NewError(
				httpapi.CodeUnauthorized,
				"Требуется авторизация",
			))
			return
		}

		user, err := h.service.ValidateAccessToken(c.Request.Context(), token)
		if err != nil {
			abortWithDomainError(c, err)
			return
		}

		authctx.Set(c, user)

		c.Next()
	}
}

// bearerToken достаёт токен из заголовка Authorization.
func bearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if len(header) <= len(bearerPrefix) || !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}

	return header[len(bearerPrefix):], true
}
