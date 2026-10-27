package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type HealthChecker interface {
	Ping(ctx context.Context) error
}

func NewEngine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(RequestID(), AccessLog(), Recover())
	router.NoRoute(func(c *gin.Context) {
		writeError(c, http.StatusNotFound, "NOT_FOUND", "route not found", nil)
	})
	return router
}

func NewRouter(health HealthChecker) *gin.Engine {
	router := NewEngine()
	router.GET("/health", Health(health))
	router.GET("/api/health", Health(health))
	return router
}

func Health(health HealthChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := health.Ping(c.Request.Context()); err != nil {
			slog.WarnContext(c.Request.Context(), "health check failed", "request_id", c.GetString(requestIDKey), "error", err)
			writeError(c, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "database is not reachable", nil)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
