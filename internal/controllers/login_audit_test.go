package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestLoginAuditRecordsOnlyOrdinaryUsers(t *testing.T) {
	db, r, user, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.SecurityLoginAudit{}); err != nil {
		t.Fatalf("migrate login audits: %v", err)
	}

	admin := models.User{Username: "root", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	r.GET("/audit-user", func(c *gin.Context) {
		if err := recordUserLoginAudit(c, &user); err != nil {
			t.Fatalf("record ordinary user audit: %v", err)
		}
		c.Status(http.StatusNoContent)
	})
	r.GET("/audit-admin", func(c *gin.Context) {
		if err := recordUserLoginAudit(c, &admin); err != nil {
			t.Fatalf("record admin audit: %v", err)
		}
		c.Status(http.StatusNoContent)
	})

	performLoginAuditRecordRequest(r, "/audit-user", "198.51.100.7:1234", "audit-test-agent")
	performLoginAuditRecordRequest(r, "/audit-admin", "203.0.113.8:5678", "admin-agent")

	var audits []models.SecurityLoginAudit
	if err := db.Find(&audits).Error; err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected one ordinary user audit, got %d", len(audits))
	}
	if audits[0].UserID != user.ID || audits[0].Username != user.Username {
		t.Fatalf("unexpected audited user: %#v", audits[0])
	}
	if audits[0].IP != "198.51.100.7" {
		t.Fatalf("expected ClientIP remote address, got %q", audits[0].IP)
	}
	if audits[0].UserAgent != "audit-test-agent" {
		t.Fatalf("expected user agent to be recorded, got %q", audits[0].UserAgent)
	}
}

func TestGetLoginAuditsFiltersByUsernameAndIP(t *testing.T) {
	db, r, user, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.SecurityLoginAudit{}); err != nil {
		t.Fatalf("migrate login audits: %v", err)
	}
	other := models.User{Username: "bob"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}

	rows := []models.SecurityLoginAudit{
		{UserID: user.ID, Username: user.Username, IP: "198.51.100.7", UserAgent: "first"},
		{UserID: other.ID, Username: other.Username, IP: "203.0.113.8", UserAgent: "second"},
	}
	for _, row := range rows {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("create audit row: %v", err)
		}
	}

	r.GET("/security/login-audits", GetLoginAudits)
	filtered := decodeLoginAuditsResponse(t, performLoginAuditListRequest(r, "/security/login-audits?username=alice&ip=198.51.100.7"))
	if len(filtered) != 1 || filtered[0].Username != user.Username || filtered[0].IP != "198.51.100.7" {
		t.Fatalf("expected filtered alice audit, got %#v", filtered)
	}

	all := decodeLoginAuditsResponse(t, performLoginAuditListRequest(r, "/security/login-audits"))
	if len(all) != 2 {
		t.Fatalf("expected all audits, got %d", len(all))
	}
}

func performLoginAuditRecordRequest(r http.Handler, path string, remoteAddr string, userAgent string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	req.Header.Set("User-Agent", userAgent)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func performLoginAuditListRequest(r http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeLoginAuditsResponse(t *testing.T, w *httptest.ResponseRecorder) []models.SecurityLoginAudit {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int                         `json:"code"`
		Data []models.SecurityLoginAudit `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 1 {
		t.Fatalf("expected success response, got %#v", resp)
	}
	return resp.Data
}
