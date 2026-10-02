// Isolated U3 coordinator, never installed in the product image or router.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/buildinfo"
	"github.com/rcy1314/echo-noise/internal/controllers"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/updates"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	health := flag.Bool("health", false, "container health probe")
	restore := flag.String("restore", "", "isolated archive restore")
	restoreOnly := flag.Bool("restore-only", false, "exit after isolated restore")
	flag.Parse()
	if *health {
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://127.0.0.1:1314/health")
		if err != nil || response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = response.Body.Close()
		return
	}
	gin.SetMode(gin.ReleaseMode)
	dir := "/data"
	newBuild := buildinfo.CurrentMetadata().Revision == "2222222222222222222222222222222222222222"
	if _, err := os.Stat(filepath.Join(dir, "start-failure")); newBuild && err == nil {
		os.Exit(70)
	}
	must(config.LoadConfig())
	if *restore != "" {
		layout := backup.DefaultLayout()
		_, err := backup.StageRestoreWithResult(*restore, layout)
		must(err)
		applied, err := backup.ApplyPendingRestore(layout)
		must(err)
		must(applied.Commit())
		if *restoreOnly {
			return
		}
	}
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(dir, "fixture.db")+"?_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	must(err)
	sqlDB, err := db.DB()
	must(err)
	sqlDB.SetMaxOpenConns(1)
	if _, err := os.Stat(filepath.Join(dir, "migration-failure")); newBuild && err == nil {
		must(db.Exec("CREATE TABLE IF NOT EXISTS migration_probe(value TEXT)").Error)
		must(db.Exec("INSERT INTO migration_probe VALUES('partial migration')").Error)
		must(db.Exec("INTENTIONALLY INVALID MIGRATION SQL").Error)
	}
	must(db.AutoMigrate(&models.UpdatePreference{}, &models.UpdateExecutorCredential{}, &models.UpdateTask{}, &models.UpdateTaskEvent{}))
	for _, statement := range []string{"CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY, username TEXT, password TEXT, is_admin INTEGER)", "CREATE TABLE IF NOT EXISTS messages(id INTEGER PRIMARY KEY, content TEXT, user_id INTEGER)", "CREATE TABLE IF NOT EXISTS site_configs(id INTEGER PRIMARY KEY)", "INSERT OR IGNORE INTO messages VALUES(1, 'old WAL note', 1)"} {
		must(db.Exec(statement).Error)
	}
	if buildinfo.CurrentMetadata().Revision == "2222222222222222222222222222222222222222" {
		must(db.Exec("INSERT OR IGNORE INTO messages VALUES(2, 'new version write', 1)").Error)
	}
	database.DB = db
	models.SetDB(db)
	service := updates.NewTaskService(db)
	credential, err := service.CurrentCredential()
	must(err)
	if credential == nil {
		_, token, err := service.CreateCredential(models.PrimaryAdminUserID, "isolated fixture")
		must(err)
		must(os.WriteFile(filepath.Join(dir, "token"), []byte(token), 0600))
	}
	instance, err := service.InstanceID()
	must(err)
	must(os.WriteFile(filepath.Join(dir, "instance"), []byte(instance), 0600))
	router := gin.New()
	router.GET("/health", func(c *gin.Context) {
		if _, err := os.Stat(filepath.Join(dir, "fail-health")); err == nil && buildinfo.CurrentMetadata().Revision == "2222222222222222222222222222222222222222" {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Status(http.StatusOK)
	})
	executor := router.Group("/api/updates/executor", middleware.UpdateExecutorAuthMiddleware())
	executor.GET("/runtime", func(c *gin.Context) {
		for _, fault := range []string{"runtime-revision-mismatch", "runtime-instance-mismatch"} {
			if _, err := os.Stat(filepath.Join(dir, fault)); newBuild && err == nil {
				identity := gin.H{"instance_id": instance, "revision": buildinfo.CurrentMetadata().Revision}
				if fault == "runtime-revision-mismatch" {
					identity["revision"] = "1111111111111111111111111111111111111111"
				} else {
					identity["instance_id"] = "wrong-instance"
				}
				c.JSON(http.StatusOK, dto.OK(identity, "isolated runtime fault"))
				return
			}
		}
		controllers.GetExecutorRuntime(c)
	})
	executor.POST("/check", controllers.RecordExecutorDeploymentCheck)
	executor.POST("/claim", controllers.ClaimUpdateTask)
	executor.POST("/tasks/:id/events", controllers.RecordUpdateTaskEvent)
	executor.POST("/tasks/:id/prepare", controllers.PrepareUpdateShutdown)
	// Fixture endpoints stay loopback-published and never exist in production.
	router.POST("/fixture/create", func(c *gin.Context) {
		task, created, err := service.Create(1, updates.Target{Channel: "edge", Image: "ghcr.io/ynby233/echo-noise", Digest: os.Getenv("FIXTURE_TARGET_DIGEST"), Revision: os.Getenv("FIXTURE_TARGET_REVISION")})
		if err != nil {
			c.Status(http.StatusConflict)
			return
		}
		if created {
			_ = updates.WakeExecutor()
		}
		c.JSON(http.StatusCreated, dto.OK(task))
	})
	router.POST("/fixture/rotate", func(c *gin.Context) {
		_, token, err := service.CreateCredential(1, "rotated fixture")
		must(err)
		must(os.WriteFile(filepath.Join(dir, "token-next"), []byte(token), 0600))
		c.Status(http.StatusOK)
	})
	router.POST("/fixture/revoke", func(c *gin.Context) {
		must(service.RevokeCredential(1))
		c.Status(http.StatusOK)
	})
	router.GET("/fixture/tasks/:id", func(c *gin.Context) {
		task, err := service.Get(c.Param("id"))
		must(err)
		c.JSON(http.StatusOK, dto.OK(task))
	})
	router.GET("/fixture/state", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		controllers.GetUpdateState(c)
	})
	server := &http.Server{Addr: ":1314", ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marker := filepath.Join(dir, "drop-final")
		if r.Method == http.MethodPost && r.URL.Path != "/fixture/create" {
			if _, err := os.Stat(marker); err == nil {
				body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
				must(err)
				r.Body = io.NopCloser(bytes.NewReader(body))
				var event struct {
					Status string `json:"status"`
				}
				_ = json.Unmarshal(body, &event)
				if event.Status == updates.TaskSucceeded {
					response := httptest.NewRecorder()
					router.ServeHTTP(response, r)
					if response.Code == http.StatusOK {
						must(os.Remove(marker))
						// The real DB committed, then the TCP response disappeared.
						conn, _, err := w.(http.Hijacker).Hijack()
						must(err)
						_ = conn.Close()
						return
					}
					w.WriteHeader(response.Code)
					_, _ = w.Write(response.Body.Bytes())
					return
				}
			}
		}
		router.ServeHTTP(w, r)
	})}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			must(err)
		}
	}()
	<-quit
	if _, err := os.Stat(filepath.Join(dir, "ignore-stop")); err == nil {
		select {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	must(server.Shutdown(ctx))
	must(sqlDB.Close())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
