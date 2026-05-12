package services

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/pkg"
	"gorm.io/gorm"
)

func setupUserServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "noise-test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	models.SetDB(db)
	return db
}

func mustCreateUser(t *testing.T, user models.User) *models.User {
	t.Helper()

	if err := repository.CreateUser(&user); err != nil {
		t.Fatalf("create user %s: %v", user.Username, err)
	}
	return &user
}

func mustGetUserByUsername(t *testing.T, username string) *models.User {
	t.Helper()

	user, err := repository.GetUserByUsername(username)
	if err != nil {
		t.Fatalf("get user by username %s: %v", username, err)
	}
	return user
}

func TestUserProfileUpdatesDoNotOverwritePasswordsAndPasswordFormatsRemainCompatible(t *testing.T) {
	setupUserServiceTestDB(t)

	originalHash := models.HashPassword("admin")
	profileUser := mustCreateUser(t, models.User{
		Username: "admin",
		Password: originalHash,
		IsAdmin:  true,
		Token:    models.GenerateToken(32),
	})

	cachedUser, err := repository.GetUserByID(profileUser.ID)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}

	// 模拟旧实现中 GetUserInfo 直接清空缓存对象的副作用。
	cachedUser.Password = ""

	if err := UpdateUser(cachedUser, dto.UserInfoDto{
		Username:    "admin-renamed",
		AvatarURL:   "https://example.com/avatar.png",
		Description: "updated profile",
	}); err != nil {
		t.Fatalf("update user profile: %v", err)
	}

	updatedProfileUser := mustGetUserByUsername(t, "admin-renamed")
	if updatedProfileUser.Password != originalHash {
		t.Fatalf("password hash changed unexpectedly after profile update, got %q want original hash", updatedProfileUser.Password)
	}
	if updatedProfileUser.AvatarURL != "https://example.com/avatar.png" {
		t.Fatalf("avatar not updated, got %q", updatedProfileUser.AvatarURL)
	}
	if updatedProfileUser.Description != "updated profile" {
		t.Fatalf("description not updated, got %q", updatedProfileUser.Description)
	}

	if _, err := Login(dto.LoginDto{Username: "admin-renamed", Password: "admin"}); err != nil {
		t.Fatalf("login with original password after profile update should succeed: %v", err)
	}

	if err := ChangePasswordWithOld(updatedProfileUser, "admin", "new-password"); err != nil {
		t.Fatalf("change password with old password: %v", err)
	}
	if _, err := Login(dto.LoginDto{Username: "admin-renamed", Password: "new-password"}); err != nil {
		t.Fatalf("login with new password should succeed: %v", err)
	}

	mustCreateUser(t, models.User{
		Username: "plain-user",
		Password: "plainpass",
		Token:    models.GenerateToken(32),
	})
	if _, err := Login(dto.LoginDto{Username: "plain-user", Password: "plainpass"}); err != nil {
		t.Fatalf("plain password login should succeed: %v", err)
	}
	plainUserAfter := mustGetUserByUsername(t, "plain-user")
	if !strings.HasPrefix(plainUserAfter.Password, "$2") {
		t.Fatalf("plain password should be upgraded to bcrypt, got %q", plainUserAfter.Password)
	}

	mustCreateUser(t, models.User{
		Username: "md5-user",
		Password: pkg.MD5Encrypt("md5pass"),
		Token:    models.GenerateToken(32),
	})
	if _, err := Login(dto.LoginDto{Username: "md5-user", Password: "md5pass"}); err != nil {
		t.Fatalf("md5 password login should succeed: %v", err)
	}
	md5UserAfter := mustGetUserByUsername(t, "md5-user")
	if !strings.HasPrefix(md5UserAfter.Password, "$2") {
		t.Fatalf("md5 password should be upgraded to bcrypt, got %q", md5UserAfter.Password)
	}

	if err := ChangePasswordWithOld(md5UserAfter, "md5pass", "md5pass-new"); err != nil {
		t.Fatalf("change password on upgraded md5 user should succeed: %v", err)
	}
	if _, err := Login(dto.LoginDto{Username: "md5-user", Password: "md5pass-new"}); err != nil {
		t.Fatalf("login with changed password should succeed: %v", err)
	}
}
