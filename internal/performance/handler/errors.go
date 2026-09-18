package handler

import (
	"errors"
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/performance/domain"
	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// CodePerformanceUnavailable - портал колледжа не отдал успеваемость.
const CodePerformanceUnavailable = "PERFORMANCE_UNAVAILABLE"

// mapDomainError переводит доменную ошибку в статус и тело ответа. Наружу
// уходит только описанный здесь текст: содержимое err не показываем.
func mapDomainError(err error) (int, httpapi.APIError) {
	switch {
	case errors.Is(err, domain.ErrPortalUnavailable):
		return http.StatusServiceUnavailable, httpapi.NewError(CodePerformanceUnavailable,
			"Успеваемость недоступна: сервер колледжа не отвечает, попробуйте позже")
	default:
		return http.StatusInternalServerError, httpapi.InternalError()
	}
}

// abortWithDomainError отвечает ошибкой, разобранной по доменному типу.
func abortWithDomainError(c *gin.Context, err error) {
	c.AbortWithStatusJSON(mapDomainError(err))
}

// abortWithValidationError отвечает разбором ошибок валидации по полям. Битое
// или пустое тело разбора по полям не имеет, тогда details нет.
func abortWithValidationError(c *gin.Context, err error) {
	apiErr := httpapi.NewError(httpapi.CodeValidationError,
		"Проверьте правильность заполнения полей")

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		details := make(map[string]string, len(validationErrs))
		for _, fieldErr := range validationErrs {
			details[scoreRequestFields[fieldErr.StructField()]] = validationMessage(fieldErr)
		}
		apiErr = apiErr.WithDetails(details)
	}

	c.AbortWithStatusJSON(http.StatusBadRequest, apiErr)
}

// validationMessage - причина отказа по конкретному полю, на русском.
func validationMessage(fieldErr validator.FieldError) string {
	switch fieldErr.Tag() {
	case "required":
		return "Поле обязательно"
	case "datetime":
		return "Дата в формате ГГГГ-ММ-ДД"
	default:
		return "Некорректное значение"
	}
}
