package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

// Exercise the real HTTP adapters together: a health check must not succeed
// with one credential while registration and account management use another.
func TestVoceChatAdminCredentialsConsistentAcrossHealthAndAccountLifecycle(t *testing.T) {
	for _, mode := range []string{"password", "password_with_expired_token", "token_only"} {
		t.Run(mode, func(t *testing.T) {
			db := setupUserServiceTestDB(t)
			createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
			t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
			admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("local-admin"), IsAdmin: true})
			users := map[int64]vocechat.User{}
			passwords := map[int64]string{}
			var nextUID int64 = 40
			rejectedAdminRequests, adminLogins, managementCalls, createdCount, deletedCount := 0, 0, 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost && r.URL.Path == "/api/token/login" {
					var body struct {
						Credential struct{ Type, Email, Password string } `json:"credential"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Credential.Type != "password" {
						t.Error("invalid password login request")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if body.Credential.Email == "admin@vc.com" && body.Credential.Password == "remote-admin" {
						adminLogins++
						_ = json.NewEncoder(w).Encode(vocechat.LoginResponse{Token: "fresh-admin-token", RefreshToken: "refresh-admin", ExpiredIn: 3600, User: vocechat.UserInfo{UID: 1, Email: "admin@vc.com", IsAdmin: true}})
						return
					}
					for uid, user := range users {
						if strings.EqualFold(user.Email, body.Credential.Email) && passwords[uid] == body.Credential.Password {
							_ = json.NewEncoder(w).Encode(vocechat.LoginResponse{Token: "user-token", User: vocechat.UserInfo{UID: uid, Email: user.Email, Name: user.Name}})
							return
						}
					}
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if !strings.HasPrefix(r.URL.Path, "/api/admin/user") {
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if r.Header.Get("X-API-Key") != "fresh-admin-token" {
					rejectedAdminRequests++
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte("authorization error"))
					return
				}
				managementCalls++
				if r.URL.Path == "/api/admin/user" {
					switch r.Method {
					case http.MethodGet:
						list := []vocechat.User{}
						for _, user := range users {
							list = append(list, user)
						}
						_ = json.NewEncoder(w).Encode(list)
					case http.MethodPost:
						var request vocechat.CreateUserRequest
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						nextUID++
						user := vocechat.User{UID: nextUID, Email: request.Email, Name: request.Name}
						users[user.UID], passwords[user.UID] = user, request.Password
						createdCount++
						_ = json.NewEncoder(w).Encode(user)
					default:
						t.Errorf("unexpected collection method %s", r.Method)
						w.WriteHeader(http.StatusMethodNotAllowed)
					}
					return
				}
				uid, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/admin/user/"), 10, 64)
				user, ok := users[uid]
				if err != nil || !ok {
					http.NotFound(w, r)
					return
				}
				switch r.Method {
				case http.MethodGet:
					_ = json.NewEncoder(w).Encode(user)
				case http.MethodPut:
					var update vocechat.UpdateUserRequest
					if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if update.Password != nil {
						passwords[uid] = *update.Password
					}
					_ = json.NewEncoder(w).Encode(user)
				case http.MethodDelete:
					delete(users, uid)
					delete(passwords, uid)
					deletedCount++
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected user method %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			updates := map[string]interface{}{"voce_chat_base_url": server.URL, "voce_chat_admin_username": "admin@vc.com", "voce_chat_admin_password": "remote-admin", "voce_chat_admin_token": ""}
			if mode == "password_with_expired_token" {
				updates["voce_chat_admin_token"] = "expired-admin-token"
			}
			if mode == "token_only" {
				updates["voce_chat_admin_username"], updates["voce_chat_admin_password"], updates["voce_chat_admin_token"] = "", "", "fresh-admin-token"
			}
			if err := db.Model(&models.SiteConfig{}).Where("id = ?", 1).Updates(updates).Error; err != nil {
				t.Fatal(err)
			}
			health, err := CheckVoceChatHealth(context.Background())
			if err != nil || health["lastHealthStatus"] != "ok" {
				t.Fatalf("health check failed: %v", err)
			}
			result, err := RegisterWithResult(dto.RegisterDto{Username: "approved_user", Password: "app-password"})
			if err != nil {
				t.Fatal(err)
			}
			application, err := repository.GetRegistrationApplicationByApplicationID(result.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			if application.VoceChatSyncStatus != models.VoceChatSyncStatusCreated || application.VoceChatEmail == "" {
				t.Fatalf("registration failed after health passed: status=%s error=%s", application.VoceChatSyncStatus, application.VoceChatSyncError)
			}
			created, err := ApproveRegistrationApplication(application.ID, admin.ID, "approve")
			if err != nil {
				t.Fatal(err)
			}
			if created.VoceChatSyncStatus != models.VoceChatSyncStatusLinked || created.VoceChatEmail != application.VoceChatEmail {
				t.Fatal("approval lost VC binding")
			}
			_, config, err := loadRuntimePolicyAndVoceChatConfig()
			if err != nil {
				t.Fatal(err)
			}
			uid, err := strconv.ParseInt(created.VoceChatUserID, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			changedPassword := "updated-app-password"
			if _, err := defaultVoceChatAdminUpdateUser(context.Background(), config, uid, vocechat.UpdateUserRequest{Password: &changedPassword}); err != nil {
				t.Fatalf("admin password update: %v", err)
			}
			if passwords[uid] != changedPassword {
				t.Fatal("remote password was not updated")
			}
			result, err = RegisterWithResult(dto.RegisterDto{Username: "rejected_user", Password: "app-password"})
			if err != nil {
				t.Fatal(err)
			}
			application, err = repository.GetRegistrationApplicationByApplicationID(result.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			if err := RejectRegistrationApplication(application.ID, admin.ID, "reject"); err != nil {
				t.Fatalf("reject and remove pre-created account: %v", err)
			}
			if err := db.Create(&models.Setting{AllowRegistration: true, AutoApproveRegistration: true}).Error; err != nil {
				t.Fatal(err)
			}
			result, err = RegisterWithResult(dto.RegisterDto{Username: "auto_approved_user", Password: "app-password"})
			if err != nil || !result.AutoApproved {
				t.Fatalf("auto approval failed: auto=%v err=%v", result.AutoApproved, err)
			}
			if rejectedAdminRequests != 0 || managementCalls == 0 || createdCount != 3 || deletedCount != 1 {
				t.Fatalf("unexpected management lifecycle: rejected=%d calls=%d creates=%d deletes=%d", rejectedAdminRequests, managementCalls, createdCount, deletedCount)
			}
			if (mode == "token_only" && adminLogins != 0) || (mode != "token_only" && adminLogins == 0) {
				t.Fatal("incorrect management credential selection")
			}
		})
	}
}

func TestVoceChatAdminAuthorizationFailureDoesNotBlameUserPassword(t *testing.T) {
	for _, path := range []string{"/admin/user", "/api/admin/user/42"} {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
			err := &vocechat.APIError{StatusCode: status, Path: path, Method: http.MethodGet, Body: "authorization error"}
			if isVoceChatCredentialRejected(err) || isVoceChatSiteTransientFailure(err) {
				t.Fatal("admin authorization failure must not invalidate user credentials or permit outage fallback")
			}
			outcome := voceChatProvisioningFailure(err)
			if outcome.Code != "admin_credential_invalid" || outcome.UserStatus == models.VoceChatSyncStatusCredentialInvalid {
				t.Fatalf("wrong admin failure outcome: %s/%s", outcome.Code, outcome.UserStatus)
			}
		}
	}
	if !isVoceChatCredentialRejected(&vocechat.APIError{StatusCode: http.StatusUnauthorized, Path: "/token/login"}) {
		t.Fatal("ordinary user login rejection must remain a credential failure")
	}
}

func TestVoceChatAdminEmailCanBeExplicitlyClearedForTokenOnly(t *testing.T) {
	config := models.SiteConfig{VoceChatBaseURL: "https://vc.example.test", VoceChatAdminUsername: "admin@vc.com", VoceChatAdminPassword: "remote-admin", VoceChatAdminToken: "valid-token", VoceChatLastHealthStatus: "ok"}
	if err := applyVoceChatConfigUpdate(&config, map[string]interface{}{"contactsCacheTTLSeconds": 60}); err != nil {
		t.Fatal(err)
	}
	if config.VoceChatAdminUsername == "" || config.VoceChatAdminPassword == "" {
		t.Fatal("omitted credentials must remain stored")
	}
	if err := applyVoceChatConfigUpdate(&config, map[string]interface{}{"clearAdminUsername": true, "clearAdminPassword": true}); err != nil {
		t.Fatal(err)
	}
	if config.VoceChatAdminUsername != "" || config.VoceChatAdminPassword != "" || config.VoceChatAdminToken != "valid-token" || config.VoceChatLastHealthStatus != "" {
		t.Fatal("explicit clear must preserve the token and require a new health check")
	}
	if !vocechat.FromSiteConfig(config).HasAdminCredential() {
		t.Fatal("token-only configuration must remain supported")
	}
	config.VoceChatAdminUsername = "admin@vc.com"
	if vocechat.FromSiteConfig(config).HasAdminCredential() {
		t.Fatal("an incomplete password credential must not be marked configured because a token is stored")
	}
}

func TestVoceChatExpiredTokenOnlyFailsHealthAndRegistrationSafely(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
	mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/admin/user" {
			t.Errorf("unexpected request with expired admin token: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("authorization error"))
	}))
	defer server.Close()
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", 1).Updates(map[string]interface{}{"voce_chat_base_url": server.URL, "voce_chat_admin_token": "expired-token"}).Error; err != nil {
		t.Fatal(err)
	}
	result := provisionRegistrationUserWithVoceChat("1", "new_user", "test-password")
	if result.UserID != "" || !strings.Contains(result.SyncError, "管理接口鉴权失败") {
		t.Fatal("registration must report administrator authorization failure")
	}
	health, err := CheckVoceChatHealth(context.Background())
	if err == nil || health["lastHealthStatus"] != "failed" {
		t.Fatal("expired token-only configuration must fail the same management health check")
	}
	registration, err := RegisterWithResult(dto.RegisterDto{Username: "new_user", Password: "test-password"})
	if err != nil || registration.Status != models.RegistrationApplicationStatusPending || registration.AutoApproved {
		t.Fatalf("registration must remain pending after failed management health: %v", err)
	}
}
