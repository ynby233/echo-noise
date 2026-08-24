package services

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
)

func seedPasswordSyncRequiredUser(t *testing.T, username string) (*models.User, *vocechat.PlainPasswordStore) {
	t.Helper()
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	user := mustCreateUser(t, models.User{
		Username:           username,
		Password:           models.HashPassword("new-local-password"),
		Token:              "unchanged-token",
		VoceChatEmail:      username + "@vc.example",
		VoceChatUserID:     "810",
		VoceChatSyncStatus: models.VoceChatSyncStatusPasswordSyncRequired,
	})
	store := vocechat.NewPlainPasswordStore(storePath)
	if err := store.UpsertUserVoceChatPassword(user.ID, user.Username, "old-remote-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed remote password: %v", err)
	}
	if err := store.UpsertUserLocalFallbackPassword(user.ID, user.Username, "new-local-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed current local password: %v", err)
	}
	return user, store
}

func TestPasswordSyncRequiredBlocksNormalVoceChatLoginBeforeAnySideEffect(t *testing.T) {
	for _, password := range []string{"new-local-password", "old-remote-password"} {
		t.Run(password, func(t *testing.T) {
			setupUserServiceTestDB(t)
			createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
			mustCreateUser(t, models.User{Username: "primary-sync-gate", Password: models.HashPassword("primary"), IsAdmin: true})
			user, _ := seedPasswordSyncRequiredUser(t, "sync-gate-user")
			before, err := readManagedPasswordSnapshot(user.ID)
			if err != nil {
				t.Fatalf("read state snapshot: %v", err)
			}
			loginCalls := 0
			stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
				loginCalls++
				return &vocechat.LoginResponse{Token: "must-not-be-used", User: vocechat.UserInfo{UID: 810, Email: user.VoceChatEmail, Name: user.Username}}, nil
			})

			loggedIn, loginErr := Login(dto.LoginDto{Username: user.Username, Password: password})
			if loggedIn != nil || loginErr == nil || !strings.Contains(loginErr.Error(), "等待 VoceChat 同步") {
				t.Fatalf("pending-sync login result user=%#v err=%v", loggedIn, loginErr)
			}
			if loginCalls != 0 {
				t.Fatalf("pending-sync login called VoceChat %d times", loginCalls)
			}
			if err := managedPasswordSnapshotMatches(user.ID, before); err != nil {
				t.Fatalf("pending-sync login changed account state: %v", err)
			}
			var config models.SiteConfig
			if err := database.DB.First(&config).Error; err != nil {
				t.Fatalf("reload site config: %v", err)
			}
			if config.VoceChatLastHealthStatus != "ok" || config.VoceChatLastHealthError != "" || config.VoceChatLastHealthCheckAt != nil {
				t.Fatalf("pending-sync login changed site health: %#v", config)
			}
		})
	}
}

func TestPasswordSyncRequiredStillUsesCurrentLocalPasswordWhileVoceChatIsDegraded(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "failed")
	mustCreateUser(t, models.User{Username: "primary-sync-degraded", Password: models.HashPassword("primary"), IsAdmin: true})
	user, _ := seedPasswordSyncRequiredUser(t, "sync-degraded-user")
	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		return nil, context.DeadlineExceeded
	})

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "new-local-password"})
	if err != nil || loggedIn == nil {
		t.Fatalf("degraded pending-sync local login user=%#v err=%v", loggedIn, err)
	}
	var updated models.User
	if err := database.DB.First(&updated, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusPasswordSyncRequired || bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("new-local-password")) != nil {
		t.Fatalf("degraded login changed pending-sync state: %#v", updated)
	}
}

