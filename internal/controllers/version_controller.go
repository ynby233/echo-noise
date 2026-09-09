package controllers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
)

// 检查版本更新
func CheckVersion(c *gin.Context) {
	client := &http.Client{Timeout: 5 * time.Second}
	type tagInfo struct{ Name, LastUpdated string }
	latest := tagInfo{}

	get := func(url string, v any) error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req = req.WithContext(ctx)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return json.NewDecoder(resp.Body).Decode(v)
	}

	type result struct {
		ok   bool
		info tagInfo
	}
	ch := make(chan result, 3)
	go func() {
		var v struct {
			Name        string `json:"name"`
			LastUpdated string `json:"last_updated"`
		}
		if get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags/latest", &v) == nil && strings.TrimSpace(v.LastUpdated) != "" {
			ch <- result{true, tagInfo{v.Name, v.LastUpdated}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	go func() {
		var v struct {
			Results []struct {
				Name        string `json:"name"`
				LastUpdated string `json:"last_updated"`
			} `json:"results"`
		}
		if get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags?page_size=1&ordering=last_updated", &v) == nil && len(v.Results) > 0 && strings.TrimSpace(v.Results[0].LastUpdated) != "" {
			r := v.Results[0]
			ch <- result{true, tagInfo{r.Name, r.LastUpdated}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	go func() {
		var v struct {
			TagName     string `json:"tag_name"`
			PublishedAt string `json:"published_at"`
		}
		if get("https://api.github.com/repos/noise233/echo-noise/releases/latest", &v) == nil && strings.TrimSpace(v.PublishedAt) != "" {
			ch <- result{true, tagInfo{v.TagName, v.PublishedAt}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	for i := 0; i < 3; i++ {
		r := <-ch
		if r.ok {
			latest = r.info
			break
		}
	}
	if strings.TrimSpace(latest.LastUpdated) == "" {
		cur := strings.TrimSpace(os.Getenv("ECHO_NOISE_VERSION"))
		if cur == "" {
			cur = strings.TrimSpace(os.Getenv("APP_VERSION"))
		}
		if cur == "" {
			cur = strings.TrimSpace(os.Getenv("IMAGE_TAG"))
		}
		if cur == "" {
			cur = "latest"
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"hasUpdate": false, "lastUpdateTime": time.Now().Format(time.RFC3339), "currentTag": publicVersionLabel()}})
		return
	}
	cur := strings.TrimSpace(os.Getenv("ECHO_NOISE_VERSION"))
	if cur == "" {
		cur = strings.TrimSpace(os.Getenv("APP_VERSION"))
	}
	if cur == "" {
		cur = strings.TrimSpace(os.Getenv("IMAGE_TAG"))
	}
	if cur == "" {
		cur = "latest"
	}
	var curUpdated string
	if strings.ToLower(cur) == "latest" {
		curUpdated = strings.TrimSpace(latest.LastUpdated)
	} else {
		if resp, err := client.Get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags/" + cur); err == nil {
			defer resp.Body.Close()
			var curTag struct {
				Name        string `json:"name"`
				LastUpdated string `json:"last_updated"`
			}
			if json.NewDecoder(resp.Body).Decode(&curTag) == nil {
				curUpdated = strings.TrimSpace(curTag.LastUpdated)
			}
		}
		if strings.TrimSpace(curUpdated) == "" {
			if resp, err := client.Get("https://api.github.com/repos/noise233/echo-noise/releases/tags/" + cur); err == nil {
				defer resp.Body.Close()
				var rel struct {
					PublishedAt string `json:"published_at"`
				}
				if json.NewDecoder(resp.Body).Decode(&rel) == nil {
					curUpdated = strings.TrimSpace(rel.PublishedAt)
				}
			}
		}
	}
	latestTime, err := time.Parse(time.RFC3339, latest.LastUpdated)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "解析时间失败"})
		return
	}
	var hasUpdate bool
	if curUpdated != "" {
		curTime, err := time.Parse(time.RFC3339, curUpdated)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "解析时间失败"})
			return
		}
		hasUpdate = latestTime.After(curTime)
	} else {
		hasUpdate = true
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"hasUpdate": hasUpdate, "lastUpdateTime": latest.LastUpdated, "currentTag": publicVersionLabel()}})
}

