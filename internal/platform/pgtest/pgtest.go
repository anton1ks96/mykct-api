// Package pgtest поднимает подключение к живой PostgreSQL для тестов
// репозиториев: запросы и схема проверяются только на настоящей базе.
package pgtest

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/migrations"
	"github.com/anton1ks96/mykct-api/pkg/database/postgres"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/jmoiron/sqlx"
)

// DSNEnv - переменная со строкой подключения к тестовой базе. Отдельная от
// POSTGRES_*: тесты чистят таблицы, и попасть в базу разработки они не должны.
//
//	TEST_POSTGRES_DSN=postgres://postgres:postgres@localhost:5433/mykct_test go test ./...
const DSNEnv = "TEST_POSTGRES_DSN"

// connectTimeout - бюджет проверки связи с тестовой базой.
const connectTimeout = 5 * time.Second

// initLogger инициализирует логгер один раз на прогон пакета: репозитории
// пишут в него на каждом вызове.
var initLogger sync.Once

// DB открывает тестовую базу, накатывает миграции и чистит переданные таблицы.
// Без TEST_POSTGRES_DSN тест пропускается: живой базы под рукой может не быть.
func DB(t *testing.T, tables ...string) *sqlx.DB {
	t.Helper()

	dsn := os.Getenv(DSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set, skipping repository test", DSNEnv)
	}

	initLogger.Do(func() { logger.Init("pgtest", "error", false) })

	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping test database: %v", err)
	}

	if err := postgres.RunMigrations(db, migrations.FS); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	truncate(t, db, tables)

	return db
}

// truncate чистит таблицы теста. Наборы таблиц у пакетов не пересекаются,
// поэтому параллельный прогон пакетов друг другу не мешает.
func truncate(t *testing.T, db *sqlx.DB, tables []string) {
	t.Helper()

	if len(tables) == 0 {
		return
	}

	if _, err := db.Exec(`TRUNCATE ` + strings.Join(tables, ", ") + ` RESTART IDENTITY`); err != nil {
		t.Fatalf("failed to truncate test tables: %v", err)
	}
}
