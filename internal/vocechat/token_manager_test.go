package vocechat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminTokenManagerUsesConfiguredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called when AdminToken is configured")
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	manager := NewAdminTokenManager(client, Config{AdminToken: " configured-token "})
	token, err := manager.GetToken(context.Background())
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if token != "configured-token" {
		t.Fatalf("token = %q, want configured-token", token)
	}
}

func TestAdminTokenManagerLogsInAndCachesToken(t *testing.T) {
	loginCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loginCount++
		if r.Method != http.MethodPost || r.URL.Path != "/api/token/login" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}

		var body struct {
			Credential struct {
				Type     string `json:"type"`
				Email    string `json:"email"`
				Password string `json:"password"`
			} `json:"credential"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode login body: %v", err)
		}
		if body.Credential.Type != "password" || body.Credential.Email != "admin@vc.com" || body.Credential.Password != "secret" {
			t.Fatalf("credential = %#v", body.Credential)
		}
		_, _ = w.Write([]byte(`{"token":"token-1","refresh_token":"refresh-1","expired_in":3600,"user":{"uid":1,"is_admin":true}}`))
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	manager := NewAdminTokenManager(client, Config{AdminUsername: "admin@vc.com", AdminPassword: "secret", AdminToken: "old-static-token"})
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	manager.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		token, err := manager.GetToken(context.Background())
		if err != nil {
			t.Fatalf("get token %d: %v", i, err)
		}
		if token != "token-1" {
			t.Fatalf("token %d = %q, want token-1", i, token)
		}
	}
	if loginCount != 1 {
		t.Fatalf("loginCount = %d, want 1", loginCount)
	}
}

func TestAdminTokenManagerRenewsNearExpiry(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/token/login":
			_, _ = w.Write([]byte(`{"token":"token-1","refresh_token":"refresh-1","expired_in":40,"user":{"uid":1,"is_admin":true}}`))
		case "/api/token/renew":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode renew body: %v", err)
			}
			if body["token"] != "token-1" || body["refresh_token"] != "refresh-1" {
				t.Fatalf("renew body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"token":"token-2","refresh_token":"refresh-2","expired_in":3600}`))
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	manager := NewAdminTokenManager(client, Config{AdminUsername: "admin@vc.com", AdminPassword: "secret", AdminToken: "old-static-token"})
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	manager.now = func() time.Time { return now }

	first, err := manager.GetToken(context.Background())
	if err != nil {
		t.Fatalf("first get token: %v", err)
	}
	if first != "token-1" {
		t.Fatalf("first token = %q", first)
	}
	now = now.Add(20 * time.Second)
	second, err := manager.GetToken(context.Background())
	if err != nil {
		t.Fatalf("second get token: %v", err)
	}
	if second != "token-2" {
		t.Fatalf("second token = %q", second)
	}
	if len(paths) != 2 || paths[0] != "/api/token/login" || paths[1] != "/api/token/renew" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestAdminTokenManagerFallsBackToLoginWhenRenewFails(t *testing.T) {
	loginCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/token/login":
			loginCount++
			if loginCount == 1 {
				_, _ = w.Write([]byte(`{"token":"token-1","refresh_token":"refresh-1","expired_in":1,"user":{"uid":1,"is_admin":true}}`))
				return
			}
			_, _ = w.Write([]byte(`{"token":"token-2","refresh_token":"refresh-2","expired_in":3600,"user":{"uid":1,"is_admin":true}}`))
		case "/api/token/renew":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"expired"}`))
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	manager := NewAdminTokenManager(client, Config{AdminUsername: "admin@vc.com", AdminPassword: "secret"})
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	manager.now = func() time.Time { return now }

	if token, err := manager.GetToken(context.Background()); err != nil || token != "token-1" {
		t.Fatalf("first token=%q err=%v", token, err)
	}
	now = now.Add(2 * time.Second)
	if token, err := manager.GetToken(context.Background()); err != nil || token != "token-2" {
		t.Fatalf("second token=%q err=%v", token, err)
	}
	if loginCount != 2 {
		t.Fatalf("loginCount = %d, want 2", loginCount)
	}
}

func TestAdminTokenManagerExplainsMissingAdminEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/token/login" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	manager := NewAdminTokenManager(client, Config{AdminUsername: "Noise", AdminPassword: "secret"})
	_, err := manager.GetToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "管理员邮箱不存在") {
		t.Fatalf("get token err = %v, want admin email hint", err)
	}
}

func TestAdminTokenManagerRejectsIncompleteCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request = %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	for _, config := range []Config{
		{AdminUsername: "admin@vc.com", AdminPassword: " ", AdminToken: "old-static-token"},
		{AdminUsername: " ", AdminPassword: "secret", AdminToken: "old-static-token"},
		{AdminToken: " "},
	} {
		manager := NewAdminTokenManager(mustTestClient(t, server), config)
		token, err := manager.GetToken(context.Background())
		want := "管理员邮箱或密码未配置完整"
		if strings.TrimSpace(config.AdminUsername) == "" && strings.TrimSpace(config.AdminPassword) == "" {
			want = "管理员凭据未配置"
		}
		if token != "" || err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("get token = %q, err = %v, want missing credentials error", token, err)
		}
	}
}

func TestAdminTokenManagerDoesNotFallBackToStaticTokenWhenPasswordRejected(t *testing.T) {
	loginCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loginCount++
		if r.Method != http.MethodPost || r.URL.Path != "/api/token/login" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid password"}`))
	}))
	defer server.Close()

	manager := NewAdminTokenManager(mustTestClient(t, server), Config{
		AdminUsername: "admin@vc.com",
		AdminPassword: "wrong-password",
		AdminToken:    "old-static-token",
	})
	token, err := manager.GetToken(context.Background())
	if token != "" || err == nil {
		t.Fatalf("get token = %q, err = %v, want login failure", token, err)
	}
	if loginCount != 1 {
		t.Fatalf("loginCount = %d, want 1", loginCount)
	}
}

func TestAdminTokenManagerRejectsNonAdminLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/token/login" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"token":"non-admin-token","refresh_token":"refresh-1","expired_in":3600,"user":{"uid":2,"is_admin":false}}`))
	}))
	defer server.Close()

	manager := NewAdminTokenManager(mustTestClient(t, server), Config{
		AdminUsername: "user@vc.com",
		AdminPassword: "secret",
		AdminToken:    "old-static-token",
	})
	token, err := manager.GetToken(context.Background())
	if token != "" || err == nil || !strings.Contains(err.Error(), "管理员邮箱对应账号不是管理员") {
		t.Fatalf("get token = %q, err = %v, want non-admin error", token, err)
	}
	if manager.token != "" || manager.refreshToken != "" {
		t.Fatal("non-admin login must not be cached")
	}
}
