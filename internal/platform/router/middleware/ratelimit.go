package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/internal/platform/httpapi"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

var rateLimitLog = logger.ComponentLogger("middleware.ratelimit")

// visitor - лимитер одного ключа, его окно и время последнего обращения.
type visitor struct {
	limiter  *rate.Limiter
	window   time.Duration
	lastSeen time.Time
}

// RateLimiter - in-memory ограничение частоты запросов (token bucket) с
// отдельным лимитером на каждый ключ и периодической чисткой протухших.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	ipRate   int
	ipBurst  int
	window   time.Duration
	enabled  bool
	stop     chan struct{}
	ipExempt map[string]struct{}
}

// NewRateLimiter создаёт лимитер из конфигурации; при выключенном - no-op.
func NewRateLimiter(cfg config.RateLimitConfig) *RateLimiter {
	if !cfg.Enabled {
		rateLimitLog.Info().Msg("rate limiting disabled")
		return &RateLimiter{enabled: false}
	}

	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		ipRate:   cfg.IPRate,
		ipBurst:  cfg.IPBurst,
		window:   time.Duration(cfg.WindowSeconds) * time.Second,
		enabled:  true,
		stop:     make(chan struct{}),
		ipExempt: make(map[string]struct{}),
	}

	go rl.cleanupLoop()

	rateLimitLog.Info().
		Int("ip_rate", cfg.IPRate).
		Int("ip_burst", cfg.IPBurst).
		Int("window_seconds", cfg.WindowSeconds).
		Msg("rate limiting enabled")

	return rl
}

// Close останавливает фоновую очистку устаревших лимитеров.
func (rl *RateLimiter) Close() error {
	if !rl.enabled {
		return nil
	}
	close(rl.stop)
	return nil
}

// ExemptFromIPLimit освобождает точные пути от общего лимита по IP.
func (rl *RateLimiter) ExemptFromIPLimit(paths ...string) {
	if !rl.enabled {
		return
	}
	for _, path := range paths {
		rl.ipExempt[path] = struct{}{}
	}
}

// LimitByIP - глобальный лимит по IP-адресу клиента.
func (rl *RateLimiter) LimitByIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.enabled {
			c.Next()
			return
		}
		if _, exempt := rl.ipExempt[c.Request.URL.Path]; exempt {
			c.Next()
			return
		}

		limiter := rl.getVisitor("rl:ip:"+c.ClientIP(), rl.ipRate, rl.ipBurst)

		if retryAfter, ok := rl.allow(c, limiter); !ok {
			rateLimitLog.Warn().
				Str("client_ip", c.ClientIP()).
				Str("path", c.Request.URL.Path).
				Msg("IP rate limit exceeded")

			c.AbortWithStatusJSON(http.StatusTooManyRequests, httpapi.NewErrorf(
				httpapi.CodeRateLimitExceeded,
				"Слишком много запросов, повторите через %d с", retryAfter,
			))
			return
		}

		c.Next()
	}
}

// Limit - лимит на конкретном эндпоинте: ключ включает путь запроса.
func (rl *RateLimiter) Limit(rateN, burst int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.enabled {
			c.Next()
			return
		}

		identifier := clientIdentifier(c)

		limiter := rl.getVisitor("rl:"+c.Request.URL.Path+":"+identifier, rateN, burst)

		if retryAfter, ok := rl.allow(c, limiter); !ok {
			rateLimitLog.Warn().
				Str("identifier", identifier).
				Str("path", c.Request.URL.Path).
				Int("rate", rateN).
				Msg("endpoint rate limit exceeded")

			c.AbortWithStatusJSON(http.StatusTooManyRequests, httpapi.NewErrorf(
				httpapi.CodeRateLimitExceeded,
				"Слишком много запросов, повторите через %d с", retryAfter,
			))
			return
		}

		c.Next()
	}
}

// clientIdentifier - ключ клиента для лимитов.
// С появлением аутентификации возвращать здесь ID пользователя, если он есть.
func clientIdentifier(c *gin.Context) string {
	return c.ClientIP()
}

// getVisitor возвращает лимитер ключа, создавая его при первом обращении.
func (rl *RateLimiter) getVisitor(key string, rateN, burst int) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, ok := rl.visitors[key]
	if !ok {
		v = &visitor{
			limiter: rate.NewLimiter(rate.Limit(float64(rateN)/rl.window.Seconds()), burst),
			window:  rl.window,
		}
		rl.visitors[key] = v
	}
	v.lastSeen = time.Now()

	return v.limiter
}

// allow проверяет лимит и выставляет заголовки; при превышении возвращает
// количество секунд до следующей попытки.
func (rl *RateLimiter) allow(c *gin.Context, limiter *rate.Limiter) (int, bool) {
	res := limiter.Reserve()
	if !res.OK() || res.Delay() > 0 {
		if res.OK() {
			res.Cancel()
		}

		retryAfter := int(res.Delay().Seconds())
		if retryAfter <= 0 {
			retryAfter = 1
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		rl.setHeaders(c, limiter)

		return retryAfter, false
	}

	rl.setHeaders(c, limiter)

	return 0, true
}

// setHeaders выставляет заголовки X-RateLimit-*.
func (rl *RateLimiter) setHeaders(c *gin.Context, limiter *rate.Limiter) {
	remaining := int(limiter.Tokens())
	if remaining < 0 {
		remaining = 0
	}
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("X-RateLimit-Reset", strconv.Itoa(int(rl.window.Seconds())))
}

// cleanupLoop периодически удаляет лимитеры, к которым давно не обращались.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.mu.Lock()
			for key, v := range rl.visitors {
				if time.Since(v.lastSeen) > 3*v.window {
					delete(rl.visitors, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}
