package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func TestPublicSearchAttributesBearerWithoutGrantingSessionAuthority(t *testing.T) {
	db, router, user, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AICredential{}, &models.SecurityConfig{}, &models.SecurityAccessLog{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SecurityConfig{AccessLogEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&user).Updates(map[string]any{"token": "search-ordinary-token", "login_issued_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	_, aiToken, err := services.CreateAICredential(user.ID, "search")
	if err != nil {
		t.Fatal(err)
	}
	message := models.Message{UserID: user.ID, Content: "search-private-sentinel", Visibility: "private", Private: true}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	middleware.InvalidateAccessLogConfigCache()
	t.Cleanup(func() {
		flushAccessLogWrites(t)
		middleware.InvalidateAccessLogConfigCache()
	})
	router.Use(middleware.AccessLogMiddleware())
	router.GET("/api/messages/search", func(c *gin.Context) {
		SearchMessages(c)
		for _, key := range []string{"user_id", "username", "is_admin", "auth_via", "credential_id"} {
			if _, exists := c.Get(key); exists {
				t.Errorf("public search must not set authorization context %s", key)
			}
		}
	})
	for _, tc := range []struct {
		header  string
		via     string
		visible bool
	}{
		{"Bearer search-ordinary-token", "token", true},
		{"Bearer " + aiToken, "ai_token", true},
		{"search-ordinary-token", "unknown", false},
		{"Bearer invalid", "unknown", false},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/messages/search?keyword=search-private-sentinel", nil)
		request.Header.Set("Authorization", tc.header)
		request.Header.Set("X-Auth-Via", "session")
		request.Header.Set("X-User-ID", "1")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":1`) {
			t.Fatalf("search failed: %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), message.Content) != tc.visible {
			t.Fatalf("unexpected private search visibility for %q: %s", tc.header, response.Body.String())
		}
		flushAccessLogWrites(t)
		var record models.SecurityAccessLog
		if err := db.Order("id DESC").First(&record).Error; err != nil {
			t.Fatal(err)
		}
		if record.AuthVia != tc.via || (record.UserID == user.ID) != tc.visible {
			t.Fatalf("search attribution: user=%d source=%q", record.UserID, record.AuthVia)
		}
	}
}
