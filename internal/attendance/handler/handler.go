// Package handler публикует HTTP API модуля посещаемости.
package handler

import (
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/attendance/service"
	"github.com/anton1ks96/mykct-api/internal/auth/authctx"
	"github.com/gin-gonic/gin"
)

// Handler обслуживает маршруты посещаемости.
type Handler struct {
	service *service.Service
	auth    gin.HandlerFunc // Middleware модуля auth, кладёт пользователя в контекст
}

// NewHandler создаёт хендлер модуля посещаемости с middleware аутентификации.
func NewHandler(svc *service.Service, auth gin.HandlerFunc) *Handler {
	return &Handler{service: svc, auth: auth}
}

// Init регистрирует маршруты модуля в группе /api/mykct/v1. Посещаемость личная:
// маршруты за аутентификацией, логин студента берётся только из токена.
func (h *Handler) Init(v1 *gin.RouterGroup) {
	attendance := v1.Group("/attendance", h.auth)
	{
		attendance.GET("", h.getAttendance)
		attendance.GET("/streak", h.getStreak)
	}
}

// getAttendance отдаёт занятия студента за период с отметками посещаемости.
func (h *Handler) getAttendance(c *gin.Context) {
	var req attendanceRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	records, err := h.service.GetAttendance(c.Request.Context(), service.GetAttendanceInput{
		Login: authctx.MustFrom(c).ID,
		Start: req.Start,
		End:   req.End,
	})
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, newAttendanceRecords(records))
}

// getStreak отдаёт серию посещений студента с начала учебного года.
func (h *Handler) getStreak(c *gin.Context) {
	user := authctx.MustFrom(c)

	streak, err := h.service.GetStreak(c.Request.Context(), service.GetStreakInput{
		Login:         user.ID,
		AcademicGroup: user.AcademicGroup,
	})
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, newStreakResponse(streak))
}
