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
		name   string
		userID *uint
		want   []string
	}{
		{name: "guest sees public only", want: []string{"alice public", "bob public"}},
		{name: "member sees public users and own restricted", userID: &aliceID, want: []string{"alice contacts", "alice private", "alice public", "alice users", "bob public", "bob users"}},
		{name: "primary administrator resolves full read scope", userID: &admin.ID, want: []string{"alice contacts", "alice private", "alice public", "alice users", "bob contacts", "bob private", "bob public", "bob users"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rows []models.Message
			query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), tt.userID)
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
		authorID uint
		want     []string
	}{
		{name: "member viewing another author keeps author filter", userID: &aliceID, authorID: bobID, want: []string{"bob public", "bob users"}},
		{name: "member viewing self keeps all own states", userID: &aliceID, authorID: aliceID, want: []string{"alice contacts", "alice private", "alice public", "alice users"}},
		{name: "guest viewing author sees public only", authorID: aliceID, want: []string{"alice public"}},
		{name: "primary administrator author scope sees all states", userID: &admin.ID, authorID: bobID, want: []string{"bob contacts", "bob private", "bob public", "bob users"}},
	}

	for _, tt := range prefilterTests {
		t.Run(tt.name, func(t *testing.T) {
			var rows []models.Message
			baseQuery := database.DB.Model(&models.Message{}).Where("user_id = ?", tt.authorID)
			query := ApplyMessageVisibilityScope(baseQuery, tt.userID)
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

	if liked, count, err := ToggleLike(message.ID, nil, "guest-session"); err == nil || liked || count != 0 {
		t.Fatalf("guest toggle like should fail, liked=%v count=%d err=%v", liked, count, err)
	}
	if created, count, err := IncrementLikeCount(message.ID, 0); err == nil || created || count != 0 {
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
	if liked, count, err := ToggleLike(message.ID, &bobID, ""); err == nil || liked || count != 0 {
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
	visible, err := GetMessageByIDForViewer(message.ID, &adminID)
	if err != nil {
		t.Fatalf("admin should still read private message: %v", err)
	}
	if visible == nil || visible.ID != message.ID {
		t.Fatalf("admin read wrong message: %#v", visible)
	}
	liked, count, err := ToggleLike(message.ID, &adminID, "")
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
	result, err := GetMessagesByPage(1, 10, nil, nil, nil, &date, nil, nil, MessagePinScopeLatest, nil)
	if err != nil {
		t.Fatalf("get messages by date: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Content != "target-day" {
		t.Fatalf("date filtered result = total %d items %#v, want only target-day", result.Total, result.Items)
	}
}

func TestGetMessagesByPageCombinesDateKeywordAndTag(t *testing.T) {
	db := setupUserServiceTestDB(t)
	alice := mustCreateUser(t, models.User{Username: "combined-filter-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	loc := shanghaiLocation()
	messages := []models.Message{
		{Content: "#总结 工作记录 命中", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic, CreatedAt: time.Date(2026, 6, 6, 9, 0, 0, 0, loc)},
		{Content: "#总结 工作记录 日期不符", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic, CreatedAt: time.Date(2026, 6, 7, 9, 0, 0, 0, loc)},
		{Content: "#总结 生活记录", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic, CreatedAt: time.Date(2026, 6, 6, 10, 0, 0, 0, loc)},
		{Content: "#总结会 工作记录", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic, CreatedAt: time.Date(2026, 6, 6, 11, 0, 0, 0, loc)},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	date := "2026-06-06"
	keyword := "工作记录"
	tag := "总结"
	result, err := GetMessagesByPage(1, 10, nil, nil, nil, &date, &keyword, &tag, MessagePinScopeLatest, nil)
	if err != nil {
		t.Fatalf("get messages by combined filters: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Content != "#总结 工作记录 命中" {
		t.Fatalf("combined filtered result = total %d items %#v, want only exact hit", result.Total, result.Items)
	}
	location, err := LocateMessagePage(messages[0].ID, 10, nil, nil, nil, &date, &keyword, &tag, MessagePinScopeLatest, nil)
	if err != nil {
		t.Fatalf("locate message by combined filters: %v", err)
	}
	if location.Page != 1 || location.Total != 1 {
		t.Fatalf("combined filter location = page %d total %d, want page 1 total 1", location.Page, location.Total)
	}
	if _, err := LocateMessagePage(messages[3].ID, 10, nil, nil, nil, &date, &keyword, &tag, MessagePinScopeLatest, nil); err == nil {
		t.Fatalf("expected similar but non-exact tag to be unavailable to locate")
	}
}

func TestGetMessagesByPageAndLocateRespectExcludeID(t *testing.T) {
	db := setupUserServiceTestDB(t)
	alice := mustCreateUser(t, models.User{Username: "exclude-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	keep := models.Message{Content: "keep", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic}
	exclude := models.Message{Content: "exclude", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&keep).Error; err != nil {
		t.Fatalf("create keep message: %v", err)
	}
	if err := db.Create(&exclude).Error; err != nil {
		t.Fatalf("create excluded message: %v", err)
	}

	excludeID := exclude.ID
	result, err := GetMessagesByPage(1, 10, nil, nil, nil, nil, nil, nil, MessagePinScopeLatest, &excludeID)
	if err != nil {
		t.Fatalf("query page with exclude id: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != keep.ID {
		t.Fatalf("exclude filtered result = total %d items %#v, want only keep", result.Total, result.Items)
	}

	location, err := LocateMessagePage(keep.ID, 10, nil, nil, nil, nil, nil, nil, MessagePinScopeLatest, &excludeID)
	if err != nil {
		t.Fatalf("locate keep with exclude id: %v", err)
	}
	if location.Page != 1 || location.Total != 1 {
		t.Fatalf("location = page %d total %d, want page 1 total 1", location.Page, location.Total)
	}
	if _, err := LocateMessagePage(exclude.ID, 10, nil, nil, nil, nil, nil, nil, MessagePinScopeLatest, &excludeID); err == nil {
		t.Fatalf("expected excluded message to be unavailable to locate")
	}
}

func TestGetMessagesByPageReturnsViewerLikedState(t *testing.T) {
	db := setupUserServiceTestDB(t)
	owner := mustCreateUser(t, models.User{Username: "like-owner", Password: models.HashPassword("owner"), Token: models.GenerateToken(32)})
	viewer := mustCreateUser(t, models.User{Username: "like-viewer", Password: models.HashPassword("viewer"), Token: models.GenerateToken(32)})

	liked := models.Message{Username: owner.Username, UserID: owner.ID, Content: "liked", Visibility: MessageVisibilityPublic, LikeCount: 1}
	unliked := models.Message{Username: owner.Username, UserID: owner.ID, Content: "unliked", Visibility: MessageVisibilityPublic}
	if err := db.Create(&liked).Error; err != nil {
		t.Fatalf("create liked message: %v", err)
	}
	if err := db.Create(&unliked).Error; err != nil {
		t.Fatalf("create unliked message: %v", err)
	}
	viewerID := viewer.ID
	if err := db.Create(&models.MessageLike{MessageID: liked.ID, UserID: &viewerID}).Error; err != nil {
		t.Fatalf("create message like: %v", err)
	}

	result, err := GetMessagesByPage(1, 10, &viewerID, nil, nil, nil, nil, nil, MessagePinScopeLatest, nil)
	if err != nil {
		t.Fatalf("query page: %v", err)
	}

	likedByID := map[uint]bool{}
	for _, message := range result.Items {
		likedByID[message.ID] = message.Liked
	}
	if !likedByID[liked.ID] {
		t.Fatalf("expected liked message %d to be marked liked", liked.ID)
	}
	if likedByID[unliked.ID] {
		t.Fatalf("expected unliked message %d to be marked unliked", unliked.ID)
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
		return []vocechat.UserContact{voceChatContactWithStatus(202, vocechat.ContactStatusAdded)}, nil
	})

	bobID := bob.ID
	var rows []models.Message
	query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), &bobID)
	if err := query.Find(&rows).Error; err != nil {
		t.Fatalf("query visible messages: %v", err)
	}
	if len(rows) != 1 || rows[0].Content != "alice contacts" {
		t.Fatalf("bob visible rows = %#v", rows)
	}

	if err := database.DB.Where("user_id = ?", alice.ID).Delete(&models.VoceChatContactCache{}).Error; err != nil {
		t.Fatalf("clear contact cache: %v", err)
	}
	if _, err := GetMessageByIDForViewer(message.ID, &bobID); err != nil {
		t.Fatalf("bob should access contacts detail after cache refresh: %v", err)
	}

	charlieID := charlie.ID
	if _, err := GetMessageByIDForViewer(message.ID, &charlieID); err == nil {
		t.Fatalf("non-contact viewer should not access contacts detail")
	}
}

func TestVoceChatContactCacheOnlyGrantsAddedContactsVisibility(t *testing.T) {
	db := setupUserServiceTestDB(t)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	enableVoceChatContactsForTest(t)

	alice := mustCreateUser(t, models.User{Username: "status_alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32), VoceChatEmail: "alice@vc.com", VoceChatUserID: "101"})
	added := mustCreateUser(t, models.User{Username: "status_added", Password: models.HashPassword("added"), Token: models.GenerateToken(32), VoceChatEmail: "added@vc.com", VoceChatUserID: "202"})
	blocked := mustCreateUser(t, models.User{Username: "status_blocked", Password: models.HashPassword("blocked"), Token: models.GenerateToken(32), VoceChatEmail: "blocked@vc.com", VoceChatUserID: "303"})
	if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(alice.ID, alice.Username, "alice-vc-password", alice.VoceChatEmail, alice.VoceChatUserID); err != nil {
		t.Fatalf("seed author plain password: %v", err)
	}
	message := models.Message{Content: "status-filtered contacts", Username: alice.Username, UserID: alice.ID, Visibility: MessageVisibilityContacts}
	if err := ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("apply contacts visibility: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contacts message: %v", err)
	}

	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{Token: "alice-token"}, nil
	})
	stubVoceChatListContacts(t, func(context.Context, vocechat.Config, string) ([]vocechat.UserContact, error) {
		return []vocechat.UserContact{
			voceChatContactWithStatus(202, vocechat.ContactStatusAdded),
			voceChatContactWithStatus(303, vocechat.ContactStatusBlocked),
		}, nil
	})

	addedID := added.ID
	if _, err := GetMessageByIDForViewer(message.ID, &addedID); err != nil {
		t.Fatalf("added contact should see contacts message: %v", err)
	}
	blockedID := blocked.ID
	if _, err := GetMessageByIDForViewer(message.ID, &blockedID); err == nil {
		t.Fatalf("blocked contact must not see contacts message")
	}
}

func TestVoceChatContactCacheUsesPrimaryAdminsPersonalBinding(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatContactsForTest(t)
	storePath := filepath.Join(t.TempDir(), "primary-contact-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)

	admin := mustCreateUser(t, models.User{Username: "sys_admin_contacts", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32), VoceChatEmail: "personal@vc.com", VoceChatUserID: "99"})
	bob := mustCreateUser(t, models.User{Username: "admin_contact_bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32), VoceChatEmail: "bob@vc.com", VoceChatUserID: "202"})
	if admin.ID != 1 {
		t.Fatalf("admin ID = %d, want 1", admin.ID)
	}
	if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(admin.ID, admin.Username, "personal-secret", admin.VoceChatEmail, admin.VoceChatUserID); err != nil {
		t.Fatal(err)
	}

	message := models.Message{Content: "admin contacts", Username: admin.Username, UserID: admin.ID, Visibility: MessageVisibilityContacts}
	if err := ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("apply contacts visibility: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contacts message: %v", err)
	}

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		if email != "personal@vc.com" || password != "personal-secret" {
			t.Fatalf("vc admin login args email=%q password=%q", email, password)
		}
		return &vocechat.LoginResponse{Token: "personal-token", User: vocechat.UserInfo{UID: 99, Email: "personal@vc.com"}}, nil
	})
	stubVoceChatListContacts(t, func(ctx context.Context, config vocechat.Config, apiKey string) ([]vocechat.UserContact, error) {
		if apiKey != "personal-token" {
			t.Fatalf("contacts api key = %q", apiKey)
		}
		return []vocechat.UserContact{voceChatContactWithStatus(202, vocechat.ContactStatusAdded)}, nil
	})

	bobID := bob.ID
	var rows []models.Message
	query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), &bobID)
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
	if storedAdmin.VoceChatUserID != "99" || storedAdmin.VoceChatEmail != "personal@vc.com" {
		t.Fatalf("primary binding changed, got uid %q email %q", storedAdmin.VoceChatUserID, storedAdmin.VoceChatEmail)
	}

	var marker models.VoceChatContactCache
	if err := database.DB.Where("user_id = ? AND contact_user_id = ?", admin.ID, 0).First(&marker).Error; err != nil {
		t.Fatalf("load admin contact marker: %v", err)
	}
	if marker.VoceChatUserID != "99" || marker.LastSyncStatus != models.VoceChatContactSyncStatusOK {
		t.Fatalf("admin contact marker = %#v", marker)
	}
}

func TestPrimaryAdminInvalidVoceChatCredentialsFailClosedAndNotifyOnce(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatContactsForTest(t)
	storePath := filepath.Join(t.TempDir(), "invalid-primary-contact.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	admin := mustCreateUser(t, models.User{Username: "primary-invalid-vc", Password: models.HashPassword("local"), IsAdmin: true, VoceChatEmail: "invalid@vc.com", VoceChatUserID: "98"})
	viewer := mustCreateUser(t, models.User{Username: "viewer-invalid-vc", Password: models.HashPassword("viewer"), VoceChatEmail: "viewer@vc.com", VoceChatUserID: "202"})
	if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(admin.ID, admin.Username, "stale-password", admin.VoceChatEmail, admin.VoceChatUserID); err != nil {
		t.Fatal(err)
	}
	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		return nil, &vocechat.APIError{StatusCode: 401}
	})
	message := models.Message{Content: "fail closed", Username: admin.Username, UserID: admin.ID, Visibility: MessageVisibilityContacts}
	if err := CreateMessage(&message); err != nil {
		t.Fatal(err)
	}
	viewerID := viewer.ID
	if CanViewMessage(message, &viewerID) {
		t.Fatal("invalid primary credentials must make contacts content private")
	}
	_ = EnsureVoceChatContactCacheForAuthor(admin.ID)
	_ = EnsureVoceChatContactCacheForAuthor(admin.ID)
	var count int64
	if err := db.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", admin.ID, models.UserNotificationTypeVoceChatCredentials).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("credential alerts=%d err=%v", count, err)
	}
}

func TestNonPrimaryInvalidVoceChatCredentialsNotifyOnceForOrdinaryAndDelegatedUsers(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatContactsForTest(t)
	storePath := filepath.Join(t.TempDir(), "invalid-non-primary-contact.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary-contact-owner", Password: models.HashPassword("primary"), IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "ordinary-invalid-vc", Password: models.HashPassword("ordinary"), VoceChatEmail: "ordinary-invalid@vc.com", VoceChatUserID: "201"})
	delegated := mustCreateUser(t, models.User{Username: "delegated-invalid-vc", Password: models.HashPassword("delegated"), IsAdmin: true, VoceChatEmail: "delegated-invalid@vc.com", VoceChatUserID: "202"})
	for _, user := range []*models.User{ordinary, delegated} {
		if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(user.ID, user.Username, "stale-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
			t.Fatal(err)
		}
	}
	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		return nil, &vocechat.APIError{StatusCode: 401}
	})

	for _, user := range []*models.User{ordinary, delegated} {
		_ = EnsureVoceChatContactCacheForAuthor(user.ID)
		_ = RefreshVoceChatContactCacheForAuthor(user.ID)
		var count int64
		if err := db.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("recipient %d password-change alerts=%d err=%v", user.ID, count, err)
		}
	}
}

