package logger

import (
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
)

// SentryHook отправляет error- и fatal-логи в Sentry.
type SentryHook struct{}

// Run вызывается на каждое событие лога и пересылает error/fatal в Sentry.
func (h SentryHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if level != zerolog.ErrorLevel && level != zerolog.FatalLevel {
		return
	}

	sentryLevel := sentry.LevelError
	if level == zerolog.FatalLevel {
		sentryLevel = sentry.LevelFatal
	}

	event := sentry.NewEvent()
	event.Message = msg
	event.Level = sentryLevel
	sentry.CaptureEvent(event)

	// Fatal завершает процесс: ждём отправки события
	if level == zerolog.FatalLevel {
		sentry.Flush(2 * time.Second)
	}
}

// EnableSentryHook вешает hook на глобальный логгер; вызывать после
// logger.Init() и sentry.Init().
func EnableSentryHook() {
	Logger = Logger.Hook(SentryHook{})
}
