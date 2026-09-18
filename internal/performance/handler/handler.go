// Package handler публикует HTTP API модуля успеваемости.
package handler

import (
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/auth/authctx"
	"github.com/anton1ks96/mykct-api/internal/performance/service"
	"github.com/gin-gonic/gin"
)

// Handler обслуживает маршруты успеваемости.
type Handler struct {
	service *service.Service
	auth    gin.HandlerFunc // Middleware модуля auth, кладёт пользователя в контекст
}

// NewHandler создаёт хендлер модуля успеваемости с middleware аутентификации.
func NewHandler(svc *service.Service, auth gin.HandlerFunc) *Handler {
	return &Handler{service: svc, auth: auth}
}

// Init регистрирует маршруты модуля в группе /api/mykct/v1. Успеваемость личная:
// маршруты за аутентификацией, логин студента берётся только из токена.
func (h *Handler) Init(v1 *gin.RouterGroup) {
	performance := v1.Group("/performance", h.auth)
	{
		performance.GET("/subjects", h.getSubjects)
		performance.POST("/score", h.getScore)
	}
}

// getSubjects отдаёт предметы студента в текущем семестре.
func (h *Handler) getSubjects(c *gin.Context) {
	subjects, err := h.service.GetSubjects(c.Request.Context(), authctx.MustFrom(c).ID)
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, newSubjects(subjects))
}

// getScore отдаёт оценки студента по предмету за период. POST с телом, а не GET,
// ради совместимости с клиентами college-app-core.
func (h *Handler) getScore(c *gin.Context) {
	var req scoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	scores, err := h.service.GetScores(c.Request.Context(), service.GetScoresInput{
		Login: authctx.MustFrom(c).ID,
		SuID:  req.SuID,
		Start: req.DataStart,
		End:   req.DataEnd,
	})
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, newScores(scores))
}
