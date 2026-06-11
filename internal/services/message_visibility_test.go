package services

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

func TestApplyMessageVisibilityForSaveNormalizesAndSyncsPrivateFlag(t *testing.T) {
	tests := []struct {
		name        string
		visibility  string
		private     bool
		wantVisible string
		wantPrivate bool
		wantErr     bool
	}{
		{name: "empty public", visibility: "", private: false, wantVisible: MessageVisibilityPublic, wantPrivate: false},
		{name: "legacy private", visibility: "", private: true, wantVisible: MessageVisibilityPrivate, wantPrivate: true},
		{name: "users", visibility: MessageVisibilityUsers, wantVisible: MessageVisibilityUsers, wantPrivate: true},
		{name: "legacy members alias", visibility: "members", wantVisible: MessageVisibilityUsers, wantPrivate: true},
		{name: "contacts", visibility: MessageVisibilityContacts, wantVisible: MessageVisibilityContacts, wantPrivate: true},
		{name: "private", visibility: MessageVisibilityPrivate, wantVisible: MessageVisibilityPrivate, wantPrivate: true},
		{name: "invalid", visibility: "friends", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := models.Message{Content: "visibility", Visibility: tt.visibility, Private: tt.private}
			err := ApplyMessageVisibilityForSave(&message)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for visibility %q", tt.visibility)
				}
				return
			}
			if err != nil {
				t.Fatalf("apply visibility: %v", err)
			}
			if message.Visibility != tt.wantVisible || message.Private != tt.wantPrivate {
				t.Fatalf("stored visibility/private = %q/%v, want %q/%v", message.Visibility, message.Private, tt.wantVisible, tt.wantPrivate)
			}
		})
	}
}

