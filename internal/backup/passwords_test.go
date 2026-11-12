package backup

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Use explicit fixture paths; never resolve or open the default password store.
func passwordPairFixture(t *testing.T, seed func(*gorm.DB, *gorm.DB)) (string, string) {
	t.Helper()
	root := t.TempDir()
	databasePath := filepath.Join(root, "database.db")
	passwordPath := filepath.Join(root, "passwords.db")
	open := func(path string) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}
	db := open(databasePath)
	passwordDB := open(passwordPath)
	if err := db.AutoMigrate(&models.User{}, &models.RegistrationApplication{}); err != nil {
		t.Fatal(err)
	}
	if err := passwordDB.AutoMigrate(&vocechat.PlainPasswordRecord{}); err != nil {
		t.Fatal(err)
	}
	seed(db, passwordDB)
	for _, connection := range []*gorm.DB{db, passwordDB} {
		sqlDB, err := connection.DB()
		if err != nil {
			t.Fatal(err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return databasePath, passwordPath
}

func passwordPairHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}

func TestValidatePasswordPairPendingCandidates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		legacy   string
		local    string
		remote   string
		username string
		missing  bool
		wantErr  bool
	}{
		{name: "separate local field", local: "application-secret"},
		{name: "separate VoceChat field", remote: "application-secret"},
		{name: "legacy fallback", legacy: "application-secret"},
		{name: "local candidate matches", local: "application-secret", remote: "different-remote"},
		{name: "VoceChat candidate matches", local: "different-local", remote: "application-secret"},
		{name: "legacy candidate matches with different local", local: "different-local", legacy: "application-secret"},
		{name: "both candidates mismatch", local: "different-local", remote: "different-remote", wantErr: true},
		{name: "explicit remote overrides legacy", legacy: "application-secret", remote: "different-remote", wantErr: true},
		{name: "empty candidates", wantErr: true},
		{name: "missing record", missing: true, wantErr: true},
		{name: "different username", local: "application-secret", username: "another-applicant", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			databasePath, passwordPath := passwordPairFixture(t, func(db, passwordDB *gorm.DB) {
				application := models.RegistrationApplication{
					ApplicationID: "pending-application",
					Username:      "applicant",
					PasswordHash:  passwordPairHash(t, "application-secret"),
					Status:        models.RegistrationApplicationStatusPending,
				}
				if err := db.Create(&application).Error; err != nil {
					t.Fatal(err)
				}
				if tc.missing {
					return
				}
				username := tc.username
				if username == "" {
					username = application.Username
				}
				record := vocechat.PlainPasswordRecord{
					Key:                   "application:pending-application",
					Kind:                  vocechat.PlainPasswordKindApplication,
					ApplicationID:         application.ApplicationID,
					Username:              username,
					Password:              tc.legacy,
					LocalFallbackPassword: tc.local,
					VoceChatPassword:      tc.remote,
				}
				if err := passwordDB.Create(&record).Error; err != nil {
					t.Fatal(err)
				}
			})
			err := validatePasswordPair(databasePath, passwordPath)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validatePasswordPair() = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "pending-application") {
				t.Fatalf("expected application mismatch, got %v", err)
			}
		})
	}
}

func TestValidatePasswordPairUserRecords(t *testing.T) {
	for _, tc := range []struct {
		name        string
		deleted     bool
		username    string
		local       string
		remote      string
		legacy      string
		wantMessage string
	}{
		{name: "separate local and remote passwords", username: "original-user", local: "local-secret", remote: "remote-secret"},
		{name: "remote password does not require local match", username: "original-user", remote: "remote-secret"},
		{name: "legacy remote password does not require local match", username: "original-user", legacy: "legacy-remote-secret"},
		{name: "local mismatch rejected", username: "original-user", local: "different-local", remote: "local-secret", wantMessage: "本地密码不一致"},
		{name: "deleted user ignored", deleted: true, local: "deleted-secret"},
		{name: "reused ID with different username rejected", username: "replacement-user", local: "local-secret", wantMessage: "账户名称"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			databasePath, passwordPath := passwordPairFixture(t, func(db, passwordDB *gorm.DB) {
				user := models.User{ID: 42, Username: "original-user", Password: passwordPairHash(t, "local-secret")}
				if err := db.Create(&user).Error; err != nil {
					t.Fatal(err)
				}
				if tc.deleted {
					if err := db.Delete(&user).Error; err != nil {
						t.Fatal(err)
					}
				} else if tc.username != user.Username {
					if err := db.Delete(&user).Error; err != nil {
						t.Fatal(err)
					}
					replacement := models.User{ID: user.ID, Username: tc.username, Password: user.Password}
					if err := db.Create(&replacement).Error; err != nil {
						t.Fatal(err)
					}
				}
				record := vocechat.PlainPasswordRecord{
					Key:                   "user:42",
					Kind:                  vocechat.PlainPasswordKindUser,
					UserID:                user.ID,
					Username:              "original-user",
					LocalFallbackPassword: tc.local,
					VoceChatPassword:      tc.remote,
					Password:              tc.legacy,
				}
				if err := passwordDB.Create(&record).Error; err != nil {
					t.Fatal(err)
				}
			})
			err := validatePasswordPair(databasePath, passwordPath)
			if tc.wantMessage == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantMessage) {
				t.Fatalf("validatePasswordPair() = %v, want error containing %q", err, tc.wantMessage)
			}
		})
	}
}

func TestValidatePasswordPairReturnsUserQueryError(t *testing.T) {
	databasePath, passwordPath := passwordPairFixture(t, func(db, passwordDB *gorm.DB) {
		// Keep the table present, but make the user query fail rather than return no rows.
		if err := db.Exec("DROP TABLE users").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("CREATE TABLE users (unexpected_column INTEGER)").Error; err != nil {
			t.Fatal(err)
		}
		record := vocechat.PlainPasswordRecord{Key: "user:42", Kind: vocechat.PlainPasswordKindUser, UserID: 42, Username: "original-user"}
		if err := passwordDB.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	})
	err := validatePasswordPair(databasePath, passwordPath)
	if err == nil || !strings.Contains(err.Error(), "查询密码库对应账户失败") {
		t.Fatalf("expected user query error, got %v", err)
	}
}
