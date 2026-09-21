// Package handler публикует HTTP API модуля уведомлений.
package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/anton1ks96/mykct-api/internal/auth/authctx"
	"github.com/anton1ks96/mykct-api/internal/notification/service"
	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// Handler обслуживает маршруты уведомлений.
type Handler struct {
	service *service.Service
	auth    gin.HandlerFunc // Middleware модуля auth, кладёт пользователя в контекст
}

// NewHandler создаёт хендлер модуля уведомлений с middleware аутентификации.
func NewHandler(svc *service.Service, auth gin.HandlerFunc) *Handler {
	return &Handler{service: svc, auth: auth}
}

// registerDeviceRequest - устройство, на которое слать уведомления.
type registerDeviceRequest struct {
	Token    string `json:"token" binding:"required,max=4096"`
	DeviceID string `json:"device_id" binding:"required,max=128"`
	Platform string `json:"platform" binding:"required,oneof=android ios"`
}

// unregisterDeviceRequest - токен устройства, которое больше не получает уведомления.
type unregisterDeviceRequest struct {
	Token string `json:"token" binding:"required,max=4096"`
}

// Init регистрирует маршруты модуля в группе /api/mykct/v1.
func (h *Handler) Init(v1 *gin.RouterGroup) {
	devices := v1.Group("/notifications/devices")
	{
		// Владелец и группа берутся только из токена, не из тела
		devices.POST("", h.auth, h.registerDevice)
		// Без аутентификации: выход работает и с протухшим access, а удалить
		// можно только токен, который знает само устройство
		devices.DELETE("", h.unregisterDevice)
	}
}

// registerDevice запоминает FCM-токен устройства за вошедшим пользователем.
func (h *Handler) registerDevice(c *gin.Context) {
	var req registerDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	user := authctx.MustFrom(c)
	err := h.service.RegisterDevice(c.Request.Context(), service.RegisterDeviceInput{
		DeviceID:      req.DeviceID,
		Token:         req.Token,
		Platform:      req.Platform,
		UserID:        user.ID,
		AcademicGroup: user.AcademicGroup,
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, httpapi.InternalError())
		return
	}

	c.Status(http.StatusNoContent)
}

// unregisterDevice забывает устройство при выходе из аккаунта.
func (h *Handler) unregisterDevice(c *gin.Context) {
	var req unregisterDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	if err := h.service.UnregisterDevice(c.Request.Context(), req.Token); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, httpapi.InternalError())
		return
	}

	c.Status(http.StatusNoContent)
}

// abortWithValidationError отвечает разбором ошибок валидации по полям.
func abortWithValidationError(c *gin.Context, err error) {
	apiErr := httpapi.NewError(httpapi.CodeValidationError, "Проверьте правильность параметров запроса")

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		details := make(map[string]string, len(validationErrs))
		for _, fieldErr := range validationErrs {
			details[strings.ToLower(fieldErr.Field())] = "Некорректное значение"
		}
		apiErr = apiErr.WithDetails(details)
	}

	c.AbortWithStatusJSON(http.StatusBadRequest, apiErr)
}
