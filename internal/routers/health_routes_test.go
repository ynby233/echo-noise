package routers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLivenessProbeDoesNotWaitForBusyDatabase(t *testing.T) {
	dsn := fmt.Sprintf("file:health-live-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	database.DB = db
	models.SetDB(db)
	middleware.InvalidateAccessLogConfigCache()
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
		middleware.InvalidateAccessLogConfigCache()
	})

	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("occupy only database connection: %v", err)
	}
	connectionReleased := false
	releaseConnection := func() {
		if connectionReleased {
			return
		}
		connectionReleased = true
		_ = conn.Close()
	}
	defer releaseConnection()

	r := SetupRouter()
	started := time.Now()
	completed := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health/live", nil))
		completed <- response
	}()

	var response *httptest.ResponseRecorder
	select {
	case response = <-completed:
	case <-time.After(250 * time.Millisecond):
		releaseConnection()
		select {
		case <-completed:
		case <-time.After(2 * time.Second):
		}
		t.Fatal("liveness probe blocked while the database connection was busy")
	}

	if response.Code != http.StatusOK {
		t.Fatalf("liveness status = %d, body = %s", response.Code, response.Body.String())
	}
	if elapsed := time.Since(started); elapsed >= 50*time.Millisecond {
		t.Fatalf("liveness probe waited for database for %s", elapsed)
	}
}

func TestReadinessProbeFailsFastWhenDatabaseIsBusyAndRecovers(t *testing.T) {
	dsn := fmt.Sprintf("file:health-ready-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	database.DB = db
	models.SetDB(db)
	middleware.InvalidateAccessLogConfigCache()
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
		middleware.InvalidateAccessLogConfigCache()
	})

	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("occupy only database connection: %v", err)
	}
	connectionReleased := false
	releaseConnection := func() {
		if connectionReleased {
			return
		}
		connectionReleased = true
		_ = conn.Close()
	}
	defer releaseConnection()

	r := SetupRouter()
	completed := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health/ready", nil))
		completed <- response
	}()

	var unavailable *httptest.ResponseRecorder
	select {
	case unavailable = <-completed:
	case <-time.After(750 * time.Millisecond):
		releaseConnection()
		select {
		case <-completed:
		case <-time.After(2 * time.Second):
		}
		t.Fatal("readiness probe did not fail fast while the database was busy")
	}
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("busy readiness status = %d, body = %s", unavailable.Code, unavailable.Body.String())
	}

	releaseConnection()
	recovered := httptest.NewRecorder()
	r.ServeHTTP(recovered, httptest.NewRequest(http.MethodGet, "/api/health/ready", nil))
	if recovered.Code != http.StatusOK {
		t.Fatalf("recovered readiness status = %d, body = %s", recovered.Code, recovered.Body.String())
	}
}
