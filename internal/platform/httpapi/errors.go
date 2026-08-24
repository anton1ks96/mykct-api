// Package httpapi содержит общие типы HTTP API.
package httpapi

import "fmt"

// Коды ошибок платформы. Доменные коды объявляются в handler/errors.go модуля.
const (
	// CodeInternalError - непредвиденная ошибка сервера.
	CodeInternalError = "INTERNAL_ERROR"
	// CodeValidationError - тело или параметры запроса не прошли валидацию.
	CodeValidationError = "VALIDATION_ERROR"
	// CodeUnauthorized - запрос без валидного токена.
	CodeUnauthorized = "UNAUTHORIZED"
	// CodeForbidden - токен валиден, но прав недостаточно.
	CodeForbidden = "FORBIDDEN"
	// CodeNotFound - маршрут или ресурс не существует.
	CodeNotFound = "NOT_FOUND"
	// CodeMethodNotAllowed - метод не поддерживается этим маршрутом.
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	// CodeRateLimitExceeded - превышен лимит частоты запросов.
	CodeRateLimitExceeded = "RATE_LIMIT_EXCEEDED"
)

// APIError - единый конверт ошибок API: машиночитаемый code для логики фронта
// и message на русском для показа пользователю.
// Успешные ответы возвращаются типизированными DTO без конверта.
type APIError struct {
	Code    string            `json:"code"`              // Машиночитаемый код: USER_NOT_FOUND
	Message string            `json:"message"`           // Текст для пользователя, на русском
	Details map[string]string `json:"details,omitempty"` // Поле -> причина, для ошибок валидации
}

// NewError собирает ответ об ошибке с сообщением на русском.
func NewError(code, message string) APIError {
	return APIError{Code: code, Message: message}
}

// NewErrorf собирает ответ об ошибке, подставляя детали запроса в message.
func NewErrorf(code, format string, args ...any) APIError {
	return NewError(code, fmt.Sprintf(format, args...))
}

// WithDetails добавляет разбор по полям, обычно для CodeValidationError.
func (e APIError) WithDetails(details map[string]string) APIError {
	e.Details = details
	return e
}

// InternalError - стандартный ответ 500: наружу подробности не отдаём.
func InternalError() APIError {
	return NewError(CodeInternalError, "Внутренняя ошибка сервера, попробуйте позже")
}
