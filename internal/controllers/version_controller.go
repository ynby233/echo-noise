package controllers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
)

const releaseRepositoryAPI = "https://api.github.com/repos/ynby233/echo-noise"

type repositoryVersionInfo struct {
	TagName     string
	PublishedAt string
}

func getGitHubJSON(client *http.Client, rawURL string, target any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "echo-noise")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("GitHub API 返回 %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

// Prefer a published GitHub Release. When none exists, use the newest tag's
// commit time so repositories that publish tags directly still have a version.
func latestRepositoryVersion(client *http.Client, baseURL string) (repositoryVersionInfo, error) {
	var release struct {
		TagName     string `json:"tag_name"`
		PublishedAt string `json:"published_at"`
	}
	if err := getGitHubJSON(client, baseURL+"/releases/latest", &release); err == nil && strings.TrimSpace(release.TagName) != "" && strings.TrimSpace(release.PublishedAt) != "" {
		return repositoryVersionInfo{TagName: release.TagName, PublishedAt: release.PublishedAt}, nil
	}

	var tags []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := getGitHubJSON(client, baseURL+"/tags?per_page=1", &tags); err != nil {
		return repositoryVersionInfo{}, err
	}
	if len(tags) == 0 || strings.TrimSpace(tags[0].Name) == "" || strings.TrimSpace(tags[0].Commit.SHA) == "" {
		return repositoryVersionInfo{}, fmt.Errorf("仓库没有可用的 Release 或 Tag")
	}
	var commit struct {
		Commit struct {
			Author struct {
				Date string `json:"date"`
			} `json:"author"`
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := getGitHubJSON(client, baseURL+"/commits/"+url.PathEscape(tags[0].Commit.SHA), &commit); err != nil {
		return repositoryVersionInfo{}, err
	}
	publishedAt := strings.TrimSpace(commit.Commit.Committer.Date)
	if publishedAt == "" {
		publishedAt = strings.TrimSpace(commit.Commit.Author.Date)
	}
	if publishedAt == "" {
		return repositoryVersionInfo{}, fmt.Errorf("仓库 Tag 缺少发布时间")
	}
	return repositoryVersionInfo{TagName: tags[0].Name, PublishedAt: publishedAt}, nil
}

// 检查版本更新
func CheckVersion(c *gin.Context) {
	client := &http.Client{Timeout: 5 * time.Second}
	latest, err := latestRepositoryVersion(client, releaseRepositoryAPI)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "获取仓库版本失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"hasUpdate": false, "lastUpdateTime": latest.PublishedAt, "currentTag": latest.TagName}})
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
