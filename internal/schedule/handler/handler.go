// Package handler публикует HTTP API модуля расписания.
package handler

import (
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/internal/schedule/service"
	"github.com/gin-gonic/gin"
)

// Handler обслуживает маршруты расписания.
type Handler struct {
	service *service.Service
}

// NewHandler создаёт хендлер модуля расписания.
func NewHandler(svc *service.Service) *Handler {
	return &Handler{service: svc}
}

// Init регистрирует маршруты модуля в группе /api/mykct/v1. Пути плоские и без
// аутентификации: мобильные клиенты ходят за расписанием без токена.
func (h *Handler) Init(v1 *gin.RouterGroup) {
	v1.GET("/schedule", h.getSchedule)
	v1.GET("/classdetails", h.getClassDetails)
}

// getSchedule отдаёт расписание группы за период вместе с признаком того,
// получены ли данные от портала колледжа прямо сейчас или из кэша.
func (h *Handler) getSchedule(c *gin.Context) {
	var req scheduleRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	out, err := h.service.GetSchedule(c.Request.Context(), service.GetScheduleInput{
		Group:           req.Group,
		Subgroup:        req.Subgroup,
		EnglishGroup:    req.EnglishGroup,
		ProfileSubgroup: req.ProfileSubgroup,
		Start:           req.Start,
		End:             req.End,
	})
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, scheduleResponse{
		Events:    newScheduleEvents(out.Events),
		Source:    out.Source,
		FetchedAt: formatFetchedAt(out.FetchedAt),
		Stale:     out.Source == domain.SourceCache,
	})
}

// getClassDetails отдаёт детали занятия. Тело портала проносится как есть:
// схема не фиксирована, а клиент показывает пользователю все пришедшие поля,
// поэтому служебные признаки источника сюда не подмешиваются.
func (h *Handler) getClassDetails(c *gin.Context) {
	var req classDetailsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	out, err := h.service.GetClassDetails(c.Request.Context(), req.ID)
	if err != nil {
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, out.Details)
}
