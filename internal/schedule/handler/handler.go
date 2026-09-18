// Package handler публикует HTTP API модуля расписания.
package handler

import (
	"errors"
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
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

// Init регистрирует маршруты модуля в группе /api/mykct/v1. Пути без
// аутентификации: мобильные клиенты ходят за расписанием без токена.
func (h *Handler) Init(v1 *gin.RouterGroup) {
	schedule := v1.Group("/schedule")
	{
		schedule.GET("", h.getSchedule)
		schedule.GET("/next-week", h.getNextWeekStatus)
	}
	v1.GET("/classdetails", h.getClassDetails)
}

// getNextWeekStatus отдаёт состояние следующей недели: появилось ли расписание
// и когда это заметил воркер.
func (h *Handler) getNextWeekStatus(c *gin.Context) {
	var req nextWeekRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		abortWithValidationError(c, err)
		return
	}

	state, err := h.service.GetNextWeekState(c.Request.Context(), req.Group)
	if err != nil {
		if errors.Is(err, domain.ErrWeekStateNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, httpapi.NewErrorf(CodeWeekNotTracked,
				"Расписание на следующую неделю для группы %s пока не отслеживается", req.Group))
			return
		}
		abortWithDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, newNextWeekResponse(state))
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
