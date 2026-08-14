// Package httpapi содержит общие типы HTTP API.
package httpapi

// APIError - единый конверт для всех ошибок API.
// Успешные ответы возвращаются типизированными DTO без конверта.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
