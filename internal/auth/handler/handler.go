// Package handler публикует HTTP API модуля аутентификации.
package handler

import (
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/auth/service"
	"github.com/anton1ks96/mykct-api/internal/platform/router/middleware"
	"github.com/gin-gonic/gin"
)

// Handler обслуживает маршруты аутентификации.
type Handler struct {
	service     *service.Service
	rateLimiter *middleware.RateLimiter
}

// NewHandler создаёт хендлер модуля аутентификации.
func NewHandler(svc *service.Service, rateLimiter *middleware.RateLimiter) *Handler {
	return &Handler{service: svc, rateLimiter: rateLimiter}
}

// Init регистрирует маршруты модуля в группе /api/mykct/v1.
func (h *Handler) Init(v1 *gin.RouterGroup) {
	auth := v1.Group("/auth")
	{
		// Подбор пароля упирается в отдельный лимит, более жёсткий, чем общий
		auth.POST("/signin", h.rateLimiter.Limit(5, 10), h.signIn)
		auth.POST("/signout", h.signOut)
		auth.POST("/refresh", h.refreshToken)
		auth.POST("/access", h.getAccessToken)
	}
}

// signIn проверяет учётные данные и выдаёт пару токенов.
func (h *Handler) signIn(c *gin.Context) {
	var req signInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	out, err := h.service.SignIn(c.Request.Context(), service.SignInInput{
		UserID:   req.Username,
		Password: req.Password,
	})
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, signInResponse{
		AccessToken:      out.AccessToken,
		RefreshToken:     out.RefreshToken,
		AccessExpiresIn:  int(out.AccessExpiresIn.Seconds()),
		RefreshExpiresIn: int(out.RefreshExpiresIn.Seconds()),
		User:             newUserInfo(out.User),
	})
}

// signOut отзывает refresh-сессию.
func (h *Handler) signOut(c *gin.Context) {
	var req refreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	if err := h.service.SignOut(c.Request.Context(), req.RefreshToken); err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, signOutResponse{Message: "Вы вышли из аккаунта"})
}

// refreshToken обменивает refresh-токен на новый, продлевая сессию.
func (h *Handler) refreshToken(c *gin.Context) {
	var req refreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	out, err := h.service.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, refreshTokenResponse{
		RefreshToken: out.RefreshToken,
		ExpiresIn:    int(out.ExpiresIn.Seconds()),
	})
}

// getAccessToken выдаёт новый access-токен по refresh-токену.
func (h *Handler) getAccessToken(c *gin.Context) {
	var req refreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	out, err := h.service.GetAccessToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, accessTokenResponse{
		AccessToken: out.AccessToken,
		ExpiresIn:   int(out.ExpiresIn.Seconds()),
		User:        newUserInfo(out.User),
	})
}
