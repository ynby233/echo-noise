// Isolated U3 coordinator, never installed in the product image or router.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
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
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "fixture.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	must(err)
	sqlDB, err := db.DB()
	must(err)
	sqlDB.SetMaxOpenConns(1)
	must(db.AutoMigrate(&models.UpdatePreference{}, &models.UpdateExecutorCredential{}, &models.UpdateTask{}, &models.UpdateTaskEvent{}))
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
	router.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	executor := router.Group("/api/updates/executor", middleware.UpdateExecutorAuthMiddleware())
	executor.GET("/runtime", controllers.GetExecutorRuntime)
	executor.POST("/claim", controllers.ClaimUpdateTask)
	executor.POST("/tasks/:id/events", controllers.RecordUpdateTaskEvent)
	// Fixture endpoints stay loopback-published and never exist in production.
	router.POST("/fixture/create", func(c *gin.Context) {
		task, _, err := service.Create(1, updates.Target{Channel: "edge", Image: "ghcr.io/ynby233/echo-noise", Digest: os.Getenv("FIXTURE_TARGET_DIGEST"), Revision: os.Getenv("FIXTURE_TARGET_REVISION")})
		if err != nil {
			c.Status(http.StatusConflict)
			return
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
	must(server.ListenAndServe())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
