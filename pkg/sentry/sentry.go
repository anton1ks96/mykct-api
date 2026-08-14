// Package sentry предоставляет обёртку над sentry-go для отслеживания ошибок.
package sentry

import (
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/getsentry/sentry-go"
)

// Options - параметры инициализации Sentry.
type Options struct {
	DSN              string  // DSN проекта Sentry (обязателен)
	Environment      string  // Окружение приложения: development, production
	TracesSampleRate float64 // Доля транзакций для трейсинга (0.0-1.0), по умолчанию 1.0
	Debug            bool    // Debug-режим клиента Sentry
}

// InitWithOptions инициализирует клиент Sentry с заданными параметрами.
func InitWithOptions(opts Options) error {
	if opts.DSN == "" {
		return errors.New("SENTRY_DSN is required")
	}

	if opts.TracesSampleRate < 0.0 || opts.TracesSampleRate > 1.0 {
		return fmt.Errorf("TracesSampleRate must be between 0.0 and 1.0, got: %f", opts.TracesSampleRate)
	}

	if opts.TracesSampleRate == 0.0 {
		opts.TracesSampleRate = 1.0
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              opts.DSN,
		Environment:      opts.Environment,
		TracesSampleRate: opts.TracesSampleRate,
		Debug:            opts.Debug,
	})
	if err != nil {
		return fmt.Errorf("sentry initialization failed: %w", err)
	}

	logger.Info().
		Str("environment", opts.Environment).
		Float64("traces_sample_rate", opts.TracesSampleRate).
		Msg("connected to Sentry")

	return nil
}

// Close досылает оставшиеся события и закрывает клиент.
func Close() {
	sentry.Flush(2 * time.Second)
}

// CaptureException отправляет ошибку в Sentry.
func CaptureException(err error) {
	if err != nil {
		sentry.CaptureException(err)
	}
}
