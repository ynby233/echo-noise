package controllers

import (
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/buildinfo"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/updates"
)

var discoverUpdates = func(c *gin.Context) updates.Report {
	metadata := buildinfo.CurrentMetadata()
	service := updates.NewDiscovery(
		&http.Client{Timeout: 10 * time.Second},
		"https://api.github.com/repos/ynby233/echo-noise",
		"https://ghcr.io/v2/ynby233/echo-noise",
		"https://ghcr.io/token?service=ghcr.io&scope=repository%3Aynby233%2Fecho-noise%3Apull",
	)
	return service.Discover(c.Request.Context(), updates.Installed{
		Revision: metadata.Revision,
		Digest:   strings.TrimSpace(os.Getenv("IMAGE_DIGEST")),
	}, updates.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH})
}

// CheckVersion exposes only public release state. Full revisions and image
// digests stay behind the primary-administrator endpoint below.
func CheckVersion(c *gin.Context) {
	report := discoverUpdates(c)
	for _, channel := range report.Channels {
		if channel.Status == updates.StatusCheckFailed {
			c.JSON(http.StatusBadGateway, dto.Fail[any]("版本检查失败，请稍后重试"))
			return
		}
	}
	if report.Source.Status == updates.StatusCheckFailed {
		c.JSON(http.StatusBadGateway, dto.Fail[any]("版本检查失败，请稍后重试"))
		return
	}
	if report.Release.Status == updates.StatusCheckFailed {
		c.JSON(http.StatusBadGateway, dto.Fail[any]("正式发行检查失败，请稍后重试"))
		return
	}
	publicRelease := gin.H{"status": report.Release.Status}
	if strings.HasPrefix(report.Release.Version, "v") && buildinfo.NormalizeIdentity(report.Release.Version) == report.Release.Version {
		publicRelease["version"] = report.Release.Version
	}

	stable := report.Channel("stable")
	channels := make([]gin.H, 0, len(report.Channels))
	hasUpdate := false
	for _, channel := range report.Channels {
		item := gin.H{
			"name":        channel.Name,
			"status":      channel.Status,
			"installable": channel.Installable,
			"hasUpdate":   channel.HasUpdate,
		}
		if channel.Name == "stable" {
			item["version"] = channel.Version
		}
		channels = append(channels, item)
		hasUpdate = hasUpdate || channel.HasUpdate
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"hasUpdate":          hasUpdate,
			"lastUpdateTime":     stable.BuiltAt,
			"currentTag":         stable.Version,
			"latestSourceStatus": report.Source.Status,
			"latestRelease":      publicRelease,
			"channels":           channels,
		},
	})
}

func GetUpdateChannels(c *gin.Context) {
	userID, ok := commentUint(c.GetUint("user_id"))
	if !ok || userID != models.PrimaryAdminUserID {
		c.JSON(http.StatusForbidden, dto.Fail[any]("仅站长可读取完整更新目标"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(discoverUpdates(c), "更新渠道读取成功"))
}

func publicVersionLabel() string { return "installed" }

func GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{"version": publicVersionLabel()},
	})
}

func UpdateVersion(c *gin.Context) { UpdateTaskInstallationUnavailable(c) }

func UpdateVersionStream(c *gin.Context) {
	c.JSON(http.StatusGone, dto.Fail[any]("旧更新流已退役；请创建一次更新任务并只读查询进度"))
}
