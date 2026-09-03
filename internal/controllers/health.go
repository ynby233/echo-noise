package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
)

const healthReadyTimeout = 300 * time.Millisecond

// HealthLive reports whether the HTTP process can still schedule and serve a
// request. It must remain independent from databases and external services.
func HealthLive(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 1, "status": "ok"})
}

// HealthReady reports whether the application can currently acquire and use a
// database connection. The deadline keeps this diagnostic endpoint useful when
// the connection pool is exhausted.
func HealthReady(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 0, "status": "unavailable"})
		return
	}

	sqlDB, err := db.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 0, "status": "unavailable"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), healthReadyTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		database.LogConnectionPoolPressure("readiness_probe", db)
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 0, "status": "unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "status": "ready"})
}
