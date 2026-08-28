// Package authctx переносит профиль аутентифицированного пользователя через
// контекст gin. Модулям-потребителям хватает этого пакета: заглядывать во
// внутренности модуля аутентификации им не нужно.
package authctx

import (
	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/gin-gonic/gin"
)

// contextKey - ключ профиля пользователя в gin-контексте.
const contextKey = "auth.user"

// Set кладёт профиль пользователя в контекст запроса.
func Set(c *gin.Context, user *domain.UserExtended) {
	c.Set(contextKey, user)
}

// From достаёт профиль пользователя из контекста запроса.
func From(c *gin.Context) (*domain.UserExtended, bool) {
	value, exists := c.Get(contextKey)
	if !exists {
		return nil, false
	}

	user, ok := value.(*domain.UserExtended)

	return user, ok
}

// MustFrom достаёт профиль пользователя и паникует, если маршрут не закрыт
// middleware аутентификации.
func MustFrom(c *gin.Context) *domain.UserExtended {
	user, ok := From(c)
	if !ok {
		panic("authctx: user is missing in context, Auth middleware is probably not applied")
	}

	return user
}