func publicVersionLabel() string {
	return "installed"
}

// 获取公开运行版本。真实镜像标签仅供服务端升级逻辑使用，避免向匿名请求暴露构建提交。
func GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"version": publicVersionLabel(),
		},
	})
}

func UpdateVersion(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	hasUpdate, _, _, chkErr := computeUpgradeInfo()
	if chkErr != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("版本检测失败: "+chkErr.Error()))
		return
	}
	if !hasUpdate {
		c.JSON(http.StatusOK, dto.Fail[string]("已是最新版，无需升级"))
		return
	}

	var logs bytes.Buffer
	shellArgs := func() (string, []string) {
		if _, err := exec.LookPath("bash"); err == nil {
			return "bash", []string{"-lc"}
		}
		return "sh", []string{"-c"}
	}
	run := func(timeout time.Duration, cmdStr string) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		prog, args := shellArgs()
		cmd := exec.CommandContext(ctx, prog, append(args, cmdStr)...)
		cmd.Env = os.Environ()
		cmd.Stdout = &logs
		cmd.Stderr = &logs
		return cmd.Run()
	}

	image := strings.TrimSpace(os.Getenv("UPDATE_IMAGE"))
	if image == "" {
		image = "noise233/echo-noise:latest"
	}
	name := strings.TrimSpace(os.Getenv("CONTAINER_NAME"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("ECH0_CONTAINER_NAME"))
	}
	if name == "" {
		name = "Ech0-Noise"
	}
	hostPort := strings.TrimSpace(os.Getenv("HTTP_PORT"))
	if hostPort == "" {
		hostPort = "1314"
	}
	wd, _ := os.Getwd()
	dataDir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dataDir == "" {
		candidates := []string{"/opt/data", filepath.Join(wd, "data"), "/data"}
		for _, d := range candidates {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				dataDir = d
				break
			}
		}
		if dataDir == "" {
			dataDir = filepath.Join(wd, "data")
			_ = os.MkdirAll(dataDir, 0755)
		}
	}

	if err := run(10*time.Second, "docker --version"); err != nil {
		custom := strings.TrimSpace(os.Getenv("DESKTOP_UPDATE_CMD"))
		if custom == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("Docker 未就绪: "+err.Error()))
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-lc", custom)
		cmd.Env = os.Environ()
		cmd.Stdout = &logs
		cmd.Stderr = &logs
		if err := cmd.Run(); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("桌面端更新失败: "+err.Error()))
			return
		}
		out := logs.String()
		if len(out) > 4000 {
			out = out[len(out)-4000:]
		}
		c.JSON(http.StatusOK, dto.OK[string](out, "桌面端已更新"))
		return
	}
	// 检测是否在Docker Compose环境中运行
	isComposeMode := false
	if os.Getenv("DOCKER_ENVIRONMENT") == "compose" {
		isComposeMode = true
	}

	dockerHost := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	dockerCmd := "docker"
	if dockerHost != "" {
		dockerCmd = "docker -H '" + dockerHost + "'"
	}
	if dockerHost == "" {
		if _, err := os.Stat("/var/run/docker.sock"); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("Docker 未就绪: 缺少 /var/run/docker.sock"))
			return
		}
	}

	if isComposeMode {
		composeCmd := "docker compose"
		if err := run(10*time.Second, composeCmd+" version"); err != nil {
			if err2 := run(10*time.Second, "docker-compose --version"); err2 == nil {
				composeCmd = "docker-compose"
			} else {
				c.JSON(http.StatusOK, dto.Fail[string]("Docker Compose 未就绪: "+err.Error()))
				return
			}
		}
		if err := run(2*time.Minute, composeCmd+" pull"); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("拉取镜像失败: "+err.Error()))
			return
		}
		if err := run(2*time.Minute, composeCmd+" up -d --force-recreate"); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("重启服务失败: "+err.Error()))
			return
		}
	} else {
		// 标准Docker模式更新流程
		if err := run(2*time.Minute, dockerCmd+" pull "+image); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("拉取镜像失败: "+err.Error()))
			return
		}
		_ = run(30*time.Second, dockerCmd+" ps -a --filter name=^"+name+"$ --format '{{.ID}}' | xargs -r "+dockerCmd+" stop")
		_ = run(30*time.Second, dockerCmd+" ps -a --filter name=^"+name+"$ --format '{{.ID}}' | xargs -r "+dockerCmd+" rm")
		runCmd := dockerCmd + " run -d --name " + name + " -p " + hostPort + ":1314 -v '" + dataDir + ":/app/data' --restart unless-stopped " + image
		if err := run(2*time.Minute, runCmd); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("启动新容器失败: "+err.Error()))
			return
		}
		_ = run(30*time.Second, dockerCmd+" image prune -f || true")
	}

	out := logs.String()
	if len(out) > 4000 {
		out = out[len(out)-4000:]
	}
	c.JSON(http.StatusOK, dto.OK[string](out, "容器已升级并重启（数据已保留）"))
}

