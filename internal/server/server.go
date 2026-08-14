// Package server реализует HTTP-сервер приложения.
package server

import (
	"context"
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

var log = logger.ComponentLogger("server.http")

// Server представляет HTTP-сервер.
type Server struct {
	httpServer *http.Server
}

// NewServer создаёт HTTP-сервер с таймаутами из конфигурации.
func NewServer(cfg *config.Config, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              ":" + cfg.Server.Port,
			Handler:           handler,
			ReadTimeout:       cfg.Server.ReadTimeout,
			ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
			WriteTimeout:      cfg.Server.WriteTimeout,
			MaxHeaderBytes:    cfg.Server.MaxHeaderBytes << 20,
			IdleTimeout:       cfg.Server.IdleTimeout,
		},
	}
}

// Run запускает HTTP-сервер.
func (s *Server) Run() error {
	log.Info().Str("addr", s.httpServer.Addr).Msg("HTTP server starting")
	return s.httpServer.ListenAndServe()
}

// Stop корректно останавливает сервер, дожидаясь активных запросов.
func (s *Server) Stop(ctx context.Context) error {
	log.Info().Msg("HTTP server shutting down")
	return s.httpServer.Shutdown(ctx)
}
