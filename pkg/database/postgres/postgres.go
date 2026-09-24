// Package postgres предоставляет клиент для подключения к PostgreSQL.
package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	_ "github.com/jackc/pgx/v5/stdlib" // Драйвер database/sql для PostgreSQL
	"github.com/jmoiron/sqlx"
)

// NewClient создаёт подключённый клиент PostgreSQL и проверяет связь ping-ом.
func NewClient(cfg *config.Config) (*sqlx.DB, error) {
	db, err := sqlx.Open("pgx", dsn(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	db.SetMaxOpenConns(cfg.Postgres.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Postgres.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Postgres.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.Postgres.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Postgres.ConnectTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	logger.Info().Str("database", cfg.Postgres.DBName).Msg("connected to PostgreSQL")

	return db, nil
}

// dsn собирает строку подключения. Пароль и имя базы экранируются: в пароле
// из openssl rand -base64 приходят / и +, от которых склеенный вручную DSN
// перестаёт разбираться.
func dsn(cfg *config.Config) string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.Postgres.User, cfg.Postgres.Password),
		Host:   net.JoinHostPort(cfg.Postgres.Host, strconv.Itoa(cfg.Postgres.Port)),
		Path:   cfg.Postgres.DBName,
		RawQuery: url.Values{
			"sslmode":          {cfg.Postgres.SSLMode},
			"application_name": {cfg.Service.Name},
		}.Encode(),
	}

	return u.String()
}

// Close закрывает соединение с PostgreSQL.
func Close(db *sqlx.DB) {
	if err := db.Close(); err != nil {
		logger.Error().Err(err).Msg("failed to close PostgreSQL connection")
		return
	}
	logger.Info().Msg("PostgreSQL connection closed")
}