func TestSuccessfulNonPrimaryVoceChatContactSyncResolvesPasswordChangedAlert(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatContactsForTest(t)
	storePath := filepath.Join(t.TempDir(), "resolved-non-primary-contact.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary-contact-owner", Password: models.HashPassword("primary"), IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "ordinary-resolved-vc", Password: models.HashPassword("ordinary"), VoceChatEmail: "ordinary-resolved@vc.com", VoceChatUserID: "301"})
	delegated := mustCreateUser(t, models.User{Username: "delegated-resolved-vc", Password: models.HashPassword("delegated"), IsAdmin: true, VoceChatEmail: "delegated-resolved@vc.com", VoceChatUserID: "302"})
	for _, user := range []*models.User{ordinary, delegated} {
		if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(user.ID, user.Username, "current-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
			t.Fatal(err)
		}
		CreateVoceChatPasswordChangedAlertOnce(user.ID)
	}
	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{Token: "contact-token"}, nil
	})
	stubVoceChatListContacts(t, func(context.Context, vocechat.Config, string) ([]vocechat.UserContact, error) {
		return nil, nil
	})

	for _, user := range []*models.User{ordinary, delegated} {
		if err := RefreshVoceChatContactCacheForAuthor(user.ID); err != nil {
			t.Fatalf("refresh contacts for recipient %d: %v", user.ID, err)
		}
		var count int64
		if err := db.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("recipient %d remaining password-change alerts=%d err=%v", user.ID, count, err)
		}
	}
}

