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
		if err := recordUserLoginAudit(c, &user, loginAuditActionLogin); err != nil {
			t.Fatalf("record ordinary user audit: %v", err)
		}
		c.Status(http.StatusNoContent)
	})
	r.GET("/audit-user-logout", func(c *gin.Context) {
		if err := recordUserLoginAudit(c, &user, loginAuditActionLogout); err != nil {
			t.Fatalf("record ordinary user logout audit: %v", err)
		}
		c.Status(http.StatusNoContent)
	})
	r.GET("/audit-admin", func(c *gin.Context) {
		if err := recordUserLoginAudit(c, &admin, loginAuditActionLogin); err != nil {
			t.Fatalf("record admin audit: %v", err)
		}
		c.Status(http.StatusNoContent)
	})

	performLoginAuditRecordRequest(r, "/audit-user", "198.51.100.7:1234", "audit-test-agent")
	performLoginAuditRecordRequest(r, "/audit-user-logout", "198.51.100.8:1234", "logout-agent")
	performLoginAuditRecordRequest(r, "/audit-admin", "203.0.113.8:5678", "admin-agent")

	var audits []models.SecurityLoginAudit
	if err := db.Order("id asc").Find(&audits).Error; err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(audits) != 2 {
		t.Fatalf("expected two ordinary user audits, got %d", len(audits))
	}
	for _, audit := range audits {
		if audit.UserID != user.ID || audit.Username != user.Username {
			t.Fatalf("unexpected audited user: %#v", audit)
		}
	}
	if audits[0].Action != loginAuditActionLogin || audits[0].IP != "198.51.100.7" {
		t.Fatalf("expected login audit first, got %#v", audits[0])
	}
	if audits[1].Action != loginAuditActionLogout || audits[1].IP != "198.51.100.8" {
		t.Fatalf("expected logout audit second, got %#v", audits[1])
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
		{UserID: user.ID, Username: user.Username, Action: loginAuditActionLogin, IP: "198.51.100.7", UserAgent: "first"},
		{UserID: other.ID, Username: other.Username, Action: loginAuditActionLogout, IP: "203.0.113.8", UserAgent: "second"},
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

	logoutOnly := decodeLoginAuditsResponse(t, performLoginAuditListRequest(r, "/security/login-audits?action=logout"))
	if len(logoutOnly) != 1 || logoutOnly[0].Username != other.Username || logoutOnly[0].Action != loginAuditActionLogout {
		t.Fatalf("expected logout audit, got %#v", logoutOnly)
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
