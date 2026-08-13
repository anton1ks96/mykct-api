// Package logger предоставляет обёртку над zerolog для структурированного логирования.
package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Logger - глобальный экземпляр логгера zerolog.
var Logger zerolog.Logger

// Init инициализирует глобальный логгер: имя сервиса, минимальный уровень
// (debug/info/warn/error) и консольный вывод вместо JSON при pretty.
func Init(serviceName, minLevel string, pretty bool) {
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}

	zerolog.TimeFieldFormat = time.RFC3339

	var writer io.Writer = os.Stdout

	if pretty {
		writer = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
	}

	Logger = zerolog.New(writer).
		With().
		Timestamp().
		Str("service", serviceName).
		Logger()

	zerolog.SetGlobalLevel(parseLevel(minLevel))
}

// parseLevel преобразует строковое представление уровня в zerolog.Level.
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// Debug возвращает событие уровня debug.
func Debug() *zerolog.Event {
	return Logger.Debug()
}

// Info возвращает событие уровня info.
func Info() *zerolog.Event {
	return Logger.Info()
}

// Warn возвращает событие уровня warn.
func Warn() *zerolog.Event {
	return Logger.Warn()
}

// Error возвращает событие уровня error.
func Error() *zerolog.Event {
	return Logger.Error()
}

// Fatal возвращает событие уровня fatal; после записи вызывает os.Exit(1).
func Fatal() *zerolog.Event {
	return Logger.Fatal()
}