func UpdateVersionStream(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusOK, dto.Fail[string]("当前服务器不支持流式输出"))
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	write := func(m map[string]any) {
		b, _ := json.Marshal(m)
		_, _ = c.Writer.Write([]byte("data: " + string(b) + "\n\n"))
		flusher.Flush()
	}

	write(map[string]any{"type": "info", "message": "开始升级流程"})
	hasUpdate, latestTime, curTag, chkErr := computeUpgradeInfo()
	if chkErr != nil {
		write(map[string]any{"type": "error", "message": "版本检测失败: " + chkErr.Error()})
		return
	}
	write(map[string]any{"type": "info", "message": fmt.Sprintf("当前版本 %s，最新发布时间 %s", curTag, latestTime)})
	if !hasUpdate {
		write(map[string]any{"type": "info", "message": "已是最新版，无需升级"})
		write(map[string]any{"type": "done", "message": "no-upgrade"})
		return
	}

	var step = func(progress int, msg string) {
		write(map[string]any{"type": "progress", "progress": progress, "message": msg})
	}

	shellArgs := func() (string, []string) {
		if _, err := exec.LookPath("bash"); err == nil {
			return "bash", []string{"-lc"}
		}
		return "sh", []string{"-c"}
	}
	runStreaming := func(timeout time.Duration, label, cmdStr string) error {
		step(0, "执行: "+label)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		prog, args := shellArgs()
		cmd := exec.CommandContext(ctx, prog, append(args, cmdStr)...)
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			return err
		}

		done := make(chan struct{}, 2)
		go func() {
			defer func() { done <- struct{}{} }()
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				write(map[string]any{"type": "log", "message": fmt.Sprintf("[%s] %s", label, line)})
			}
		}()
		go func() {
			defer func() { done <- struct{}{} }()
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				write(map[string]any{"type": "log", "message": fmt.Sprintf("[%s] %s", label, line)})
			}
		}()
		err := cmd.Wait()
		<-done
		<-done
		return err
	}

	image := strings.TrimSpace(os.Getenv("UPDATE_IMAGE"))
	if image == "" {
		image = "noise233/echo-noise:latest"
	}
	name := strings.TrimSpace(os.Getenv("CONTAINER_NAME"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("ECH0_CONTAINER_NAME"))
	}
	if name == "" {
		name = "Ech0-Noise"
	}
	hostPort := strings.TrimSpace(os.Getenv("HTTP_PORT"))
	if hostPort == "" {
		hostPort = "1314"
	}
	wd, _ := os.Getwd()
	dataDir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dataDir == "" {
		candidates := []string{"/opt/data", filepath.Join(wd, "data"), "/data"}
		for _, d := range candidates {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				dataDir = d
				break
			}
		}
		if dataDir == "" {
			dataDir = filepath.Join(wd, "data")
			_ = os.MkdirAll(dataDir, 0755)
		}
	}

	isComposeMode := false
	if os.Getenv("DOCKER_ENVIRONMENT") == "compose" {
		isComposeMode = true
	}

	if err := runStreaming(10*time.Second, "docker", "docker --version"); err != nil {
		if custom := strings.TrimSpace(os.Getenv("DESKTOP_UPDATE_CMD")); custom != "" {
			step(20, "桌面端更新执行...")
			if err2 := runStreaming(5*time.Minute, "desktop", custom); err2 != nil {
				write(map[string]any{"type": "error", "message": "桌面端更新失败: " + err2.Error()})
				return
			}
			write(map[string]any{"type": "success", "message": "桌面端已更新"})
			write(map[string]any{"type": "done", "message": "desktop-updated"})
			return
		}
		write(map[string]any{"type": "error", "message": "Docker 未就绪: " + err.Error()})
		return
	}
	dockerHost := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	dockerCmd := "docker"
	if dockerHost != "" {
		dockerCmd = "docker -H '" + dockerHost + "'"
	}
	if dockerHost == "" {
		if _, err := os.Stat("/var/run/docker.sock"); err != nil {
			write(map[string]any{"type": "error", "message": "Docker 未就绪: 缺少 /var/run/docker.sock"})
			return
		}
	}

	if isComposeMode {
		composeCmd := "docker compose"
		if err := runStreaming(10*time.Second, "compose", composeCmd+" version"); err != nil {
			if err2 := runStreaming(10*time.Second, "compose", "docker-compose --version"); err2 == nil {
				composeCmd = "docker-compose"
			} else {
				write(map[string]any{"type": "error", "message": "Docker Compose 未就绪: " + err.Error()})
				return
			}
		}
		step(30, "拉取镜像...")
		if err := runStreaming(3*time.Minute, "compose", composeCmd+" pull"); err != nil {
			write(map[string]any{"type": "error", "message": "拉取镜像失败: " + err.Error()})
			return
		}
		step(70, "重启服务...")
		if err := runStreaming(2*time.Minute, "compose", composeCmd+" up -d --force-recreate"); err != nil {
			write(map[string]any{"type": "error", "message": "重启服务失败: " + err.Error()})
			return
		}
		write(map[string]any{"type": "success", "message": "容器已升级并重启（数据已保留）"})
		step(100, "完成")
		write(map[string]any{"type": "done", "message": "ok"})
		return
	}
	step(25, "拉取镜像...")
	if err := runStreaming(3*time.Minute, "pull", dockerCmd+" pull "+image); err != nil {
		write(map[string]any{"type": "error", "message": "拉取镜像失败: " + err.Error()})
		return
	}
	step(45, "停止旧容器...")
	_ = runStreaming(30*time.Second, "stop", dockerCmd+" ps -a --filter name=^"+name+"$ --format '{{.ID}}' | xargs -r "+dockerCmd+" stop")
	step(55, "移除旧容器...")
	_ = runStreaming(30*time.Second, "rm", dockerCmd+" ps -a --filter name=^"+name+"$ --format '{{.ID}}' | xargs -r "+dockerCmd+" rm")
	step(75, "启动新容器...")
	runCmd := dockerCmd + " run -d --name " + name + " -p " + hostPort + ":1314 -v '" + dataDir + ":/app/data' --restart unless-stopped " + image
	if err := runStreaming(2*time.Minute, "run", runCmd); err != nil {
		write(map[string]any{"type": "error", "message": "启动新容器失败: " + err.Error()})
		return
	}
	step(90, "清理旧镜像...")
	_ = runStreaming(30*time.Second, "prune", dockerCmd+" image prune -f || true")

	write(map[string]any{"type": "success", "message": "容器已升级并重启（数据已保留）"})
	step(100, "完成")
	write(map[string]any{"type": "done", "message": "ok"})
}

// 版本升级逻辑仅通过容器镜像更新，保留数据卷；非容器桌面端由 DESKTOP_UPDATE_CMD 处理