func TestApplyMessageVisibilityScopeMatchesFourStateRules(t *testing.T) {
	db := setupUserServiceTestDB(t)
	admin := mustCreateUser(t, models.User{Username: "visibility-admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "visibility-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "visibility-bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32)})

	messages := []models.Message{
		{Content: "alice public", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic},
		{Content: "alice users", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityUsers},
		{Content: "alice contacts", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityContacts},
		{Content: "alice private", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPrivate},
		{Content: "bob public", Username: bob.Username, UserID: bob.ID, Visibility: MessageVisibilityPublic},
		{Content: "bob users", Username: bob.Username, UserID: bob.ID, Visibility: MessageVisibilityUsers},
		{Content: "bob contacts", Username: bob.Username, UserID: bob.ID, Visibility: MessageVisibilityContacts},
		{Content: "bob private", Username: bob.Username, UserID: bob.ID, Visibility: MessageVisibilityPrivate},
	}
	for i := range messages {
		if err := ApplyMessageVisibilityForSave(&messages[i]); err != nil {
			t.Fatalf("apply visibility to %q: %v", messages[i].Content, err)
		}
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	aliceID := alice.ID
	tests := []struct {
		name    string
		userID  *uint
		isAdmin bool
		want    []string
	}{
		{name: "guest sees public only", want: []string{"alice public", "bob public"}},
		{name: "member sees public users and own restricted", userID: &aliceID, want: []string{"alice contacts", "alice private", "alice public", "alice users", "bob public", "bob users"}},
		{name: "admin sees all", userID: &admin.ID, isAdmin: true, want: []string{"alice contacts", "alice private", "alice public", "alice users", "bob contacts", "bob private", "bob public", "bob users"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rows []models.Message
			query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), tt.userID, tt.isAdmin)
			if err := query.Find(&rows).Error; err != nil {
				t.Fatalf("query messages: %v", err)
			}
			got := make([]string, 0, len(rows))
			for _, row := range rows {
				got = append(got, row.Content)
			}
			sort.Strings(got)
			if !sameStringSlice(got, tt.want) {
				t.Fatalf("visible messages = %#v, want %#v", got, tt.want)
			}
		})
	}
	bobID := bob.ID
	prefilterTests := []struct {
		name     string
		userID   *uint
		isAdmin  bool
		authorID uint
		want     []string
	}{
		{name: "member viewing another author keeps author filter", userID: &aliceID, authorID: bobID, want: []string{"bob public", "bob users"}},
		{name: "member viewing self keeps all own states", userID: &aliceID, authorID: aliceID, want: []string{"alice contacts", "alice private", "alice public", "alice users"}},
		{name: "guest viewing author sees public only", authorID: aliceID, want: []string{"alice public"}},
		{name: "admin viewing author sees all states", userID: &admin.ID, isAdmin: true, authorID: bobID, want: []string{"bob contacts", "bob private", "bob public", "bob users"}},
	}

	for _, tt := range prefilterTests {
		t.Run(tt.name, func(t *testing.T) {
			var rows []models.Message
			baseQuery := database.DB.Model(&models.Message{}).Where("user_id = ?", tt.authorID)
			query := ApplyMessageVisibilityScope(baseQuery, tt.userID, tt.isAdmin)
			if err := query.Find(&rows).Error; err != nil {
				t.Fatalf("query filtered messages: %v", err)
			}
			got := make([]string, 0, len(rows))
			for _, row := range rows {
				got = append(got, row.Content)
			}
			sort.Strings(got)
			if !sameStringSlice(got, tt.want) {
				t.Fatalf("visible filtered messages = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestGuestCannotLikePublicMessage(t *testing.T) {
	db := setupUserServiceTestDB(t)
	alice := mustCreateUser(t, models.User{Username: "guest-like-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	message := models.Message{Content: "public message", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create public message: %v", err)
	}

	if liked, count, err := ToggleLike(message.ID, nil, "guest-session", false); err == nil || liked || count != 0 {
		t.Fatalf("guest toggle like should fail, liked=%v count=%d err=%v", liked, count, err)
	}
	if created, count, err := IncrementLikeCount(message.ID, 0, false); err == nil || created || count != 0 {
		t.Fatalf("guest increment like should fail, created=%v count=%d err=%v", created, count, err)
	}

	var likeCount int64
	if err := db.Model(&models.MessageLike{}).Where("message_id = ?", message.ID).Count(&likeCount).Error; err != nil {
		t.Fatalf("count likes: %v", err)
	}
	if likeCount != 0 {
		t.Fatalf("expected no guest likes stored, got %d", likeCount)
	}
}

func TestUserCannotLikeInvisiblePrivateMessage(t *testing.T) {
	db := setupUserServiceTestDB(t)
	alice := mustCreateUser(t, models.User{Username: "private-like-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "private-like-bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32)})
	message := models.Message{Content: "alice private", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPrivate, Private: true}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create private message: %v", err)
	}

	bobID := bob.ID
	if liked, count, err := ToggleLike(message.ID, &bobID, "", false); err == nil || liked || count != 0 {
		t.Fatalf("invisible private like should fail, liked=%v count=%d err=%v", liked, count, err)
	}

	var likeCount int64
	if err := db.Model(&models.MessageLike{}).Where("message_id = ?", message.ID).Count(&likeCount).Error; err != nil {
		t.Fatalf("count likes: %v", err)
	}
	if likeCount != 0 {
		t.Fatalf("expected no invisible private likes stored, got %d", likeCount)
	}
}

func TestAdminCannotLikeOthersPrivateMessage(t *testing.T) {
	db := setupUserServiceTestDB(t)
	admin := mustCreateUser(t, models.User{Username: "like-admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "like-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	message := models.Message{Content: "alice private", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPrivate, Private: true}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create private message: %v", err)
	}

	adminID := admin.ID
	visible, err := GetMessageByIDForViewer(message.ID, &adminID, true)
	if err != nil {
		t.Fatalf("admin should still read private message: %v", err)
	}
	if visible == nil || visible.ID != message.ID {
		t.Fatalf("admin read wrong message: %#v", visible)
	}
	liked, count, err := ToggleLike(message.ID, &adminID, "", true)
	if err == nil {
		t.Fatalf("expected admin like on other private message to fail")
	}
	if liked || count != 0 {
		t.Fatalf("rejected like returned liked=%v count=%d", liked, count)
	}
	var likeCount int64
	if err := db.Model(&models.MessageLike{}).Where("message_id = ?", message.ID).Count(&likeCount).Error; err != nil {
		t.Fatalf("count likes: %v", err)
	}
	if likeCount != 0 {
		t.Fatalf("expected no likes stored, got %d", likeCount)
	}
}

func TestGetMessagesByPageFiltersByShanghaiDate(t *testing.T) {
	db := setupUserServiceTestDB(t)
	alice := mustCreateUser(t, models.User{Username: "calendar-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	loc := shanghaiLocation()
	messages := []models.Message{
		{Content: "day-before", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic, CreatedAt: time.Date(2026, 1, 1, 23, 30, 0, 0, loc)},
		{Content: "target-day", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic, CreatedAt: time.Date(2026, 1, 2, 12, 0, 0, 0, loc)},
		{Content: "day-after", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic, CreatedAt: time.Date(2026, 1, 3, 0, 0, 0, 0, loc)},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	date := "2026-01-02"
	result, err := GetMessagesByPage(1, 10, nil, false, nil, nil, &date)
	if err != nil {
		t.Fatalf("get messages by date: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Content != "target-day" {
		t.Fatalf("date filtered result = total %d items %#v, want only target-day", result.Total, result.Items)
	}
}

func TestUpdateMessageAcceptsVisibilityAndLegacyPrivatePayloads(t *testing.T) {
	db := setupUserServiceTestDB(t)
	alice := mustCreateUser(t, models.User{Username: "update-visibility-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	message := models.Message{Content: "editable", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	usersVisibility := MessageVisibilityUsers
	updated, err := UpdateMessage(message.ID, nil, nil, &usersVisibility, nil)
	if err != nil {
		t.Fatalf("update visibility: %v", err)
	}
	if updated.Visibility != MessageVisibilityUsers || !updated.Private {
		t.Fatalf("updated users visibility/private = %q/%v, want users/true", updated.Visibility, updated.Private)
	}

	legacyPublic := false
	updated, err = UpdateMessage(message.ID, nil, &legacyPublic, nil, nil)
	if err != nil {
		t.Fatalf("update legacy private flag: %v", err)
	}
	if updated.Visibility != MessageVisibilityPublic || updated.Private {
		t.Fatalf("updated legacy public visibility/private = %q/%v, want public/false", updated.Visibility, updated.Private)
	}
}

func TestVoceChatContactCacheAllowsContactsVisibility(t *testing.T) {
	db := setupUserServiceTestDB(t)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	enableVoceChatContactsForTest(t)

	alice := mustCreateUser(t, models.User{Username: "contact_alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32), VoceChatEmail: "alice@vc.com", VoceChatUserID: "101"})
	bob := mustCreateUser(t, models.User{Username: "contact_bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32), VoceChatEmail: "bob@vc.com", VoceChatUserID: "202"})
	charlie := mustCreateUser(t, models.User{Username: "contact_charlie", Password: models.HashPassword("charlie"), Token: models.GenerateToken(32), VoceChatEmail: "charlie@vc.com", VoceChatUserID: "303"})
	if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(alice.ID, alice.Username, "alice-vc-password", alice.VoceChatEmail, alice.VoceChatUserID); err != nil {
		t.Fatalf("seed author plain password: %v", err)
	}

	message := models.Message{Content: "alice contacts", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityContacts}
	if err := ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("apply contacts visibility: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contacts message: %v", err)
	}

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		if email != "alice@vc.com" || password != "alice-vc-password" {
			t.Fatalf("vc author login args email=%q password=%q", email, password)
		}
		return &vocechat.LoginResponse{Token: "alice-token"}, nil
	})
	stubVoceChatListContacts(t, func(ctx context.Context, config vocechat.Config, apiKey string) ([]vocechat.UserContact, error) {
		if apiKey != "alice-token" {
			t.Fatalf("contacts api key = %q", apiKey)
		}
		return []vocechat.UserContact{{TargetUID: 202}}, nil
	})

	bobID := bob.ID
	var rows []models.Message
	query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), &bobID, false)
	if err := query.Find(&rows).Error; err != nil {
		t.Fatalf("query visible messages: %v", err)
	}
	if len(rows) != 1 || rows[0].Content != "alice contacts" {
		t.Fatalf("bob visible rows = %#v", rows)
	}

	if err := database.DB.Where("user_id = ?", alice.ID).Delete(&models.VoceChatContactCache{}).Error; err != nil {
		t.Fatalf("clear contact cache: %v", err)
	}
	if _, err := GetMessageByIDForViewer(message.ID, &bobID, false); err != nil {
		t.Fatalf("bob should access contacts detail after cache refresh: %v", err)
	}

	charlieID := charlie.ID
	if _, err := GetMessageByIDForViewer(message.ID, &charlieID, false); err == nil {
		t.Fatalf("non-contact viewer should not access contacts detail")
	}
}

func TestVoceChatContactCacheUsesConfiguredAdminForSysAdminAuthor(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatContactsForTest(t)

	admin := mustCreateUser(t, models.User{Username: "sys_admin_contacts", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "admin_contact_bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32), VoceChatEmail: "bob@vc.com", VoceChatUserID: "202"})
	if admin.ID != 1 {
		t.Fatalf("admin ID = %d, want 1", admin.ID)
	}

	message := models.Message{Content: "admin contacts", Username: admin.Username, UserID: admin.ID, Visibility: MessageVisibilityContacts}
	if err := ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("apply contacts visibility: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contacts message: %v", err)
	}

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		if email != "admin@vc.com" || password != "admin-secret" {
			t.Fatalf("vc admin login args email=%q password=%q", email, password)
		}
		return &vocechat.LoginResponse{Token: "admin-token", User: vocechat.UserInfo{UID: 99, Email: "admin@vc.com", IsAdmin: true}}, nil
	})
	stubVoceChatListContacts(t, func(ctx context.Context, config vocechat.Config, apiKey string) ([]vocechat.UserContact, error) {
		if apiKey != "admin-token" {
			t.Fatalf("contacts api key = %q", apiKey)
		}
		return []vocechat.UserContact{{TargetUID: 202}}, nil
	})

	bobID := bob.ID
	var rows []models.Message
	query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), &bobID, false)
	if err := query.Find(&rows).Error; err != nil {
		t.Fatalf("query visible messages: %v", err)
	}
	if len(rows) != 1 || rows[0].Content != "admin contacts" {
		t.Fatalf("bob visible rows = %#v", rows)
	}

	var storedAdmin models.User
	if err := database.DB.First(&storedAdmin, admin.ID).Error; err != nil {
		t.Fatalf("load stored admin: %v", err)
	}
	if storedAdmin.VoceChatUserID != "" || storedAdmin.VoceChatEmail != "" || (storedAdmin.VoceChatSyncStatus != "" && storedAdmin.VoceChatSyncStatus != models.VoceChatSyncStatusNone) {
		t.Fatalf("admin should not be bound by contacts sync, got uid %q email %q status %q", storedAdmin.VoceChatUserID, storedAdmin.VoceChatEmail, storedAdmin.VoceChatSyncStatus)
	}

	var marker models.VoceChatContactCache
	if err := database.DB.Where("user_id = ? AND contact_user_id = ?", admin.ID, 0).First(&marker).Error; err != nil {
		t.Fatalf("load admin contact marker: %v", err)
	}
	if marker.VoceChatUserID != "99" || marker.LastSyncStatus != models.VoceChatContactSyncStatusOK {
		t.Fatalf("admin contact marker = %#v", marker)
	}
}

func TestVoceChatContactVisibilityDisabledIgnoresFreshCache(t *testing.T) {
	db := setupUserServiceTestDB(t)
	if err := database.DB.Create(&models.SiteConfig{
		SiteTitle:                       "Test Site",
		VoceChatEnabled:                 true,
		VoceChatContactsEnabled:         false,
		VoceChatContactsCacheTTLSeconds: 3600,
	}).Error; err != nil {
		t.Fatalf("create disabled contacts site config: %v", err)
	}

	alice := mustCreateUser(t, models.User{Username: "disabled_contact_alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32), VoceChatEmail: "alice@vc.com", VoceChatUserID: "101"})
	bob := mustCreateUser(t, models.User{Username: "disabled_contact_bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32), VoceChatEmail: "bob@vc.com", VoceChatUserID: "202"})
	message := models.Message{Content: "disabled contacts", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityContacts}
	if err := ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("apply contacts visibility: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contacts message: %v", err)
	}
	now := time.Now().UTC()
	if err := database.DB.Create(&models.VoceChatContactCache{
		UserID:            alice.ID,
		ContactUserID:     bob.ID,
		VoceChatUserID:    alice.VoceChatUserID,
		ContactVoceChatID: bob.VoceChatUserID,
		Source:            "vocechat",
		SyncedAt:          now,
		ExpiresAt:         now.Add(time.Hour),
		LastSyncStatus:    models.VoceChatContactSyncStatusOK,
	}).Error; err != nil {
		t.Fatalf("seed fresh contact cache: %v", err)
	}

	bobID := bob.ID
	var rows []models.Message
	query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), &bobID, false)
	if err := query.Find(&rows).Error; err != nil {
		t.Fatalf("query visible messages: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("disabled contacts visibility should ignore cache, got %#v", rows)
	}
	if _, err := GetMessageByIDForViewer(message.ID, &bobID, false); err == nil {
		t.Fatalf("disabled contacts detail should remain hidden")
	}
}

func TestAdminCannotLikeContactsMessageHiddenFromOwnSocialGraph(t *testing.T) {
	db := setupUserServiceTestDB(t)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	enableVoceChatContactsForTest(t)

	admin := mustCreateUser(t, models.User{Username: "like_admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32), VoceChatEmail: "admin@vc.com", VoceChatUserID: "1"})
	alice := mustCreateUser(t, models.User{Username: "like_alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32), VoceChatEmail: "alice@vc.com", VoceChatUserID: "101"})

	message := models.Message{Content: "alice contacts", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityContacts}
	if err := ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("apply contacts visibility: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contacts message: %v", err)
	}

	adminID := admin.ID
	if _, err := GetMessageByIDForViewer(message.ID, &adminID, true); err != nil {
		t.Fatalf("admin should still read contacts message for moderation: %v", err)
	}
	if liked, count, err := ToggleLike(message.ID, &adminID, "", true); err == nil || liked || count != 0 {
		t.Fatalf("admin should not like contacts message outside own contacts, liked=%v count=%d err=%v", liked, count, err)
	}

	var likeCount int64
	if err := database.DB.Model(&models.MessageLike{}).Where("message_id = ?", message.ID).Count(&likeCount).Error; err != nil {
		t.Fatalf("count likes: %v", err)
	}
	if likeCount != 0 {
		t.Fatalf("unexpected like rows = %d", likeCount)
	}
}

func TestVoceChatContactCacheFailureKeepsContactsPrivate(t *testing.T) {
	db := setupUserServiceTestDB(t)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	enableVoceChatContactsForTest(t)

	alice := mustCreateUser(t, models.User{Username: "fail_alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32), VoceChatEmail: "alice@vc.com", VoceChatUserID: "101"})
	bob := mustCreateUser(t, models.User{Username: "fail_bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32), VoceChatEmail: "bob@vc.com", VoceChatUserID: "202"})
	if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(alice.ID, alice.Username, "alice-vc-password", alice.VoceChatEmail, alice.VoceChatUserID); err != nil {
		t.Fatalf("seed author plain password: %v", err)
	}

	message := models.Message{Content: "alice contacts", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityContacts}
	if err := ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("apply contacts visibility: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contacts message: %v", err)
	}

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{Token: "alice-token"}, nil
	})
	listCalls := 0
	stubVoceChatListContacts(t, func(ctx context.Context, config vocechat.Config, apiKey string) ([]vocechat.UserContact, error) {
		listCalls++
		return nil, errors.New("vc contacts unavailable")
	})

	bobID := bob.ID
	for i := 0; i < 2; i++ {
		var rows []models.Message
		query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), &bobID, false)
		if err := query.Find(&rows).Error; err != nil {
			t.Fatalf("query visible messages: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("contacts message should be private on sync failure, got %#v", rows)
		}
	}
	if listCalls != 1 {
		t.Fatalf("contacts sync should be cooled down after failure, calls=%d", listCalls)
	}
	if _, err := GetMessageByIDForViewer(message.ID, &bobID, false); err == nil {
		t.Fatalf("contacts detail should remain hidden on sync failure")
	}

	var marker models.VoceChatContactCache
	if err := database.DB.Where("user_id = ?", alice.ID).First(&marker).Error; err != nil {
		t.Fatalf("read failure marker: %v", err)
	}
	if marker.ContactUserID != 0 || marker.LastSyncStatus != models.VoceChatContactSyncStatusFailed || marker.LastSyncError == "" {
		t.Fatalf("unexpected failure marker: %#v", marker)
	}
}

func enableVoceChatContactsForTest(t *testing.T) {
	t.Helper()
	if err := database.DB.Create(&models.SiteConfig{
		SiteTitle:                       "Test Site",
		VoceChatEnabled:                 true,
		VoceChatBaseURL:                 "https://vc.example.com",
		VoceChatAdminUsername:           "admin@vc.com",
		VoceChatAdminPassword:           "admin-secret",
		VoceChatContactsEnabled:         true,
		VoceChatContactsCacheTTLSeconds: 3600,
	}).Error; err != nil {
		t.Fatalf("create vc contacts site config: %v", err)
	}
}

func stubVoceChatListContacts(t *testing.T, fn voceChatListContactsFunc) {
	t.Helper()
	original := voceChatListContacts
	voceChatListContacts = fn
	t.Cleanup(func() { voceChatListContacts = original })
}

func sameStringSlice(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
