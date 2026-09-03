package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/routers"
	"github.com/rcy1314/echo-noise/internal/services"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
)

func init() {
	// 确保必要的目录存在
	dirs := []string{
		"data",
		"data/images",
		"logs",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("创建目录失败 %s: %v", dir, err)
		}
	}

	// 设置日志输出
	logFile := filepath.Join("logs", fmt.Sprintf("ech0_%s.log", time.Now().Format("2006-01-02")))
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("打开日志文件失败: %v", err)
	}
	log.SetOutput(io.MultiWriter(f, os.Stdout))
}

const databaseCloseTimeout = 5 * time.Second

func logLifecycleStage(phase, stage, status string, started time.Time) {
	log.Printf("lifecycle phase=%s stage=%s status=%s elapsed=%s", phase, stage, status, time.Since(started).Round(time.Millisecond))
}

func closeDatabaseWithTimeout(timeout time.Duration) error {
	if database.DB == nil {
		return nil
	}
	sqlDB, err := database.DB.DB()
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- sqlDB.Close()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return context.DeadlineExceeded
	}
}

func main() {
	stageStarted := time.Now()
	logLifecycleStage("startup", "config_load", "begin", stageStarted)
	// 加载配置
	if err := config.LoadConfig(); err != nil {
		logLifecycleStage("startup", "config_load", "failed", stageStarted)
		log.Fatalf(models.LoadConfigErrorMessage+": %v", err)
	}
	logLifecycleStage("startup", "config_load", "completed", stageStarted)

	stageStarted = time.Now()
	logLifecycleStage("startup", "database_init", "begin", stageStarted)

	// 初始化数据库
	if err := database.InitDB(); err != nil {
		logLifecycleStage("startup", "database_init", "failed", stageStarted)
		log.Fatalf(models.DatabaseInitErrorMessage+": %v", err)
	}
	logLifecycleStage("startup", "database_init", "completed", stageStarted)

	stageStarted = time.Now()
	logLifecycleStage("startup", "default_data_seed", "begin", stageStarted)

	// 初始化默认数据
	if err := services.SeedDefaultData(); err != nil {
		log.Printf("初始化默认数据警告: %v", err)
	}
	logLifecycleStage("startup", "default_data_seed", "completed", stageStarted)

	stageStarted = time.Now()
	logLifecycleStage("startup", "workers_start", "begin", stageStarted)
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	services.StartAnnouncementPushDispatcher(workerCtx, database.DB)
	services.StartWebPushDispatcher(workerCtx, database.DB)
	services.StartVoceChatProvisioningWorker(workerCtx)
	services.StartLogRetentionWorker(workerCtx, database.DB)
	// Recycle-bin retention is a system policy, independent of any delegated
	// administrator. Run once at startup and retry on each daily tick.
	go func() {
		run := func() {
			succeeded, failed, skipped, err := services.RunRecycleBinAutoCleanup(database.DB, time.Now().UTC())
			if err != nil {
				log.Printf("回收站自动清理失败: %v", err)
				return
			}
			if succeeded > 0 || failed > 0 || skipped > 0 {
				log.Printf("回收站自动清理完成: 成功=%d 失败=%d 跳过=%d", succeeded, failed, skipped)
			}
			commentSucceeded, commentFailed, commentErr := services.RunCommentRecycleBinAutoCleanup(database.DB, time.Now().UTC())
			if commentErr != nil {
				log.Printf("互动回收站自动清理失败: %v", commentErr)
			} else if commentSucceeded > 0 || commentFailed > 0 {
				log.Printf("互动回收站自动清理完成: 成功=%d 失败=%d", commentSucceeded, commentFailed)
			}
		}
		run()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				run()
			case <-workerCtx.Done():
				return
			}
		}
	}()

	// 读取站点配置并应用到自动同步管理器（确保定时/即时模式在启动后即生效）
	func() {
		db := models.GetDB()
		if db == nil {
			return
		}
		var cfg models.SiteConfig
		if err := db.Table("site_configs").First(&cfg).Error; err == nil {
			syncmanager.Configure(cfg)
		}
	}()
	services.StartInfoFeedAutoRefresh()
	logLifecycleStage("startup", "workers_start", "completed", stageStarted)

	// 设置Gin模式
	ginMode := config.Config.Server.Mode
	if ginMode == "debug" {
		gin.SetMode(gin.DebugMode)
	} else if ginMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// 设置路由
	stageStarted = time.Now()
	logLifecycleStage("startup", "router_setup", "begin", stageStarted)
	r := routers.SetupRouter()
	logLifecycleStage("startup", "router_setup", "completed", stageStarted)

	// Migrate historical public cloud-attachment URLs in the background. The
	// job is retry-safe and intentionally does not delay server availability.
	go func() {
		run := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
			defer cancel()
			if err := services.MigrateLegacyCloudAttachments(ctx); err != nil {
				log.Printf("历史云附件安全迁移暂未完成，将自动重试: %v", err)
			}
		}
		run()
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()

	// 创建HTTP服务器
	srv := &http.Server{
		Addr:         config.Config.Server.Host + ":" + config.Config.Server.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Minute,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	// 启动定时清理缓存任务
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			repository.ClearExpiredCache() // 移除错误检查
		}
	}()

	// 启动定时清理过期日志任务
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanOldLogs(7) // 保留最近7天的日志
		}
	}()

	// 在独立的goroutine中启动服务器
	go func() {
		log.Printf("服务器启动于 %s\n", srv.Addr)
		listenStarted := time.Now()
		logLifecycleStage("startup", "http_listen", "begin", listenStarted)
		listener, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			logLifecycleStage("startup", "http_listen", "failed", listenStarted)
			log.Fatalf(models.ServerLaunchErrorMessage+": %v", err)
		}
		logLifecycleStage("startup", "http_listen", "ready", listenStarted)
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf(models.ServerLaunchErrorMessage+": %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务器...")
	stageStarted = time.Now()
	logLifecycleStage("shutdown", "workers_stop", "begin", stageStarted)
	cancelWorkers()
	logLifecycleStage("shutdown", "workers_stop", "completed", stageStarted)

	// 设置关闭超时时间
	stageStarted = time.Now()
	logLifecycleStage("shutdown", "http_shutdown", "begin", stageStarted)
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)

	// 优雅关闭服务器
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logLifecycleStage("shutdown", "http_shutdown", "failed", stageStarted)
		log.Printf("服务器关闭错误: %v\n", err)
	} else {
		logLifecycleStage("shutdown", "http_shutdown", "completed", stageStarted)
	}
	cancelShutdown()

	stageStarted = time.Now()
	logLifecycleStage("shutdown", "access_log_flush", "begin", stageStarted)
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), 5*time.Second)
	if err := middleware.FlushAccessLogs(flushCtx); err != nil {
		logLifecycleStage("shutdown", "access_log_flush", "failed", stageStarted)
		log.Printf("访问日志刷新错误: %v\n", err)
	} else {
		logLifecycleStage("shutdown", "access_log_flush", "completed", stageStarted)
	}
	cancelFlush()

	// 所有请求和异步日志都已结束后再关闭数据库连接。
	stageStarted = time.Now()
	logLifecycleStage("shutdown", "database_close", "begin", stageStarted)
	if err := closeDatabaseWithTimeout(databaseCloseTimeout); err != nil {
		logLifecycleStage("shutdown", "database_close", "failed", stageStarted)
		log.Printf("数据库关闭错误: %v\n", err)
	} else {
		logLifecycleStage("shutdown", "database_close", "completed", stageStarted)
	}

	log.Println("服务器已关闭")
}

// cleanOldLogs 清理指定天数之前的日志文件
func cleanOldLogs(days int) {
	logDir := "logs"
	cutoff := time.Now().AddDate(0, 0, -days)

	files, err := os.ReadDir(logDir)
	if err != nil {
		log.Printf("读取日志目录失败: %v", err)
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(logDir, file.Name())
			if err := os.Remove(path); err != nil {
				log.Printf("删除旧日志文件失败 %s: %v", path, err)
			}
		}
	}
}
