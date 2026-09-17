package handler

import (
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// Коды ошибок модуля расписания.
const (
	// CodeScheduleUnavailable - портал колледжа недоступен и снимка расписания нет.
	CodeScheduleUnavailable = "SCHEDULE_UNAVAILABLE"
	// CodeClassDetailsUnavailable - портал колледжа недоступен и деталей занятия нет.
	CodeClassDetailsUnavailable = "CLASS_DETAILS_UNAVAILABLE"
	// CodeWeekNotTracked - за следующей неделей группы ещё не следили.
	CodeWeekNotTracked = "SCHEDULE_WEEK_NOT_TRACKED"
)

// mapDomainError переводит доменную ошибку в статус и тело ответа. Наружу
// уходит только описанный здесь текст: содержимое err не показываем.
func mapDomainError(err error) (int, httpapi.APIError) {
	switch {
	case errors.Is(err, domain.ErrScheduleUnavailable):
		return http.StatusServiceUnavailable, httpapi.NewError(CodeScheduleUnavailable,
			"Расписание недоступно: сервер колледжа не отвечает, а сохранённой копии пока нет")
	case errors.Is(err, domain.ErrClassDetailsUnavailable):
		return http.StatusServiceUnavailable, httpapi.NewError(CodeClassDetailsUnavailable,
			"Детали занятия недоступны: сервер колледжа не отвечает, а сохранённой копии пока нет")
	default:
		return http.StatusInternalServerError, httpapi.InternalError()
	}
}

// abortWithDomainError отвечает ошибкой, разобранной по доменному типу.
func abortWithDomainError(c *gin.Context, err error) {
	c.AbortWithStatusJSON(mapDomainError(err))
}

// abortWithValidationError отвечает разбором ошибок валидации по полям.
func abortWithValidationError(c *gin.Context, err error) {
	apiErr := httpapi.NewError(httpapi.CodeValidationError,
		"Проверьте правильность параметров запроса")

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		details := make(map[string]string, len(validationErrs))
		for _, fieldErr := range validationErrs {
			details[queryFieldName(fieldErr.Field())] = validationMessage(fieldErr)
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

// queryFieldName переводит имя поля структуры в snake_case, как в query-строке:
// EnglishGroup -> english_group. Подряд идущие заглавные не разрываются, иначе
// ID превратилось бы в i_d.
func queryFieldName(field string) string {
	var b strings.Builder
	b.Grow(len(field) + 3)

	prevUpper := false
	for i, r := range field {
		if unicode.IsUpper(r) {
			if i > 0 && !prevUpper {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			prevUpper = true
			continue
		}
		b.WriteRune(r)
		prevUpper = false
	}

	return b.String()
}
