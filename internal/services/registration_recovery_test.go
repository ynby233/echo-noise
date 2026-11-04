package services

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

func TestRegistrationApprovalRetriesRecordedUIDAfterVerificationFailure(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true})
	provisionCalls := 0
	setRegistrationProvisionForTest(t, func(id, username, password string) registrationVoceChatProvisionResult {
		provisionCalls++
		if provisionCalls > 1 {
			t.Fatal("approval must reuse the recorded remote account")
		}
		return registrationVoceChatProvisionResult{Email: id + "@vc.com", UserID: "42", SyncStatus: models.VoceChatSyncStatusCreated}
	})
	verifyCalls := 0
	setRegistrationVerifyForTest(t, func(application models.RegistrationApplication) (bool, error) {
		verifyCalls++
		if application.VoceChatUserID != "42" {
			t.Fatal("recorded UID was lost")
		}
		if verifyCalls == 1 {
			return false, errors.New("temporary verification failure")
		}
		return true, nil
	})
	if err := Register(dto.RegisterDto{Username: "retry_user", Password: "test-password"}); err != nil {
		t.Fatal(err)
	}
	application, err := repository.GetPendingRegistrationApplicationByUsername("retry_user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveRegistrationApplication(application.ID, admin.ID, ""); !isRegistrationApprovalDeferred(err) {
		t.Fatalf("expected deferred approval, got %v", err)
	}
	created, err := ApproveRegistrationApplication(application.ID, admin.ID, "retry")
	if err != nil {
		t.Fatal(err)
	}
	if verifyCalls != 2 || provisionCalls != 1 || created.VoceChatUserID != "42" || created.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
		t.Fatal("retry did not restore the existing binding")
	}
	approved, err := repository.GetRegistrationApplicationByID(application.ID)
	if err != nil || approved.Status != models.RegistrationApplicationStatusApproved || approved.VoceChatSyncError != "" {
		t.Fatal("approval did not clear the failed verification state")
	}
}

func TestRegistrationApprovalRecoversExistingAccountOnlyWithMatchingPassword(t *testing.T) {
	for _, validPassword := range []bool{true, false} {
		name := "matching_password"
		if !validPassword {
			name = "wrong_password"
		}
		t.Run(name, func(t *testing.T) {
			db := setupUserServiceTestDB(t)
			createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
			t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
			admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true})
			application := models.RegistrationApplication{Username: "recover_user", PasswordHash: models.HashPassword("test-password"), Status: models.RegistrationApplicationStatusPending, VoceChatSyncStatus: models.VoceChatSyncStatusConflicted}
			if err := repository.CreateRegistrationApplicationWithPermanentNumber(&application, "vc.com"); err != nil {
				t.Fatal(err)
			}
			if err := vocechat.DefaultPlainPasswordStore().UpsertApplicationVoceChatPassword(application.ApplicationID, application.Username, "test-password", application.VoceChatCandidateEmail, ""); err != nil {
				t.Fatal(err)
			}
			loginCalls := 0
			createCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/admin/user":
					_ = json.NewEncoder(w).Encode([]vocechat.User{{UID: 42, Email: application.VoceChatCandidateEmail, Name: application.Username}})
				case r.Method == http.MethodPost && r.URL.Path == "/api/token/login":
					loginCalls++
					var body struct {
						Credential struct {
							Email    string `json:"email"`
							Password string `json:"password"`
						} `json:"credential"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Credential.Email != application.VoceChatCandidateEmail || body.Credential.Password != "test-password" {
						t.Error("recovery did not verify the application credentials")
					}
					if !validPassword {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					_ = json.NewEncoder(w).Encode(vocechat.LoginResponse{Token: "test-token", User: vocechat.UserInfo{UID: 42, Email: application.VoceChatCandidateEmail, Name: application.Username}})
				case r.Method == http.MethodPost && r.URL.Path == "/api/admin/user":
					createCalls++
					w.WriteHeader(http.StatusConflict)
				default:
					t.Errorf("unexpected VC request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			if err := db.Model(&models.SiteConfig{}).Where("id = ?", 1).Updates(map[string]interface{}{"voce_chat_base_url": server.URL, "voce_chat_admin_token": "test-admin-token"}).Error; err != nil {
				t.Fatal(err)
			}
			created, err := ApproveRegistrationApplication(application.ID, admin.ID, "recover")
			if loginCalls != 1 || createCalls != 0 {
				t.Fatalf("login calls = %d, create calls = %d", loginCalls, createCalls)
			}
			if validPassword {
				if err != nil {
					t.Fatal(err)
				}
				if created.VoceChatUserID != "42" || created.VoceChatEmail != application.VoceChatCandidateEmail || created.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
					t.Fatal("existing VC binding was not recovered")
				}
			} else {
				if !isRegistrationApprovalDeferred(err) || created != nil {
					t.Fatal("wrong password must not approve an existing VC account")
				}
				pending, loadErr := repository.GetRegistrationApplicationByID(application.ID)
				if loadErr != nil || pending.Status != models.RegistrationApplicationStatusPending || pending.LocalUserID != nil {
					t.Fatal("rejected credentials must leave the application pending")
				}
				var count int64
				if err := db.Model(&models.User{}).Where("username = ?", application.Username).Count(&count).Error; err != nil || count != 0 {
					t.Fatal("wrong credentials created a local user")
				}
			}
		})
	}
}

func TestRegistrationVerificationRequiresMatchingUIDAndEmail(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	remote := vocechat.User{UID: 42, Email: "42@vc.com"}
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/admin/user/42" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(remote)
	}))
	defer server.Close()
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", 1).Updates(map[string]interface{}{"voce_chat_base_url": server.URL, "voce_chat_admin_token": "test-admin-token"}).Error; err != nil {
		t.Fatal(err)
	}
	application := models.RegistrationApplication{VoceChatUserID: "42", VoceChatCandidateEmail: "42@vc.com"}
	for _, tc := range []struct {
		name   string
		uid    int64
		email  string
		status int
		want   bool
	}{
		{"matching", 42, "42@VC.COM", http.StatusOK, true},
		{"wrong_uid", 43, "42@vc.com", http.StatusOK, false},
		{"wrong_email", 42, "someone@vc.com", http.StatusOK, false},
		{"missing_email", 42, "", http.StatusOK, false},
		{"deleted", 42, "42@vc.com", http.StatusNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote.UID, remote.Email, status = tc.uid, tc.email, tc.status
			exists, err := verifyRegistrationUserWithVoceChat(application)
			wantError := tc.name == "wrong_uid" || tc.name == "wrong_email" || tc.name == "missing_email"
			if (err != nil) != wantError || exists != tc.want {
				t.Fatalf("verification = %v, %v; want exists=%v error=%v", exists, err, tc.want, wantError)
			}
		})
	}
}
