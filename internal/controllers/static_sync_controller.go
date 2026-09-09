package controllers

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
)

func SyncStatic(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	wd, _ := os.Getwd()
	webDir := filepath.Join(wd, "web")
	outDir := filepath.Join(webDir, ".output", "public")
	{
		var stderr bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-lc", "cd web && npm run generate")
		cmd.Env = os.Environ()
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			c.JSON(http.StatusOK, dto.Fail[string]("前端构建失败: "+msg))
			return
		}
	}
	pubDir := filepath.Join(wd, "public")
	_ = os.RemoveAll(pubDir)
	_ = os.MkdirAll(pubDir, 0755)
	if _, err := exec.LookPath("rsync"); err == nil {
		var stderr bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-lc", "rsync -a --delete '"+outDir+"/' '"+pubDir+"/'")
		cmd.Env = os.Environ()
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			c.JSON(http.StatusOK, dto.Fail[string]("静态资源同步失败: "+msg))
			return
		}
	} else {
		if err := copyDir(outDir, pubDir); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("静态资源同步失败: "+err.Error()))
			return
		}
	}
	c.JSON(http.StatusOK, dto.OK[any](gin.H{"public": pubDir}, "静态资源已同步"))
}

func GetRuntimeEnv(c *gin.Context) {
	isContainer := func() bool {
		if _, err := os.Stat("/.dockerenv"); err == nil {
			return true
		}
		b, _ := os.ReadFile("/proc/1/cgroup")
		s := strings.ToLower(string(b))
		if strings.Contains(s, "docker") || strings.Contains(s, "containerd") || strings.Contains(s, "kubepods") {
			return true
		}
		return false
	}()
	wd, _ := os.Getwd()
	outDir := filepath.Join(wd, "web", ".output", "public")
	pubDir := filepath.Join(wd, "public")
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"isContainer":         isContainer,
			"staticSyncAvailable": !isContainer,
			"outDir":              outDir,
			"publicDir":           pubDir,
		},
	})
}
