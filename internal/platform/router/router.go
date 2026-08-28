// Package router собирает HTTP-роутер приложения: глобальные middleware
// и маршруты модулей.
package router

import (
	"fmt"
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/anton1ks96/mykct-api/internal/platform/router/middleware"
	"github.com/gin-gonic/gin"
)

// Module - бизнес-модуль, регистрирующий свои маршруты в группе /api/v1.
type Module interface {
	Init(v1 *gin.RouterGroup)
}

// Router собирает gin.Engine с глобальными middleware и маршрутами модулей.
type Router struct {
	cfg         *config.Config
	rateLimiter *middleware.RateLimiter
	modules     []Module
}

// NewRouter создаёт новый экземпляр роутера с маршрутами переданных модулей.
func NewRouter(cfg *config.Config, rateLimiter *middleware.RateLimiter, modules ...Module) *Router {
	return &Router{
		cfg:         cfg,
		rateLimiter: rateLimiter,
		modules:     modules,
	}
}

// InitRoutes инициализирует все маршруты и возвращает готовый gin.Engine.
func (r *Router) InitRoutes() (*gin.Engine, error) {
	router := gin.New()

	if err := router.SetTrustedProxies(r.cfg.Server.TrustedProxies); err != nil {
		return nil, fmt.Errorf("invalid TRUSTED_PROXIES: %w", err)
	}

	router.HandleMethodNotAllowed = true
	router.NoRoute(notFound)
	router.NoMethod(methodNotAllowed)

	router.Use(
		middleware.Recovery(),
		middleware.Logging(),
		middleware.CORS(r.cfg.CORS.AllowedOrigins),
		r.rateLimiter.LimitByIP(),
	)

	api := router.Group("/api")
	{
		api.GET("/ping", r.ping)

		v1 := api.Group("/v1")
		{
			for _, module := range r.modules {
				module.Init(v1)
			}
		}
	}

	return router, nil
}

// ping проверяет доступность сервиса.
func (r *Router) ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// notFound отвечает на неизвестный маршрут в общем формате ошибок.
func notFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, httpapi.NewErrorf(
		httpapi.CodeNotFound,
		"Маршрут %s %s не найден", c.Request.Method, c.Request.URL.Path,
	))
}

// methodNotAllowed отвечает, когда маршрут есть, но метод другой.
func methodNotAllowed(c *gin.Context) {
	c.JSON(http.StatusMethodNotAllowed, httpapi.NewErrorf(
		httpapi.CodeMethodNotAllowed,
		"Метод %s не поддерживается для %s", c.Request.Method, c.Request.URL.Path,
	))
}
