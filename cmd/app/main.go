package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/platform/router"
	"github.com/anton1ks96/mykct-api/internal/platform/router/middleware"
	"github.com/anton1ks96/mykct-api/internal/server"
	"github.com/anton1ks96/mykct-api/pkg/database/mongodb"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	pkgsentry "github.com/anton1ks96/mykct-api/pkg/sentry"
	"github.com/gin-gonic/gin"
)

// main - точка входа. Весь DI собирается здесь, в порядке зависимостей.
func main() {
	cfg, err := config.Init()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger.Init(cfg.Service.Name, cfg.Logger.MinLevel, cfg.Logger.Pretty)

	logger.Info().Str("service", cfg.Service.Name).Msg("starting service")

	if cfg.Logger.MinLevel != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Sentry (опционально)
	if cfg.Sentry.DSN != "" {
		err := pkgsentry.InitWithOptions(pkgsentry.Options{
			DSN:              cfg.Sentry.DSN,
			Environment:      cfg.Sentry.Environment,
			TracesSampleRate: cfg.Sentry.TracesSampleRate,
			Debug:            cfg.Sentry.Debug,
		})
		if err != nil {
			logger.Warn().Err(err).Msg("failed to initialize Sentry, continuing without it")
		} else {
			defer pkgsentry.Close()
			logger.EnableSentryHook()
			logger.Info().Msg("Sentry enabled")
		}
	} else {
		logger.Info().Msg("Sentry DSN not provided, running without Sentry")
	}

	// MongoDB
	mongoClient, err := mongodb.NewClient(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to MongoDB")
	}
	defer mongodb.Close(context.Background(), mongoClient)

	if err := mongodb.EnsureAll(context.Background()); err != nil {
		logger.Fatal().Err(err).Msg("failed to ensure MongoDB indexes")
	}

	// Rate limiter
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimit)
	defer rateLimiter.Close()

	// Роутер и сервер
	r := router.NewRouter(cfg, rateLimiter)

	engine, err := r.InitRoutes()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init routes")
	}
	srv := server.NewServer(cfg, engine)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	_ = workerCtx

	go func() {
		if err := srv.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("failed to run HTTP server")
		}
	}()

	logger.Info().Str("service", cfg.Service.Name).Str("port", cfg.Server.Port).Msg("service started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Dur("timeout", cfg.Server.ShutdownTimeout).Msg("shutting down server")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancelShutdown()
	stopWorkers()

	var stopping sync.WaitGroup
	stopping.Add(2)
	go func() {
		defer stopping.Done()
		if err := srv.Stop(shutdownCtx); err != nil {
			logger.Warn().Err(err).Msg("HTTP server did not drain gracefully")
		}
	}()
	go func() {
		defer stopping.Done()
		workers.Wait()
		logger.Info().Msg("background workers stopped")
	}()

	stopped := make(chan struct{})
	go func() {
		stopping.Wait()
		close(stopped)
	}()

	select {
	case <-stopped:
		logger.Info().Msg("server exited")
	case <-shutdownCtx.Done():
		logger.Warn().Dur("timeout", cfg.Server.ShutdownTimeout).
			Msg("shutdown budget exhausted, exiting with workers still running")
	}
}
