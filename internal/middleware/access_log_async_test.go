package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAccessLogMiddlewareCachesConfigAndBatchesWritesOffRequestPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:access-log-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.SecurityConfig{}, &models.SecurityAccessLog{}); err != nil {
		t.Fatalf("migrate access log tables: %v", err)
	}
	if err := db.Create(&models.SecurityConfig{AccessLogEnabled: true}).Error; err != nil {
		t.Fatalf("enable access log: %v", err)
	}
	models.SetDB(db)
	InvalidateAccessLogConfigCache()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = FlushAccessLogs(ctx)
		models.SetDB(nil)
		InvalidateAccessLogConfigCache()
	})

	var configQueries atomic.Int64
	if err := db.Callback().Query().Before("gorm:query").Register("count_security_config_queries", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "security_configs" {
			configQueries.Add(1)
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	var accessLogCreateCalls atomic.Int64
	if err := db.Callback().Create().Before("gorm:create").Register("slow_access_log_writes", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "security_access_logs" {
			accessLogCreateCalls.Add(1)
			time.Sleep(200 * time.Millisecond)
		}
	}); err != nil {
		t.Fatalf("register create callback: %v", err)
	}

	r := gin.New()
	r.Use(AccessLogMiddleware())
	r.GET("/api/work", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.String(http.StatusOK, "ok")
	})
	started := time.Now()
	for range 3 {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/work", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("request status = %d", response.Code)
		}
	}
	if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
		t.Fatalf("access-log writes blocked request path for %s", elapsed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := FlushAccessLogs(ctx); err != nil {
		t.Fatalf("flush access logs: %v", err)
	}
	var count int64
	if err := db.Model(&models.SecurityAccessLog{}).Count(&count).Error; err != nil {
		t.Fatalf("count access logs: %v", err)
	}
	if count != 3 {
		t.Fatalf("access log count = %d", count)
	}
	if calls := accessLogCreateCalls.Load(); calls != 1 {
		t.Fatalf("access log batch create calls = %d, want one", calls)
	}
	if queries := configQueries.Load(); queries != 1 {
		t.Fatalf("security config queries = %d, want one cached lookup", queries)
	}
}
