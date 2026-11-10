package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestAccessAndSiteVisitLogsUseOnlyVerifiedIdentity(t *testing.T) {
	cases := []struct {
		name          string
		contextSource string
		session       string
		bearer        string
		wantSource    string
		wantUser      bool
	}{
		{name: "anonymous forged source", wantSource: "unknown"},
		{name: "invalid bearer forged source", bearer: "invalid-token", wantSource: "unknown"},
		{name: "verified session ignores stale profile", session: "valid", wantSource: "session", wantUser: true},
		{name: "expired session", session: "expired", wantSource: "unknown"},
		{name: "deleted session user", session: "deleted", wantSource: "unknown"},
		{name: "verified bearer", bearer: "verified-token", wantSource: "token", wantUser: true},
		{name: "authenticated session context", contextSource: "session", wantSource: "session", wantUser: true},
		{name: "authenticated token context", contextSource: "token", wantSource: "token", wantUser: true},
		{name: "authenticated AI context", contextSource: "ai_token", wantSource: "ai_token", wantUser: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, r, _, _ := setupCommentAccountTest(t)
			if err := db.AutoMigrate(&models.SecurityConfig{}, &models.SecurityAccessLog{}, &models.SecuritySiteVisitLog{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.SecurityConfig{AccessLogEnabled: true, SiteVisitLogEnabled: true}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.SiteConfig{DelegatedAdminLoginExpireHours: 1}).Error; err != nil {
				t.Fatal(err)
			}
			issuedAt := time.Now().Add(-time.Minute)
			user := models.User{Username: "verified-log-user", IsAdmin: true, Token: "verified-token", LoginIssuedAt: &issuedAt}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			middleware.InvalidateAccessLogConfigCache()
			t.Cleanup(func() {
				flushAccessLogWrites(t)
				middleware.InvalidateAccessLogConfigCache()
			})
			r.Use(func(c *gin.Context) {
				if tc.contextSource != "" {
					// Simulate a trusted authentication middleware, never request headers.
					c.Set("user_id", user.ID)
					c.Set("username", user.Username)
					c.Set("is_admin", user.IsAdmin)
					c.Set("auth_via", tc.contextSource)
				}
				if tc.session != "" {
					session := sessions.Default(c)
					session.Set("user_id", user.ID)
					session.Set("username", "stale-session-name")
					session.Set("is_admin", false)
					session.Set("login_issued_at", issuedAt.Unix())
					if tc.session == "expired" {
						session.Set("login_issued_at", time.Now().Add(-365*24*time.Hour).Unix())
					}
					if tc.session == "deleted" {
						session.Set("user_id", user.ID+1000)
					}
				}
				c.Next()
			})
			r.Use(middleware.AccessLogMiddleware())
			r.GET("/", func(c *gin.Context) {
				// Exercise the shared read-only cache before the log writer runs.
				middleware.ResolveReadIdentity(c)
				if tc.contextSource == "" {
					if _, exists := c.Get("user_id"); exists {
						t.Fatal("read-only identity must not populate authorization user_id")
					}
				}
				c.String(http.StatusOK, "home")
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept", "text/html")
			req.Header.Set("Auth-Via", "ai_token")
			req.Header.Set("X-Auth-Via", "ai_token")
			req.Header.Set("X-Authentication-Source", "ai_token")
			req.Header.Set("X-User-ID", "1")
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			flushAccessLogWrites(t)
			var access []models.SecurityAccessLog
			var visits []models.SecuritySiteVisitLog
			if err := db.Find(&access).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Find(&visits).Error; err != nil {
				t.Fatal(err)
			}
			if len(access) != 1 || len(visits) != 1 {
				t.Fatalf("expected one access and site visit log, got %d and %d", len(access), len(visits))
			}
			if access[0].AuthVia != tc.wantSource || visits[0].AuthVia != tc.wantSource {
				t.Fatalf("sources = %q, %q; want %q", access[0].AuthVia, visits[0].AuthVia, tc.wantSource)
			}
			if tc.wantUser {
				if access[0].UserID != user.ID || visits[0].UserID != user.ID || access[0].Username != user.Username || visits[0].Username != user.Username || !access[0].IsAdmin || !visits[0].IsAdmin {
					t.Fatalf("logs must use verified current profile: access=%#v visit=%#v", access[0], visits[0])
				}
			} else if access[0].UserID != 0 || visits[0].UserID != 0 || access[0].Username != "" || visits[0].Username != "" || access[0].IsAdmin || visits[0].IsAdmin {
				t.Fatalf("unverified identity leaked into logs: access=%#v visit=%#v", access[0], visits[0])
			}
		})
	}
}
