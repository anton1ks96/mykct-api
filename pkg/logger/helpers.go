package logger

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// ContextKey - тип ключей контекста пакета.
type ContextKey string

const (
	// RequestIDKey - ключ request_id в контексте запроса.
	RequestIDKey ContextKey = "request_id"
)

// ComponentLogger создаёт логгер слоя с предустановленным полем component.
func ComponentLogger(component string) LazyLogger {
	return LazyLogger{component: component}
}

// LazyLogger берёт актуальный Logger при каждом вызове: package-level
// переменные инициализируются раньше logger.Init().
type LazyLogger struct {
	component string
}

// Debug возвращает событие уровня debug.
func (l LazyLogger) Debug() *zerolog.Event {
	return Logger.Debug().Str("component", l.component)
}

// Info возвращает событие уровня info.
func (l LazyLogger) Info() *zerolog.Event {
	return Logger.Info().Str("component", l.component)
}

// Warn возвращает событие уровня warn.
func (l LazyLogger) Warn() *zerolog.Event {
	return Logger.Warn().Str("component", l.component)
}

// Error возвращает событие уровня error.
func (l LazyLogger) Error() *zerolog.Event {
	return Logger.Error().Str("component", l.component)
}

// WithContext добавляет request_id из контекста в событие лога.
func WithContext(ctx context.Context, e *zerolog.Event) *zerolog.Event {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok && requestID != "" {
		return e.Str("request_id", requestID)
	}
	return e
}

// MaskPhone маскирует номер телефона: +79991234567 -> +7***4567
func MaskPhone(phone string) string {
	if len(phone) < 8 {
		return "***"
	}
	return phone[:2] + "***" + phone[len(phone)-4:]
}

// MaskEmail маскирует почтовый адрес: ivan@example.com -> i***@example.com
func MaskEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at < 1 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}

// Timer измеряет время выполнения операции.
type Timer struct {
	start time.Time
}

// StartTimer создаёт новый таймер.
func StartTimer() *Timer {
	return &Timer{start: time.Now()}
}

// ElapsedMs возвращает прошедшее время в миллисекундах.
func (t *Timer) ElapsedMs() int64 {
	return time.Since(t.start).Milliseconds()
}

// LogOp - контекст логирования одной операции: добавляет method,
// duration_ms и request_id из контекста.
type LogOp struct {
	ctx    context.Context
	logger LazyLogger
	method string
	timer  *Timer
}

// NewLogOp создаёт контекст логирования операции и запускает таймер.
func NewLogOp(ctx context.Context, logger LazyLogger, method string) *LogOp {
	return &LogOp{
		ctx:    ctx,
		logger: logger,
		method: method,
		timer:  StartTimer(),
	}
}

// Debug возвращает событие debug с контекстом операции.
func (op *LogOp) Debug() *zerolog.Event {
	return WithContext(op.ctx, op.logger.Debug()).Str("method", op.method)
}

// Info возвращает событие info с контекстом операции.
func (op *LogOp) Info() *zerolog.Event {
	return WithContext(op.ctx, op.logger.Info()).Str("method", op.method)
}

// Warn возвращает событие warn с контекстом операции и duration.
func (op *LogOp) Warn() *zerolog.Event {
	return WithContext(op.ctx, op.logger.Warn()).
		Str("method", op.method).
		Int64("duration_ms", op.timer.ElapsedMs())
}

// Error возвращает событие error с ошибкой и duration.
func (op *LogOp) Error(err error) *zerolog.Event {
	return WithContext(op.ctx, op.logger.Error()).
		Str("method", op.method).
		Int64("duration_ms", op.timer.ElapsedMs()).
		Err(err)
}

// Started логирует начало операции.
func (op *LogOp) Started() *zerolog.Event {
	return WithContext(op.ctx, op.logger.Info()).Str("method", op.method)
}

// Completed логирует успешное завершение операции с duration.
func (op *LogOp) Completed() *zerolog.Event {
	return WithContext(op.ctx, op.logger.Info()).
		Str("method", op.method).
		Int64("duration_ms", op.timer.ElapsedMs())
}

// Failed логирует неуспешное завершение операции с ошибкой и duration.
func (op *LogOp) Failed(err error) *zerolog.Event {
	return WithContext(op.ctx, op.logger.Error()).
		Str("method", op.method).
		Int64("duration_ms", op.timer.ElapsedMs()).
		Err(err)
}

// DurationMs возвращает текущее время выполнения операции в миллисекундах.
func (op *LogOp) DurationMs() int64 {
	return op.timer.ElapsedMs()
}
