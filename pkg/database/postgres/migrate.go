package postgres

import (
	"embed"
	"errors"
	"fmt"

	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
)

// RunMigrations применяет SQL-миграции из встроенной файловой системы.
// Зовётся при старте приложения; отсутствие новых миграций ошибкой не считается.
func RunMigrations(db *sqlx.DB, migrationsFS embed.FS) error {
	sourceDriver, err := iofs.New(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("failed to create migrations source: %w", err)
	}

	dbDriver, err := migratepg.WithInstance(db.DB, &migratepg.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migrations database driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info().Msg("no new migrations to apply")
			return nil
		}
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	logger.Info().Msg("migrations applied")

	return nil
}
