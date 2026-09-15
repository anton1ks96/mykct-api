package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// CodeAttendanceUnavailable - портал колледжа не отдал посещаемость.
const CodeAttendanceUnavailable = "ATTENDANCE_UNAVAILABLE"

// mapDomainError переводит доменную ошибку в статус и тело ответа. Наружу
// уходит только описанный здесь текст: содержимое err не показываем.
func mapDomainError(err error) (int, httpapi.APIError) {
	switch {
	case errors.Is(err, domain.ErrPortalUnavailable):
		return http.StatusServiceUnavailable, httpapi.NewError(CodeAttendanceUnavailable,
			"Посещаемость недоступна: сервер колледжа не отвечает, попробуйте позже")
	default:
		return http.StatusInternalServerError, httpapi.InternalError()
	}
}

// abortWithDomainError отвечает ошибкой, разобранной по доменному типу.
func abortWithDomainError(c *gin.Context, err error) {
	c.AbortWithStatusJSON(mapDomainError(err))
}

// abortWithValidationError отвечает разбором ошибок валидации по полям. Имена
// параметров однословные, поэтому в query-имя их переводит нижний регистр.
func abortWithValidationError(c *gin.Context, err error) {
	apiErr := httpapi.NewError(httpapi.CodeValidationError,
		"Проверьте правильность параметров запроса")

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		details := make(map[string]string, len(validationErrs))
		for _, fieldErr := range validationErrs {
			details[strings.ToLower(fieldErr.Field())] = validationMessage(fieldErr)
		}
		apiErr = apiErr.WithDetails(details)
	}

	c.AbortWithStatusJSON(http.StatusBadRequest, apiErr)
}

// validationMessage - причина отказа по конкретному параметру, на русском.
func validationMessage(fieldErr validator.FieldError) string {
	switch fieldErr.Tag() {
	case "required":
		return "Параметр обязателен"
	case "datetime":
		return "Дата в формате ГГГГ-ММ-ДД"
	default:
		return "Некорректное значение"
	}
}
