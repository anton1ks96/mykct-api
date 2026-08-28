package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// Коды ошибок модуля аутентификации.
const (
	// CodeInvalidCredentials - неверный логин или пароль.
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	// CodeSessionNotFound - refresh-токен неизвестен, истёк или уже использован.
	CodeSessionNotFound = "SESSION_NOT_FOUND"
	// CodeInvalidToken - access-токен повреждён или просрочен.
	CodeInvalidToken = "INVALID_TOKEN"
	// CodeRoleNotDetermined - у учётной записи нет роли для входа в приложение.
	CodeRoleNotDetermined = "ROLE_NOT_DETERMINED"
	// CodeDirectoryUnavailable - каталог колледжа недоступен.
	CodeDirectoryUnavailable = "DIRECTORY_UNAVAILABLE"
)

// mapDomainError переводит доменную ошибку в статус и тело ответа. Наружу
// уходит только описанный здесь текст: содержимое err не показываем.
func mapDomainError(err error) (int, httpapi.APIError) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		return http.StatusUnauthorized, httpapi.NewError(CodeInvalidCredentials,
			"Неверный логин или пароль")
	case errors.Is(err, domain.ErrSessionNotFound):
		return http.StatusUnauthorized, httpapi.NewError(CodeSessionNotFound,
			"Сессия не найдена или истекла, войдите заново")
	case errors.Is(err, domain.ErrInvalidToken):
		return http.StatusUnauthorized, httpapi.NewError(CodeInvalidToken,
			"Токен доступа недействителен или истёк")
	case errors.Is(err, domain.ErrRoleNotDetermined):
		return http.StatusForbidden, httpapi.NewError(CodeRoleNotDetermined,
			"У учётной записи нет роли для входа в приложение")
	case errors.Is(err, domain.ErrDirectoryUnavailable):
		return http.StatusServiceUnavailable, httpapi.NewError(CodeDirectoryUnavailable,
			"Сервис авторизации колледжа недоступен, попробуйте позже")
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
		"Проверьте правильность заполнения полей")

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		details := make(map[string]string, len(validationErrs))
		for _, fieldErr := range validationErrs {
			details[jsonFieldName(fieldErr.Field())] = validationMessage(fieldErr)
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
	case "min":
		return fmt.Sprintf("Минимальная длина - %s символов", fieldErr.Param())
	default:
		return "Некорректное значение"
	}
}

// jsonFieldName переводит имя поля структуры в snake_case, как в JSON:
// RefreshToken -> refresh_token.
func jsonFieldName(field string) string {
	var b strings.Builder
	b.Grow(len(field) + 3)

	for i, r := range field {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}

	return b.String()
}
