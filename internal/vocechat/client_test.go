package vocechat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientLoginWithPasswordRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/token/login" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		credential, ok := body["credential"].(map[string]interface{})
		if !ok {
			t.Fatalf("credential missing from body: %#v", body)
		}
		if credential["type"] != "password" || credential["email"] != "admin@vc.com" || credential["password"] != "secret" {
			t.Fatalf("credential = %#v", credential)
		}
		if body["device"] != defaultDevice {
			t.Fatalf("device = %v, want %s", body["device"], defaultDevice)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"server_id":"server-1","token":"access-token","refresh_token":"refresh-token","expired_in":3600,"user":{"uid":1,"email":"admin@vc.com","name":"admin","is_admin":true}}`))
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	response, err := client.LoginWithPassword(context.Background(), " admin@vc.com ", "secret", "")
	if err != nil {
		t.Fatalf("login with password: %v", err)
	}
	if response.Token != "access-token" || response.RefreshToken != "refresh-token" || response.User.Email != "admin@vc.com" {
		t.Fatalf("response = %#v", response)
	}
}

func TestClientAdminUserRequestsUseAPIKey(t *testing.T) {
	var sawCreate bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(apiKeyHeader) != "admin-token" {
			t.Fatalf("%s = %q, want admin-token", apiKeyHeader, r.Header.Get(apiKeyHeader))
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/user":
			sawCreate = true
			var body CreateUserRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create user body: %v", err)
			}
			if body.Email != "Tom@vc.com" || body.Password != "pw" || body.Name != "Tom" || body.Gender != 0 || body.IsAdmin {
				t.Fatalf("create body = %#v", body)
			}
			if body.Language != "zh-CN" {
				t.Fatalf("language = %q, want zh-CN", body.Language)
			}
			_, _ = w.Write([]byte(`{"uid":9,"email":"Tom@vc.com","name":"Tom","gender":0,"is_admin":false}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/admin/user/9":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	created, err := client.CreateUser(context.Background(), "admin-token", CreateUserRequest{
		Email:    "Tom@vc.com",
		Password: "pw",
		Name:     "Tom",
		Gender:   0,
		IsAdmin:  false,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if created.UID != 9 || !sawCreate {
		t.Fatalf("created = %#v sawCreate=%v", created, sawCreate)
	}
	if err := client.DeleteUser(context.Background(), "admin-token", 9); err != nil {
		t.Fatalf("delete user: %v", err)
	}
}

func TestClientCheckEmailAndThirdPartyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/check_email":
			if r.URL.Query().Get("email") != "Tom@vc.com" {
				t.Fatalf("email query = %q", r.URL.Query().Get("email"))
			}
			_, _ = w.Write([]byte(`true`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/create_third_party_key":
			if r.Header.Get(thirdPartySecretKey) != "third-secret" {
				t.Fatalf("%s = %q", thirdPartySecretKey, r.Header.Get(thirdPartySecretKey))
			}
			_, _ = w.Write([]byte(`"login-key"`))
		default:
			t.Fatalf("unexpected request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	available, err := client.CheckEmail(context.Background(), " Tom@vc.com ")
	if err != nil {
		t.Fatalf("check email: %v", err)
	}
	if !available {
		t.Fatal("email should be available")
	}

	key, err := client.CreateThirdPartyKey(context.Background(), "third-secret", "42", "Tom")
	if err != nil {
		t.Fatalf("create third-party key: %v", err)
	}
	if key != "login-key" {
		t.Fatalf("key = %q, want login-key", key)
	}
}

func TestClientSendMarkdownToUserUsesBotAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/bot/send_to_user/42" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get(apiKeyHeader) != "bot-token" {
			t.Fatalf("%s = %q, want bot-token", apiKeyHeader, r.Header.Get(apiKeyHeader))
		}
		if r.Header.Get("Content-Type") != "text/markdown" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "hello **world**" {
			t.Fatalf("body = %q", string(body))
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	if err := client.SendMarkdownToUser(context.Background(), " bot-token ", " 42 ", " hello **world** "); err != nil {
		t.Fatalf("send markdown: %v", err)
	}
}

func TestClientReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"reason":"email_conflict"}`))
	}))
	defer server.Close()

	client := mustTestClient(t, server)
	_, err := client.CreateUser(context.Background(), "admin-token", CreateUserRequest{})
	if err == nil {
		t.Fatal("expected API error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusConflict || apiErr.Path != "/admin/user" {
		t.Fatalf("apiErr = %#v", apiErr)
	}
}

func mustTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	client, err := NewClientWithHTTPClient(Config{BaseURL: server.URL + "/"}, server.Client())
	if err != nil {
		t.Fatalf("new test client: %v", err)
	}
	if client.baseURL != server.URL+"/api" {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, server.URL+"/api")
	}
	return client
}
