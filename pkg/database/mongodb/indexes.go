package mongodb

import (
	"context"
	"fmt"

	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// IndexEnsurer реализуется репозиторием, который сам заводит свои индексы.
type IndexEnsurer interface {
	EnsureIndexes(ctx context.Context) error
}

// EnsureAll создаёт индексы всех переданных репозиториев.
func EnsureAll(ctx context.Context, ensurers ...IndexEnsurer) error {
	for _, ensurer := range ensurers {
		if err := ensurer.EnsureIndexes(ctx); err != nil {
			return fmt.Errorf("failed to ensure indexes for %T: %w", ensurer, err)
		}
	}

	logger.Info().Int("repositories", len(ensurers)).Msg("MongoDB indexes ensured")

	return nil
}
