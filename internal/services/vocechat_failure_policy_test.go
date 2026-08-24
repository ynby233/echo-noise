package services

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
)

func voceChatAPIError(status int, body string) error {
	return &vocechat.APIError{StatusCode: status, Method: http.MethodPost, Path: "/api/token/login", Body: body}
}

func samePlainPasswordRecordState(left, right vocechat.PlainPasswordRecord) bool {
	return left.Key == right.Key &&
		left.Kind == right.Kind &&
		left.UserID == right.UserID &&
		left.ApplicationID == right.ApplicationID &&
		left.Username == right.Username &&
		left.Password == right.Password &&
		left.VoceChatPassword == right.VoceChatPassword &&
		sameOptionalTime(left.VoceChatPasswordUpdatedAt, right.VoceChatPasswordUpdatedAt) &&
		left.LocalFallbackPassword == right.LocalFallbackPassword &&
		sameOptionalTime(left.LocalFallbackPasswordUpdatedAt, right.LocalFallbackPasswordUpdatedAt) &&
		left.VoceChatEmail == right.VoceChatEmail &&
		left.VoceChatUserID == right.VoceChatUserID &&
		left.UpdatedAt.Equal(right.UpdatedAt)
}

func TestVoceChatLoginFailurePolicyOnlyFallsBackForTransientSiteFailures(t *testing.T) {
	tests := []struct {
		name              string
		failure           error
		wantFallback      bool
		wantHealthFailure bool
	}{
		{name: "credential 400", failure: voceChatAPIError(http.StatusBadRequest, `{"error":"invalid password"}`)},
		{name: "credential 401", failure: voceChatAPIError(http.StatusUnauthorized, `{"error":"invalid password"}`)},
		{name: "credential 403", failure: voceChatAPIError(http.StatusForbidden, `{"error":"invalid credentials"}`)},
		{name: "semantic account 404", failure: voceChatAPIError(http.StatusNotFound, `{"error":"user email not found"}`)},
		{name: "generic 404", failure: voceChatAPIError(http.StatusNotFound, `{"error":"resource not found"}`)},
		{name: "other nontransient 4xx", failure: voceChatAPIError(http.StatusTeapot, `{"error":"unsupported request"}`)},
		{name: "unclassified go error", failure: errors.New("unexpected response shape")},
		{name: "deadline", failure: context.DeadlineExceeded, wantFallback: true, wantHealthFailure: true},
		{name: "url transport", failure: &url.Error{Op: "Post", URL: "https://vc.example.test/api/token/login", Err: syscall.ECONNREFUSED}, wantFallback: true, wantHealthFailure: true},
		{name: "network", failure: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, wantFallback: true, wantHealthFailure: true},
		{name: "server 500", failure: voceChatAPIError(http.StatusInternalServerError, `{"error":"internal"}`), wantFallback: true, wantHealthFailure: true},
		{name: "server 502", failure: voceChatAPIError(http.StatusBadGateway, `{"error":"gateway"}`), wantFallback: true, wantHealthFailure: true},
		{name: "server 503", failure: voceChatAPIError(http.StatusServiceUnavailable, `{"error":"unavailable"}`), wantFallback: true, wantHealthFailure: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupUserServiceTestDB(t)
			createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
			storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
			t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
			mustCreateUser(t, models.User{Username: "primary-failure-policy", Password: models.HashPassword("primary"), IsAdmin: true})
			user := mustCreateUser(t, models.User{
				Username:           "failure-policy-user",
				Password:           models.HashPassword("local-password"),
				Token:              "unchanged-token",
				VoceChatEmail:      "failure-policy@vc.example",
				VoceChatUserID:     "710",
				VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
			})
			store := vocechat.NewPlainPasswordStore(storePath)
			if err := store.UpsertUserVoceChatPassword(user.ID, user.Username, "remote-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
				t.Fatalf("seed remote password: %v", err)
			}
			if err := store.UpsertUserLocalFallbackPassword(user.ID, user.Username, "local-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
				t.Fatalf("seed local fallback password: %v", err)
			}
			beforeRecord, beforeRecordSeen, err := store.GetUserPassword(user.ID)
			if err != nil || !beforeRecordSeen {
				t.Fatalf("read password snapshot: seen=%v err=%v", beforeRecordSeen, err)
			}

			loginCalls := 0
			stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
				loginCalls++
				return nil, tc.failure
			})

			loggedIn, loginErr := Login(dto.LoginDto{Username: user.Username, Password: "local-password"})
			if tc.wantFallback {
				if loginErr != nil || loggedIn == nil {
					t.Fatalf("transient failure must allow local fallback: user=%#v err=%v", loggedIn, loginErr)
				}
			} else {
				if loginErr == nil || loggedIn != nil {
					t.Fatalf("nontransient failure must fail closed: user=%#v err=%v", loggedIn, loginErr)
				}
			}
			if loginCalls != 1 {
				t.Fatalf("VoceChat login calls=%d, want 1", loginCalls)
			}

			var config models.SiteConfig
			if err := database.DB.First(&config).Error; err != nil {
				t.Fatalf("reload site config: %v", err)
			}
			if tc.wantHealthFailure {
				if config.VoceChatLastHealthStatus != "failed" {
					t.Fatalf("transient failure health=%q, want failed", config.VoceChatLastHealthStatus)
				}
			} else if config.VoceChatLastHealthStatus != "ok" || config.VoceChatLastHealthError != "" {
				t.Fatalf("nontransient failure changed site health to %q/%q", config.VoceChatLastHealthStatus, config.VoceChatLastHealthError)
			}

			if !tc.wantFallback {
				updated := mustGetUserByUsername(t, user.Username)
				if updated.Token != "unchanged-token" || updated.VoceChatSyncStatus != models.VoceChatSyncStatusLinked || updated.VoceChatSyncError != "" || bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("local-password")) != nil {
					t.Fatalf("nontransient failure changed user state: %#v", updated)
				}
				afterRecord, afterRecordSeen, err := store.GetUserPassword(user.ID)
				if err != nil || afterRecordSeen != beforeRecordSeen || !samePlainPasswordRecordState(afterRecord, beforeRecord) {
					t.Fatalf("nontransient failure changed password record: before=%#v after=%#v seen=%v err=%v", beforeRecord, afterRecord, afterRecordSeen, err)
				}
			}
		})
	}
}

