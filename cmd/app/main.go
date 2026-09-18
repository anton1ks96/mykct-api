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

	attendancehandler "github.com/anton1ks96/mykct-api/internal/attendance/handler"
	attendanceportal "github.com/anton1ks96/mykct-api/internal/attendance/repository/portal"
	attendanceservice "github.com/anton1ks96/mykct-api/internal/attendance/service"
	authhandler "github.com/anton1ks96/mykct-api/internal/auth/handler"
	authldap "github.com/anton1ks96/mykct-api/internal/auth/repository/ldap"
	authmongo "github.com/anton1ks96/mykct-api/internal/auth/repository/mongo"
	authservice "github.com/anton1ks96/mykct-api/internal/auth/service"
	performancehandler "github.com/anton1ks96/mykct-api/internal/performance/handler"
	performanceportal "github.com/anton1ks96/mykct-api/internal/performance/repository/portal"
	performanceservice "github.com/anton1ks96/mykct-api/internal/performance/service"
	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/platform/router"
	"github.com/anton1ks96/mykct-api/internal/platform/router/middleware"
	schedulehandler "github.com/anton1ks96/mykct-api/internal/schedule/handler"
	schedulemongo "github.com/anton1ks96/mykct-api/internal/schedule/repository/mongo"
	scheduleportal "github.com/anton1ks96/mykct-api/internal/schedule/repository/portal"
	scheduleservice "github.com/anton1ks96/mykct-api/internal/schedule/service"
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

	// Rate limiter
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimit)
	defer rateLimiter.Close()

	// Модуль аутентификации
	if cfg.Auth.TestMode {
		logger.Warn().Msg("AUTH_TEST_MODE is enabled, LDAP is bypassed with a stub user")
	}

	authSessions := authmongo.NewSessionRepository(mongoClient, cfg.Mongo.Database)
	authDirectory := authldap.NewDirectory(cfg.LDAP)
	authSvc := authservice.NewService(authDirectory, authSessions, cfg.Auth)
	authAPI := authhandler.NewHandler(authSvc, rateLimiter)

	// Модуль расписания
	schedulePortal := scheduleportal.NewClient(cfg.Schedule)
	scheduleSnapshots := schedulemongo.NewSnapshotRepository(mongoClient, cfg.Mongo.Database, cfg.Schedule.CacheTTL)
	scheduleStates := schedulemongo.NewWeekStateRepository(mongoClient, cfg.Mongo.Database, cfg.Schedule.Watch.StateTTL)
	scheduleTracked := schedulemongo.NewTrackedGroupRepository(mongoClient, cfg.Mongo.Database)
	scheduleSvc := scheduleservice.NewService(schedulePortal, scheduleSnapshots, scheduleStates,
		scheduleTracked, authSvc, cfg.Schedule.Watch)
	scheduleAPI := schedulehandler.NewHandler(scheduleSvc)

	// Модуль посещаемости
	attendancePortal := attendanceportal.NewClient(cfg.Attendance)
	attendanceSvc := attendanceservice.NewService(attendancePortal)
	attendanceAPI := attendancehandler.NewHandler(attendanceSvc, authAPI.Auth())

	// Модуль успеваемости
	performancePortal := performanceportal.NewClient(cfg.Performance)
	performanceSvc := performanceservice.NewService(performancePortal)
	performanceAPI := performancehandler.NewHandler(performanceSvc, authAPI.Auth())

	if err := mongodb.EnsureAll(context.Background(), authSessions, scheduleSnapshots, scheduleStates, scheduleTracked); err != nil {
		logger.Fatal().Err(err).Msg("failed to ensure MongoDB indexes")
	}

	// Роутер и сервер
	r := router.NewRouter(cfg, rateLimiter, authAPI, scheduleAPI, attendanceAPI, performanceAPI)

	engine, err := r.InitRoutes()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init routes")
	}
	srv := server.NewServer(cfg, engine)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	var workers sync.WaitGroup

	if cfg.Schedule.Watch.Enabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			scheduleSvc.RunNextWeekWatcher(workerCtx)
		}()
	} else {
		logger.Info().Msg("next week schedule watcher is disabled")
	}

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
