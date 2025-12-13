package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lin-snow/ech0/config"
	"github.com/lin-snow/ech0/internal/database"
	"github.com/lin-snow/ech0/internal/models"
	"github.com/lin-snow/ech0/internal/repository"
	"github.com/lin-snow/ech0/internal/routers"
	"github.com/lin-snow/ech0/internal/syncmanager"
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

func main() {
	// 加载配置
	if err := config.LoadConfig(); err != nil {
		log.Fatalf(models.LoadConfigErrorMessage+": %v", err)
	}

	// 初始化数据库
	if err := database.InitDB(); err != nil {
		log.Fatalf(models.DatabaseInitErrorMessage+": %v", err)
	}

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
	r := routers.SetupRouter()

	// 创建HTTP服务器
	srv := &http.Server{
		Addr:         config.Config.Server.Host + ":" + config.Config.Server.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf(models.ServerLaunchErrorMessage+": %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务器...")

	// 设置关闭超时时间
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 关闭数据库连接
	sqlDB, err := database.DB.DB()
	if err != nil {
		log.Printf("获取数据库实例失败: %v\n", err)
	} else {
		if err := sqlDB.Close(); err != nil {
			log.Printf("数据库关闭错误: %v\n", err)
		}
	}

	// 优雅关闭服务器
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("服务器关闭错误: %v\n", err)
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