func TestUnexpectedProvisioningFailureDoesNotDegradeVoceChatSiteHealth(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	primary := mustCreateUser(t, models.User{Username: "primary-provision-failure", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{Username: "unexpected-provision-failure", Password: models.HashPassword("local-password"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(user.ID, user.Username, "local-password", "", ""); err != nil {
		t.Fatalf("seed local password: %v", err)
	}
	stubVoceChatProvisioningCreate(t, func(context.Context, vocechat.Config, string, string, string) (voceChatProvisioningCreateResult, error) {
		return voceChatProvisioningCreateResult{}, errors.New("unexpected response shape")
	})

	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("start provisioning: %v", err)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run provisioning: %v", err)
	}
	var task models.VoceChatProvisioningTask
	if err := db.Where("user_id = ?", user.ID).First(&task).Error; err != nil {
		t.Fatalf("load failed task: %v", err)
	}
	if task.Status != models.VoceChatSyncStatusFailed {
		t.Fatalf("unexpected task status=%q", task.Status)
	}
	var config models.SiteConfig
	if err := db.First(&config).Error; err != nil {
		t.Fatalf("reload site config: %v", err)
	}
	if config.VoceChatLastHealthStatus != "ok" || config.VoceChatLastHealthError != "" {
		t.Fatalf("unexpected provisioning error degraded site health to %q/%q", config.VoceChatLastHealthStatus, config.VoceChatLastHealthError)
	}
}
