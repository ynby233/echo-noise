package main

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/controllers"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

// Only the isolated binary redirects discovery requests. The real controllers,
// official repository allowlist and task authentication stay in use.
type channelTransport struct {
	base  http.RoundTripper
	proxy *url.URL
}

func (t channelTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != "api.github.com" && request.URL.Host != "ghcr.io" {
		return t.base.RoundTrip(request)
	}
	copy := request.Clone(request.Context())
	target := *request.URL
	target.Scheme, target.Host = t.proxy.Scheme, t.proxy.Host
	copy.URL, copy.Host = &target, target.Host
	return t.base.RoundTrip(copy)
}

func channelRoutes(router *gin.Engine, db *gorm.DB, proxy string) {
	parsed, err := url.Parse(proxy)
	must(err)
	if parsed.Scheme != "http" || !strings.HasPrefix(parsed.Host, "127.0.0.1:") {
		panic("fixture channel proxy must be loopback")
	}
	http.DefaultTransport = channelTransport{http.DefaultTransport, parsed}
	must(db.AutoMigrate(&models.User{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}))
	must(db.FirstOrCreate(&models.User{ID: 1, Username: "fixture-owner", IsAdmin: true}, "id = ?", 1).Error)
	primary := router.Group("/fixture/channels", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		_ = os.Setenv("IMAGE_DIGEST", c.GetHeader("X-Fixture-Installed-Digest"))
		c.Next()
	})
	primary.GET("", controllers.GetUpdates)
	primary.PUT("/preference", controllers.UpdateFollowChannel)
	primary.POST("/install", controllers.CreateUpdateTask)
}
