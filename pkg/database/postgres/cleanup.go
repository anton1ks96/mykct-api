package postgres

import (
	"context"
	"time"

	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// ExpiryCleaner реализуется репозиторием, который сам удаляет свои протухшие
// строки: TTL-индексов, как в MongoDB, в PostgreSQL нет.
type ExpiryCleaner interface {
	// DeleteExpired удаляет строки с истёкшим expires_at и возвращает их число.
	DeleteExpired(ctx context.Context) (int64, error)
}

// RunCleanup чистит протухшие строки всех переданных репозиториев, пока не
// отменят контекст. Первый прогон идёт сразу, на старте сервиса.
func RunCleanup(ctx context.Context, interval time.Duration, cleaners ...ExpiryCleaner) {
	logger.Info().Dur("interval", interval).Int("repositories", len(cleaners)).
		Msg("postgres cleanup worker started")

	for {
		if ctx.Err() != nil {
			logger.Info().Msg("postgres cleanup worker stopped")
			return
		}

		cleanupOnce(ctx, cleaners)

		select {
		case <-ctx.Done():
			logger.Info().Msg("postgres cleanup worker stopped")
			return
		case <-time.After(interval):
		}
	}
}

// cleanupOnce - один прогон чистки. Сбой одного репозитория не отменяет
// остальные: протухшие строки никому не мешают до следующего прогона.
func cleanupOnce(ctx context.Context, cleaners []ExpiryCleaner) {
	var deleted int64
	for _, cleaner := range cleaners {
		count, err := cleaner.DeleteExpired(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warn().Err(err).Msgf("failed to delete expired rows for %T", cleaner)
			continue
		}
		deleted += count
	}

	if deleted > 0 {
		logger.Info().Int64("deleted", deleted).Msg("expired rows deleted")
	}
}
