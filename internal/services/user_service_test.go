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

func TestGetStatusIncludesUserAvatarURLs(t *testing.T) {
	setupUserServiceTestDB(t)

	mustCreateUser(t, models.User{
		Username:  "admin",
		Password:  models.HashPassword("admin"),
		IsAdmin:   true,
		Token:     models.GenerateToken(32),
		AvatarURL: "https://example.com/admin-avatar.png",
	})
	mustCreateUser(t, models.User{
		Username:  "alice",
		Password:  models.HashPassword("alice"),
		Token:     models.GenerateToken(32),
		AvatarURL: "/api/images/alice.png",
	})

	status, err := GetStatus(0)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}

	avatars := map[string]string{}
	for _, user := range status.Users {
		avatars[user.Username] = user.AvatarURL
	}
	if avatars["admin"] != "https://example.com/admin-avatar.png" {
		t.Fatalf("admin avatar missing from status, got %q", avatars["admin"])
	}
	if avatars["alice"] != "/api/images/alice.png" {
		t.Fatalf("alice avatar missing from status, got %q", avatars["alice"])
	}
}

func TestGetStatusUsesViewerScopedDashboardCounts(t *testing.T) {
	setupUserServiceTestDB(t)

	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32)})

	adminMessage := models.Message{Content: "admin message", UserID: admin.ID, Username: admin.Username}
	aliceMessage := models.Message{Content: "alice message", UserID: alice.ID, Username: alice.Username}
	bobMessage := models.Message{Content: "bob message", UserID: bob.ID, Username: bob.Username}
	if err := database.DB.Create(&adminMessage).Error; err != nil {
		t.Fatalf("create admin message: %v", err)
	}
	if err := database.DB.Create(&aliceMessage).Error; err != nil {
		t.Fatalf("create alice message: %v", err)
	}
	if err := database.DB.Create(&bobMessage).Error; err != nil {
		t.Fatalf("create bob message: %v", err)
	}

	bobCommentOnAlice := models.Comment{MessageID: aliceMessage.ID, UserID: &bob.ID, Content: "bob comment", Visibility: "public"}
	aliceCommentOnAlice := models.Comment{MessageID: aliceMessage.ID, UserID: &alice.ID, Content: "self comment", Visibility: "public"}
	aliceCommentOnBob := models.Comment{MessageID: bobMessage.ID, UserID: &alice.ID, Content: "alice comment", Visibility: "public"}
	bobCommentOnAdmin := models.Comment{MessageID: adminMessage.ID, UserID: &bob.ID, Content: "bob admin comment", Visibility: "public"}
	for _, comment := range []*models.Comment{&bobCommentOnAlice, &aliceCommentOnAlice, &aliceCommentOnBob, &bobCommentOnAdmin} {
		if err := database.DB.Create(comment).Error; err != nil {
			t.Fatalf("create comment: %v", err)
		}
	}
	parentID := aliceCommentOnBob.ID
	bobReplyToAlice := models.Comment{MessageID: bobMessage.ID, UserID: &bob.ID, ParentID: &parentID, Content: "bob reply", Visibility: "public"}
	aliceReplyToSelf := models.Comment{MessageID: bobMessage.ID, UserID: &alice.ID, ParentID: &parentID, Content: "self reply", Visibility: "public"}
	for _, reply := range []*models.Comment{&bobReplyToAlice, &aliceReplyToSelf} {
		if err := database.DB.Create(reply).Error; err != nil {
			t.Fatalf("create reply: %v", err)
		}
	}

	aliceStatus, err := GetStatus(alice.ID)
	if err != nil {
		t.Fatalf("get alice status: %v", err)
	}
	if aliceStatus.TotalMessages != 1 {
		t.Fatalf("alice total messages = %d, want 1", aliceStatus.TotalMessages)
	}
	if aliceStatus.ReceivedComments != 1 {
		t.Fatalf("alice received comments = %d, want 1", aliceStatus.ReceivedComments)
	}
	if aliceStatus.ReceivedReplies != 1 {
		t.Fatalf("alice received replies = %d, want 1", aliceStatus.ReceivedReplies)
	}

	adminStatus, err := GetStatus(admin.ID)
	if err != nil {
		t.Fatalf("get admin status: %v", err)
	}
	if adminStatus.TotalMessages != 3 {
		t.Fatalf("admin total messages = %d, want 3", adminStatus.TotalMessages)
	}
	if adminStatus.ReceivedComments != 1 {
		t.Fatalf("admin received comments = %d, want 1", adminStatus.ReceivedComments)
	}
	if adminStatus.TotalUsers != 3 {
		t.Fatalf("total users = %d, want 3", adminStatus.TotalUsers)
	}
	if adminStatus.TotalComments != 4 {
		t.Fatalf("total comments = %d, want 4", adminStatus.TotalComments)
	}
	if adminStatus.TotalReplies != 2 {
		t.Fatalf("total replies = %d, want 2", adminStatus.TotalReplies)
	}
}
