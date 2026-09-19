// Package handler публикует HTTP API модуля посещаемости.
package handler

import (
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/internal/attendance/service"
	"github.com/anton1ks96/mykct-api/internal/auth/authctx"
	authdomain "github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/platform/router/middleware"
	"github.com/gin-gonic/gin"
)

// Лимит рейтинга: таблица читается из базы и стоит дёшево, но частый опрос -
// это сбор динамики псевдонимов. Ключ лимита - IP, а за NAT колледжа он общий
// на всех, поэтому щедро.
const (
	leaderboardRate  = 30
	leaderboardBurst = 60
)

// Handler обслуживает маршруты посещаемости.
type Handler struct {
	service     *service.Service
	auth        gin.HandlerFunc // Middleware модуля auth, кладёт пользователя в контекст
	rateLimiter *middleware.RateLimiter
	leaderboard bool // Включён ли рейтинг: без него маршрута нет вовсе
}

// NewHandler создаёт хендлер модуля посещаемости с middleware аутентификации.
func NewHandler(
	svc *service.Service,
	auth gin.HandlerFunc,
	rateLimiter *middleware.RateLimiter,
	leaderboard config.LeaderboardConfig,
) *Handler {
	return &Handler{
		service:     svc,
		auth:        auth,
		rateLimiter: rateLimiter,
		leaderboard: leaderboard.Enabled,
	}
}

// Init регистрирует маршруты модуля в группе /api/mykct/v1. Посещаемость личная:
// маршруты за аутентификацией, логин студента берётся только из токена.
func (h *Handler) Init(v1 *gin.RouterGroup) {
	attendance := v1.Group("/attendance", h.auth)
	{
		attendance.GET("", h.getAttendance)
		attendance.GET("/streak", h.getStreak)
		// Выключенный рейтинг отдаёт штатный 404, а не мёртвый маршрут
		if h.leaderboard {
			attendance.GET("/leaderboard",
				h.rateLimiter.Limit(leaderboardRate, leaderboardBurst), h.getLeaderboard)
		}
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

// getLeaderboard отдаёт анонимный рейтинг курса студента и его место в нём.
func (h *Handler) getLeaderboard(c *gin.Context) {
	user := authctx.MustFrom(c)

	// Рейтинг студенческий: преподавателю в нём не с кем соревноваться, а его
	// собственный курс из токена не выводится
	if user.Role != authdomain.RoleStudent || user.AcademicGroup == "" {
		abortWithDomainError(c, domain.ErrLeaderboardForbidden)
		return
	}

	board, err := h.service.GetLeaderboard(c.Request.Context(), service.GetLeaderboardInput{
		Login:         user.ID,
		AcademicGroup: user.AcademicGroup,
	})
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, newLeaderboardResponse(board))
}
