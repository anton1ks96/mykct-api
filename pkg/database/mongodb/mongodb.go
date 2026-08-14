// Package mongodb предоставляет клиент для подключения к MongoDB.
package mongodb

import (
	"context"
	"fmt"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// NewClient создаёт подключённый клиент MongoDB и проверяет связь ping-ом.
func NewClient(cfg *config.Config) (*mongo.Client, error) {
	opts := options.Client().
		ApplyURI(cfg.Mongo.URI).
		SetAppName(cfg.Service.Name).
		SetConnectTimeout(cfg.Mongo.ConnectTimeout).
		SetServerSelectionTimeout(cfg.Mongo.ServerSelectionTimeout).
		SetMaxPoolSize(cfg.Mongo.MaxPoolSize).
		SetMinPoolSize(cfg.Mongo.MinPoolSize).
		SetMaxConnIdleTime(cfg.Mongo.MaxConnIdleTime)

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open MongoDB connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Mongo.ConnectTimeout)
	defer cancel()

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	logger.Info().Str("database", cfg.Mongo.Database).Msg("connected to MongoDB")

	return client, nil
}

// Close закрывает соединение с MongoDB.
func Close(ctx context.Context, client *mongo.Client) {
	if err := client.Disconnect(ctx); err != nil {
		logger.Error().Err(err).Msg("failed to close MongoDB connection")
		return
	}
	logger.Info().Msg("MongoDB connection closed")
}
