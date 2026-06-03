package services

import (
	"sort"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
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