func TestPrimaryAdminVoceChatOutageFailsClosedWithoutCredentialAlert(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatContactsForTest(t)
	storePath := filepath.Join(t.TempDir(), "outage-primary-contact.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	admin := mustCreateUser(t, models.User{Username: "primary-vc-outage", Password: models.HashPassword("local"), IsAdmin: true, VoceChatEmail: "owner@vc.com", VoceChatUserID: "97"})
	viewer := mustCreateUser(t, models.User{Username: "viewer-vc-outage", Password: models.HashPassword("viewer"), VoceChatEmail: "viewer@vc.com", VoceChatUserID: "203"})
	if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(admin.ID, admin.Username, "valid-password", admin.VoceChatEmail, admin.VoceChatUserID); err != nil {
		t.Fatal(err)
	}
	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		return nil, &vocechat.APIError{StatusCode: 503, Body: "service unavailable"}
	})
	message := models.Message{Content: "outage fail closed", Username: admin.Username, UserID: admin.ID, Visibility: MessageVisibilityContacts}
	if err := CreateMessage(&message); err != nil {
		t.Fatal(err)
	}
	viewerID := viewer.ID
	if CanViewMessage(message, &viewerID) {
		t.Fatal("VoceChat outage must make contacts content private")
	}
	var count int64
	if err := db.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", admin.ID, models.UserNotificationTypeVoceChatCredentials).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("outage credential alerts=%d err=%v", count, err)
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
	query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), &bobID)
	if err := query.Find(&rows).Error; err != nil {
		t.Fatalf("query visible messages: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("disabled contacts visibility should ignore cache, got %#v", rows)
	}
	if _, err := GetMessageByIDForViewer(message.ID, &bobID); err == nil {
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
	if _, err := GetMessageByIDForViewer(message.ID, &adminID); err != nil {
		t.Fatalf("admin should still read contacts message for moderation: %v", err)
	}
	if liked, count, err := ToggleLike(message.ID, &adminID, ""); err == nil || liked || count != 0 {
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
		query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), &bobID)
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
	if _, err := GetMessageByIDForViewer(message.ID, &bobID); err == nil {
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

func voceChatContactWithStatus(uid int64, status string) vocechat.UserContact {
	contact := vocechat.UserContact{TargetUID: uid}
	contact.ContactInfo.Status = status
	return contact
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
