package mobilebackend

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lin-snow/ech0/config"
	"github.com/lin-snow/ech0/internal/database"
	"github.com/lin-snow/ech0/internal/routers"
)

var srv *http.Server

func ensureDirs() {
	_ = os.MkdirAll("data", 0755)
	_ = os.MkdirAll("logs", 0755)
}

func Start(workDir string) error {
	if workDir != "" {
		_ = os.Chdir(workDir)
	}
	ensureDirs()
	os.Setenv("DB_TYPE", "sqlite")
	if os.Getenv("DB_PATH") == "" {
		os.Setenv("DB_PATH", filepath.Join("data", "noise.db"))
	}
	if err := config.LoadConfig(); err != nil {
		config.Config.Server.Port = "1314"
		config.Config.Server.Host = "127.0.0.1"
		config.Config.Server.Mode = "release"
		config.Config.Database.Type = "sqlite"
		if p := os.Getenv("DB_PATH"); p != "" {
			config.Config.Database.Path = p
		} else {
			config.Config.Database.Path = filepath.Join("data", "noise.db")
		}
	}
	if err := database.InitDB(); err != nil {
		return err
	}
	mode := config.Config.Server.Mode
	if mode == "debug" {
		gin.SetMode(gin.DebugMode)
	} else if mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}
	r := routers.SetupRouter()
	srv = &http.Server{
		Addr:         config.Config.Server.Host + ":" + config.Config.Server.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() { _ = srv.ListenAndServe() }()
	return nil
}

func Stop() {
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
