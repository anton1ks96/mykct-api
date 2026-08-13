// Package router собирает HTTP-роутер приложения: глобальные middleware
// и маршруты модулей.
package router

import (
	"fmt"
	"net/http"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/platform/router/middleware"
	"github.com/gin-gonic/gin"
)

// Router собирает gin.Engine с глобальными middleware и маршрутами модулей.
type Router struct {
	cfg         *config.Config
	rateLimiter *middleware.RateLimiter
}

// NewRouter создаёт новый экземпляр роутера.
func NewRouter(cfg *config.Config, rateLimiter *middleware.RateLimiter) *Router {
	return &Router{
		cfg:         cfg,
		rateLimiter: rateLimiter,
	}
}

// InitRoutes инициализирует все маршруты и возвращает готовый gin.Engine.
func (r *Router) InitRoutes() (*gin.Engine, error) {
	router := gin.New()

	if err := router.SetTrustedProxies(r.cfg.Server.TrustedProxies); err != nil {
		return nil, fmt.Errorf("invalid TRUSTED_PROXIES: %w", err)
	}

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
			_ = v1
		}
	}

	return router, nil
}

// ping проверяет доступность сервиса.
func (r *Router) ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
