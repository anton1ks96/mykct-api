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

	"firebase.google.com/go/v4/messaging"
	attendancehandler "github.com/anton1ks96/mykct-api/internal/attendance/handler"
	attendanceportal "github.com/anton1ks96/mykct-api/internal/attendance/repository/portal"
	attendancepg "github.com/anton1ks96/mykct-api/internal/attendance/repository/postgres"
	attendanceservice "github.com/anton1ks96/mykct-api/internal/attendance/service"
	authhandler "github.com/anton1ks96/mykct-api/internal/auth/handler"
	authldap "github.com/anton1ks96/mykct-api/internal/auth/repository/ldap"
	authpg "github.com/anton1ks96/mykct-api/internal/auth/repository/postgres"
	authservice "github.com/anton1ks96/mykct-api/internal/auth/service"
	notificationhandler "github.com/anton1ks96/mykct-api/internal/notification/handler"
	notificationpg "github.com/anton1ks96/mykct-api/internal/notification/repository/postgres"
	notificationservice "github.com/anton1ks96/mykct-api/internal/notification/service"
	performancehandler "github.com/anton1ks96/mykct-api/internal/performance/handler"
	performanceportal "github.com/anton1ks96/mykct-api/internal/performance/repository/portal"
	performanceservice "github.com/anton1ks96/mykct-api/internal/performance/service"
	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/platform/router"
	"github.com/anton1ks96/mykct-api/internal/platform/router/middleware"
	schedulehandler "github.com/anton1ks96/mykct-api/internal/schedule/handler"
	scheduleportal "github.com/anton1ks96/mykct-api/internal/schedule/repository/portal"
	schedulepg "github.com/anton1ks96/mykct-api/internal/schedule/repository/postgres"
	scheduleservice "github.com/anton1ks96/mykct-api/internal/schedule/service"
	"github.com/anton1ks96/mykct-api/internal/server"
	"github.com/anton1ks96/mykct-api/migrations"
	"github.com/anton1ks96/mykct-api/pkg/database/postgres"
	pkgfirebase "github.com/anton1ks96/mykct-api/pkg/firebase"
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

	// PostgreSQL
	db, err := postgres.NewClient(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to PostgreSQL")
	}
	defer postgres.Close(db)

	if err := postgres.RunMigrations(db, migrations.FS); err != nil {
		logger.Fatal().Err(err).Msg("failed to run migrations")
	}

	// Rate limiter
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimit)
	defer rateLimiter.Close()

	// Модуль аутентификации
	if cfg.Auth.TestMode {
		logger.Warn().Msg("AUTH_TEST_MODE is enabled, LDAP is bypassed with a stub user")
	}

	authSessions := authpg.NewSessionRepository(db)
	authDirectory := authldap.NewDirectory(cfg.LDAP)
	authSvc := authservice.NewService(authDirectory, authSessions, cfg.Auth)
	authAPI := authhandler.NewHandler(authSvc, rateLimiter)

	// Модуль уведомлений. Без ключа Firebase устройства регистрируются, но
	// рассылки нет: notifier остаётся nil, и воркер расписания её не зовёт
	var fcmClient *messaging.Client
	if cfg.Push.CredentialsPath != "" {
		fcmClient, err = pkgfirebase.NewMessagingClient(context.Background(), cfg.Push.CredentialsPath)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to init FCM")
		}
		logger.Info().Msg("push notifications enabled")
	} else {
		logger.Info().Msg("FCM_CREDENTIALS_PATH not provided, push notifications are disabled")
	}

	notificationDevices := notificationpg.NewDeviceRepository(db)
	notificationSvc := notificationservice.NewService(notificationDevices, fcmClient)
	notificationAPI := notificationhandler.NewHandler(notificationSvc, authAPI.Auth())

	// Интерфейс заполняется только живым клиентом: *Service с nil внутри дал
	// бы непустой интерфейс, и воркер слал бы в выключенную рассылку
	var scheduleNotifier scheduleservice.Notifier
	if fcmClient != nil {
		scheduleNotifier = notificationSvc
	}

	// Модуль расписания
	schedulePortal := scheduleportal.NewClient(cfg.Schedule)
	scheduleSnapshots := schedulepg.NewSnapshotRepository(db, cfg.Schedule.CacheTTL)
	scheduleStates := schedulepg.NewWeekStateRepository(db, cfg.Schedule.Watch.StateTTL)
	scheduleTracked := schedulepg.NewTrackedGroupRepository(db)
	scheduleChanges := schedulepg.NewChangeRepository(db, cfg.Schedule.Watch.StateTTL)
	scheduleSvc := scheduleservice.NewService(schedulePortal, scheduleSnapshots, scheduleStates,
		scheduleTracked, scheduleChanges, authSvc, scheduleNotifier, cfg.Schedule.Watch)
	scheduleAPI := schedulehandler.NewHandler(scheduleSvc)

	// Модуль посещаемости
	attendancePortal := attendanceportal.NewClient(cfg.Attendance)
	attendanceLeaderboard := attendancepg.NewLeaderboardRepository(db)
	attendanceSvc := attendanceservice.NewService(attendancePortal, attendanceLeaderboard, authSvc,
		cfg.Attendance.Leaderboard)
	attendanceAPI := attendancehandler.NewHandler(attendanceSvc, authAPI.Auth(), rateLimiter,
		cfg.Attendance.Leaderboard)

	// Модуль успеваемости
	performancePortal := performanceportal.NewClient(cfg.Performance)
	performanceSvc := performanceservice.NewService(performancePortal)
	performanceAPI := performancehandler.NewHandler(performanceSvc, authAPI.Auth())

	// Роутер и сервер
	r := router.NewRouter(cfg, rateLimiter, authAPI, scheduleAPI, attendanceAPI, performanceAPI,
		notificationAPI)

	engine, err := r.InitRoutes()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init routes")
	}
	srv := server.NewServer(cfg, engine)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	var workers sync.WaitGroup

	workers.Add(1)
	go func() {
		defer workers.Done()
		postgres.RunCleanup(workerCtx, cfg.Postgres.CleanupInterval,
			authSessions, scheduleSnapshots, scheduleStates, scheduleChanges)
	}()

	if cfg.Schedule.Watch.Enabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			scheduleSvc.RunScheduleWatcher(workerCtx)
		}()
	} else {
		logger.Info().Msg("schedule watcher is disabled")
	}

	if cfg.Attendance.Leaderboard.Enabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			attendanceSvc.RunLeaderboardWorker(workerCtx)
		}()
	} else {
		logger.Info().Msg("attendance leaderboard is disabled")
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
