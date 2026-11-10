package services

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"gorm.io/gorm"
)

func setupAICredentialServiceTest(t *testing.T) (*gorm.DB, models.User, models.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	oldDB := database.DB
	oldModelDB := models.GetDB()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = oldDB
		models.SetDB(oldModelDB)
		_ = sqlDB.Close()
	})
	if err := db.AutoMigrate(&models.User{}, &models.AICredential{}, &models.SiteConfig{}); err != nil {
		t.Fatal(err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	if err := db.Create(&models.SiteConfig{
		LoginExpireHours: 1, DelegatedAdminLoginExpireHours: 2,
		RuntimeMode: models.RuntimeModeLocal, RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.SiteConfig{}).Where("id > 0").Updates(map[string]any{
		"login_expire_days": 0, "delegated_admin_login_expire_days": 0,
	}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Minute).UTC()
	owner := models.User{ID: 21, Username: "ai-owner", Password: models.HashPassword("owner-password"), Token: "OrdinaryOwnerToken", LoginIssuedAt: &now}
	other := models.User{ID: 22, Username: "ai-other", Password: models.HashPassword("other-password"), Token: "OrdinaryOtherToken", LoginIssuedAt: &now}
	for _, user := range []*models.User{&owner, &other} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db, owner, other
}

func TestAICredentialMigration(t *testing.T) {
	db := setupUserServiceTestDB(t)
	if !db.Migrator().HasTable(&models.AICredential{}) || !db.Migrator().HasIndex(&models.AICredential{}, "TokenHash") {
		t.Fatal("normal migration must create AI credentials and their unique hash index")
	}
}

func TestAICredentialSecretIsRandomHashedAndShownOnlyAtCreation(t *testing.T) {
	db, owner, other := setupAICredentialServiceTest(t)
	first, token, err := CreateAICredential(owner.ID, "  编辑助手\t")
	if err != nil {
		t.Fatal(err)
	}
	second, secondToken, err := CreateAICredential(owner.ID, "编辑助手")
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "编辑助手" || first.ID == second.ID || token == secondToken || !strings.HasPrefix(token, AICredentialTokenPrefix) {
		t.Fatal("creation must trim names and issue independent credentials")
	}
	random, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, AICredentialTokenPrefix))
	if err != nil || len(random) != 32 {
		t.Fatal("expected a 32-byte random token payload")
	}
	var stored models.AICredential
	if err := db.First(&stored, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(token))
	if stored.TokenHash != hex.EncodeToString(digest[:]) || stored.TokenHash == token {
		t.Fatal("database must store only the SHA-256 digest")
	}
	if db.Migrator().HasColumn(&models.AICredential{}, "token") {
		t.Fatal("credential schema must not have a plaintext token column")
	}
	duplicate := stored
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("token hashes must be unique")
	}
	listed, err := ListAICredentials(owner.ID)
	if err != nil || len(listed) != 2 {
		t.Fatalf("owner list: len=%d err=%v", len(listed), err)
	}
	encoded, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), token) || strings.Contains(string(encoded), stored.TokenHash) || strings.Contains(string(encoded), "user_id") || strings.Contains(string(encoded), "token") {
		t.Fatal("listing must reveal only public credential metadata")
	}
	foreign, err := ListAICredentials(other.ID)
	if err != nil || len(foreign) != 0 {
		t.Fatal("another account must not list owner credentials")
	}
	if err := RevokeAICredential(other.ID, first.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("foreign revoke: %v", err)
	}
	if _, _, _, err := ResolveBearerIdentity("Bearer "+token, false); err != nil {
		t.Fatal("foreign revocation must not affect owner credential")
	}
	if err := RevokeAICredential(owner.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	for _, allowExpired := range []bool{false, true} {
		if _, _, _, err := ResolveBearerIdentity("Bearer "+token, allowExpired); err == nil {
			t.Fatal("revoked credentials must fail even with allowExpired")
		}
	}
	listed, err = ListAICredentials(owner.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != second.ID {
		t.Fatal("list must contain only remaining valid credentials")
	}
}

func TestAICredentialNameLimitsCountUnicodeCharacters(t *testing.T) {
	_, owner, _ := setupAICredentialServiceTest(t)
	for _, name := range []string{"", " \t\n", strings.Repeat("界", 101), string([]byte{0xff})} {
		if _, _, err := CreateAICredential(owner.ID, name); !errors.Is(err, ErrInvalidAICredentialName) {
			t.Fatalf("invalid name accepted: %v", err)
		}
	}
	for _, name := range []string{"界", strings.Repeat("界", 100)} {
		if _, _, err := CreateAICredential(owner.ID, " \t"+name+"\n"); err != nil {
			t.Fatalf("valid Unicode name rejected: %v", err)
		}
	}
}

func TestResolveBearerIdentityExpiryAndCurrentDatabaseState(t *testing.T) {
	db, owner, _ := setupAICredentialServiceTest(t)
	credential, token, err := CreateAICredential(owner.ID, "expiry")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetUserByID(owner.ID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-90 * time.Minute)
	if err := db.Model(&owner).Updates(map[string]any{"login_issued_at": old, "username": "renamed", "is_admin": false}).Error; err != nil {
		t.Fatal(err)
	}
	for _, allowExpired := range []bool{false, true} {
		if _, _, _, err := ResolveBearerIdentity("Bearer "+owner.Token, allowExpired); !errors.Is(err, ErrBearerLoginExpired) {
			t.Fatalf("ordinary token expiry must never be bypassed: %v", err)
		}
	}
	if _, _, _, err := ResolveBearerIdentity("Bearer "+token, false); !errors.Is(err, ErrBearerLoginExpired) {
		t.Fatalf("AI expiry: %v", err)
	}
	user, got, via, err := ResolveBearerIdentity("bEaReR\t"+token, true)
	if err != nil || user.Username != "renamed" || got.ID != credential.ID || via != "ai_token" {
		t.Fatalf("allow expired must still return current identity: %v", err)
	}
	// A role change changes the applicable current database policy immediately.
	if err := db.Model(&owner).Update("is_admin", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, bearer := range []string{token, owner.Token} {
		user, _, _, err := ResolveBearerIdentity("Bearer "+bearer, false)
		if err != nil || !user.IsAdmin {
			t.Fatalf("current administrator identity/policy not applied: %v", err)
		}
	}
	if err := db.Model(&models.SiteConfig{}).Where("id > 0").Update("delegated_admin_login_expire_hours", 1).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ResolveBearerIdentity("Bearer "+token, false); !errors.Is(err, ErrBearerLoginExpired) {
		t.Fatal("policy changes must apply immediately")
	}
	if err := db.Model(&owner).Update("login_issued_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ResolveBearerIdentity("Bearer "+token, false); err != nil {
		t.Fatal("a refreshed database login timestamp must restore access")
	}
	if err := db.Delete(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ResolveBearerIdentity("Bearer "+token, true); err == nil {
		t.Fatal("deleted users must not authenticate via cached identity")
	}
}

func TestResolveBearerIdentityStrictSchemeAndNoAIPrefixFallback(t *testing.T) {
	db, owner, _ := setupAICredentialServiceTest(t)
	credential, token, err := CreateAICredential(owner.ID, "strict")
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"", owner.Token, "Bearer", "Basic " + owner.Token, "Bearer " + owner.Token + " extra", "Bearer noise_ai_invalid", "Bearer " + strings.ToLower(owner.Token)} {
		if _, _, _, err := ResolveBearerIdentity(header, true); err == nil {
			t.Fatalf("invalid authorization accepted: %q", header)
		}
	}
	user, got, via, err := ResolveBearerIdentity("Bearer "+owner.Token, false)
	if err != nil || user.ID != owner.ID || got != nil || via != "token" {
		t.Fatalf("ordinary bearer contract failed: %v", err)
	}
	if err := db.Model(&owner).Update("token", token).Error; err != nil {
		t.Fatal(err)
	}
	if err := RevokeAICredential(owner.ID, credential.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ResolveBearerIdentity("Bearer "+token, true); err == nil {
		t.Fatal("an AI-prefixed token must never fall back to users.token")
	}
}

func TestLoginWithAICredentialRejectsOtherAccountBeforeLoginSideEffects(t *testing.T) {
	db, owner, other := setupAICredentialServiceTest(t)
	_, token, err := CreateAICredential(owner.ID, "renew")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour).UTC()
	if err := db.Model(&models.User{}).Where("id IN ?", []uint{owner.ID, other.ID}).Update("login_issued_at", old).Error; err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []struct{ header, username, password string }{
		{"Bearer " + token, other.Username, "other-password"},
		{"Bearer " + token, owner.Username, "wrong-password"},
		{"Bearer " + owner.Token, owner.Username, "owner-password"},
		{"", owner.Username, "owner-password"},
	} {
		if _, _, err := LoginWithAICredential(attempt.header, dto.LoginDto{Username: attempt.username, Password: attempt.password}); err == nil {
			t.Fatal("unauthorized renewal accepted")
		}
		for _, id := range []uint{owner.ID, other.ID} {
			var user models.User
			if err := db.First(&user, id).Error; err != nil {
				t.Fatal(err)
			}
			if user.LoginIssuedAt == nil || !user.LoginIssuedAt.Equal(old) {
				t.Fatal("failed renewal must not refresh either account")
			}
		}
	}
	user, _, err := LoginWithAICredential("Bearer "+token, dto.LoginDto{Username: " " + owner.Username + " ", Password: "owner-password"})
	if err != nil || user.ID != owner.ID || user.Token != owner.Token || user.LoginIssuedAt == nil || !user.LoginIssuedAt.After(old) {
		t.Fatalf("owner renewal must preserve ordinary token and refresh login: %v", err)
	}
	var untouched models.User
	if err := db.First(&untouched, other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !untouched.LoginIssuedAt.Equal(old) || untouched.Token != other.Token {
		t.Fatal("successful owner renewal must not affect another account")
	}
}