func TestPasswordSyncRetryPreservesSyncActionAfterTransientFailure(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	mustCreateUser(t, models.User{Username: "primary-sync-retry", Password: models.HashPassword("primary"), IsAdmin: true})
	user, store := seedPasswordSyncRequiredUser(t, "sync-retry-user")
	primary, err := repositoryUserByIDForTest(models.PrimaryAdminUserID)
	if err != nil {
		t.Fatalf("load primary: %v", err)
	}

	remotePassword := "old-remote-password"
	updateCalls := 0
	failFirst := true
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		updateCalls++
		if request.Password == nil || *request.Password != "new-local-password" {
			t.Fatalf("sync request password=%#v", request.Password)
		}
		if failFirst {
			failFirst = false
			return nil, voceChatAPIError(503, `{"error":"temporarily unavailable"}`)
		}
		remotePassword = *request.Password
		return &vocechat.User{UID: 810, Email: user.VoceChatEmail, Name: user.Username}, nil
	})
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		if password != remotePassword {
			return nil, voceChatAPIError(401, `{"error":"invalid password"}`)
		}
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 810, Email: email, Name: user.Username}}, nil
	})

	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("start password sync: %v", err)
	}
	var task models.VoceChatProvisioningTask
	if err := db.Where("user_id = ?", user.ID).First(&task).Error; err != nil {
		t.Fatalf("load queued task: %v", err)
	}
	if task.Action != models.VoceChatProvisioningActionSyncPassword {
		t.Fatalf("initial task action=%q", task.Action)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run first password sync: %v", err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatalf("reload failed task: %v", err)
	}
	var failedUser models.User
	if err := db.First(&failedUser, user.ID).Error; err != nil {
		t.Fatalf("reload failed user: %v", err)
	}
	if task.Status != models.VoceChatSyncStatusFailed || task.Action != models.VoceChatProvisioningActionSyncPassword || failedUser.VoceChatSyncStatus != models.VoceChatSyncStatusPasswordSyncRequired {
		t.Fatalf("failed sync lost pending action: task=%#v user=%#v", task, failedUser)
	}
	if err := db.Model(&models.SiteConfig{}).Where("id > 0").Updates(map[string]interface{}{
		"voce_chat_last_health_status": "ok",
		"voce_chat_last_health_error":  "",
	}).Error; err != nil {
		t.Fatalf("simulate recovered VoceChat health: %v", err)
	}

	if _, err := RetryVoceChatProvisioningFailures(context.Background(), primary.ID); err != nil {
		t.Fatalf("retry password sync: %v", err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatalf("reload retried task: %v", err)
	}
	if task.Action != models.VoceChatProvisioningActionSyncPassword {
		t.Fatalf("retry action=%q, want sync_password", task.Action)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run password sync retry: %v", err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatalf("reload completed task: %v", err)
	}
	var completedUser models.User
	if err := db.First(&completedUser, user.ID).Error; err != nil {
		t.Fatalf("reload completed user: %v", err)
	}
	record, found, err := store.GetUserPassword(user.ID)
	if err != nil || !found {
		t.Fatalf("read completed password record: found=%v err=%v", found, err)
	}
	if updateCalls != 2 || remotePassword != "new-local-password" || task.Status != models.VoceChatSyncStatusLinked || completedUser.VoceChatSyncStatus != models.VoceChatSyncStatusLinked || record.VoceChatPasswordValue() != "new-local-password" || record.LocalFallbackPasswordValue() != "new-local-password" {
		t.Fatalf("retry did not complete three-store sync: calls=%d remote=%q task=%#v user=%#v record=%#v", updateCalls, remotePassword, task, completedUser, record)
	}
	loggedIn, loginErr := Login(dto.LoginDto{Username: user.Username, Password: "new-local-password"})
	if loginErr != nil || loggedIn == nil {
		t.Fatalf("new synchronized password login user=%#v err=%v", loggedIn, loginErr)
	}
	loggedIn, loginErr = Login(dto.LoginDto{Username: user.Username, Password: "old-remote-password"})
	if loginErr == nil || loggedIn != nil {
		t.Fatalf("old remote password remained valid user=%#v err=%v", loggedIn, loginErr)
	}
}

func repositoryUserByIDForTest(userID uint) (*models.User, error) {
	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
