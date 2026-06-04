package services

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestDefaultHeaderImagesOnlyContainsSelectedImage(t *testing.T) {
	images := defaultHeaderImages()
	if len(images) != 1 {
		t.Fatalf("default header images count = %d, want 1", len(images))
	}
	if images[0] != defaultHeaderImageURL {
		t.Fatalf("default header image = %q, want %q", images[0], defaultHeaderImageURL)
	}
}

func TestShouldCollapseLegacyBackgrounds(t *testing.T) {
	tests := []struct {
		name        string
		backgrounds []string
		want        bool
	}{
		{
			name: "legacy defaults collapse",
			backgrounds: []string{
				"https://s2.loli.net/2025/03/27/KJ1trnU2ksbFEYM.jpg",
				defaultHeaderImageURL,
				"https://s2.loli.net/2025/03/27/y67m2k5xcSdTsHN.jpg",
			},
			want: true,
		},
		{
			name:        "selected default collapses",
			backgrounds: []string{" ", defaultHeaderImageURL},
			want:        true,
		},
		{
			name:        "custom image is preserved",
			backgrounds: []string{defaultHeaderImageURL, "https://example.com/custom.jpg"},
			want:        false,
		},
		{
			name:        "empty list is not legacy data",
			backgrounds: []string{},
			want:        false,
		},
		{
			name:        "blank list is not legacy data",
			backgrounds: []string{" ", ""},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldCollapseLegacyBackgrounds(tt.backgrounds); got != tt.want {
				t.Fatalf("shouldCollapseLegacyBackgrounds() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRSSConfigDefaultsToAdminMembersAndHidesMemberListFromPublic(t *testing.T) {
	db := setupUserServiceTestDB(t)
	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	member := mustCreateUser(t, models.User{Username: "member", Password: models.HashPassword("member"), Token: models.GenerateToken(32)})
	if err := db.Create(&models.SiteConfig{RSSEnabled: false}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}

	publicConfig, err := GetFrontendConfig()
	if err != nil {
		t.Fatalf("get public frontend config: %v", err)
	}
	publicSettings := publicConfig["frontendSettings"].(map[string]interface{})
	if got := publicSettings["rssEnabled"].(bool); got {
		t.Fatalf("public rssEnabled = %v, want false", got)
	}
	if got := publicSettings["rssMemberIDs"].([]uint); len(got) != 0 {
		t.Fatalf("public rssMemberIDs = %#v, want empty", got)
	}
	if got := publicSettings["rssAvailableMembers"].([]map[string]interface{}); len(got) != 0 {
		t.Fatalf("public rssAvailableMembers = %#v, want empty", got)
	}

	memberConfig, err := GetFrontendConfig(member.ID)
	if err != nil {
		t.Fatalf("get member frontend config: %v", err)
	}
	memberSettings := memberConfig["frontendSettings"].(map[string]interface{})
	if got := memberSettings["rssMemberIDs"].([]uint); len(got) != 0 {
		t.Fatalf("non-admin rssMemberIDs = %#v, want empty", got)
	}
	if got := memberSettings["rssAvailableMembers"].([]map[string]interface{}); len(got) != 0 {
		t.Fatalf("non-admin rssAvailableMembers = %#v, want empty", got)
	}

	adminConfig, err := GetFrontendConfig(admin.ID)
	if err != nil {
		t.Fatalf("get admin frontend config: %v", err)
	}
	adminSettings := adminConfig["frontendSettings"].(map[string]interface{})
	ids := adminSettings["rssMemberIDs"].([]uint)
	if len(ids) != 1 || ids[0] != admin.ID {
		t.Fatalf("admin rssMemberIDs = %#v, want [%d]", ids, admin.ID)
	}
	members := adminSettings["rssAvailableMembers"].([]map[string]interface{})
	if len(members) != 2 {
		t.Fatalf("admin rssAvailableMembers count = %d, want 2", len(members))
	}
}

func TestFrontendConfigExposesVoceChatStatusWithoutSecrets(t *testing.T) {
	db := setupUserServiceTestDB(t)
	if err := db.Create(&models.SiteConfig{
		VoceChatEnabled:                  true,
		VoceChatBaseURL:                  "https://vc.example.test/",
		VoceChatAdminUsername:            "admin",
		VoceChatAdminPassword:            "admin-password",
		VoceChatAdminToken:               "admin-token",
		VoceChatThirdPartySecret:         "third-party-secret",
		VoceChatEmailDomain:              "vc.com",
		VoceChatLoginVerificationEnabled: true,
		VoceChatContactsEnabled:          true,
		VoceChatContactsCacheTTLSeconds:  120,
		VoceChatLastHealthStatus:         "ok",
		VoceChatLastHealthError:          "",
	}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}

	config, err := GetFrontendConfig()
	if err != nil {
		t.Fatalf("get frontend config: %v", err)
	}
	voceConfig, ok := config["voceChatConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("voceChatConfig has type %T", config["voceChatConfig"])
	}
	for _, secretKey := range []string{"adminPassword", "adminToken", "thirdPartySecret"} {
		if _, exists := voceConfig[secretKey]; exists {
			t.Fatalf("voceChatConfig exposes secret key %s", secretKey)
		}
	}
	if got := voceConfig["enabled"]; got != true {
		t.Fatalf("enabled = %#v, want true", got)
	}
	if got := voceConfig["configured"]; got != true {
		t.Fatalf("configured = %#v, want true", got)
	}
	if got := voceConfig["adminCredentialConfigured"]; got != true {
		t.Fatalf("adminCredentialConfigured = %#v, want true", got)
	}
	if got := voceConfig["thirdPartySecretConfigured"]; got != true {
		t.Fatalf("thirdPartySecretConfigured = %#v, want true", got)
	}
	if got := voceConfig["contactsCacheTTLSeconds"]; got != 120 {
		t.Fatalf("contactsCacheTTLSeconds = %#v, want 120", got)
	}
}

func TestApplyVoceChatConfigRejectsAdminDisplayNameWhenPasswordLoginIsUsed(t *testing.T) {
	config := &models.SiteConfig{}
	err := applyVoceChatConfigUpdate(config, map[string]interface{}{
		"enabled":       true,
		"baseURL":       "https://vc.example.test",
		"adminUsername": "Noise",
		"adminPassword": "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "管理员邮箱格式无效") {
		t.Fatalf("apply voce config err = %v, want admin email validation error", err)
	}
}

func TestApplyVoceChatConfigAllowsAdminTokenWithoutAdminEmail(t *testing.T) {
	config := &models.SiteConfig{}
	if err := applyVoceChatConfigUpdate(config, map[string]interface{}{
		"enabled":    true,
		"baseURL":    "https://vc.example.test",
		"adminToken": "configured-token",
	}); err != nil {
		t.Fatalf("apply voce config with token: %v", err)
	}
}

func TestGenerateRSSExportsOnlySelectedMembersPublicMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupUserServiceTestDB(t)
	admin := mustCreateUser(t, models.User{Username: "rss-admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	member := mustCreateUser(t, models.User{Username: "rss-member", Password: models.HashPassword("member"), Token: models.GenerateToken(32)})
	other := mustCreateUser(t, models.User{Username: "rss-other", Password: models.HashPassword("other"), Token: models.GenerateToken(32)})
	if err := db.Create(&models.SiteConfig{
		RSSEnabled:    true,
		RSSMemberIDs:  fmt.Sprintf("[%d,%d]", admin.ID, member.ID),
		RSSTitle:      "Selected RSS",
		RSSAuthorName: "Noise",
		RSSFaviconURL: "/favicon-32x32.png",
	}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}

	now := time.Now()
	messages := []models.Message{
		{Content: "admin-public-rss-entry\nvisible", Username: admin.Username, UserID: admin.ID, CreatedAt: now.Add(-4 * time.Hour)},
		{Content: "member-public-rss-entry\nvisible", Username: member.Username, UserID: member.ID, CreatedAt: now.Add(-3 * time.Hour)},
		{Content: "member-private-rss-entry", Username: member.Username, UserID: member.ID, Private: true, CreatedAt: now.Add(-2 * time.Hour)},
		{Content: "other-public-rss-entry", Username: other.Username, UserID: other.ID, CreatedAt: now.Add(-1 * time.Hour)},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "http://example.test/rss", nil)

	rss, err := GenerateRSS(c)
	if err != nil {
		t.Fatalf("generate rss: %v", err)
	}
	for _, want := range []string{"admin-public-rss-entry", "member-public-rss-entry"} {
		if !strings.Contains(rss, want) {
			t.Fatalf("rss missing selected public entry %q:\n%s", want, rss)
		}
	}
	for _, unwanted := range []string{"member-private-rss-entry", "other-public-rss-entry"} {
		if strings.Contains(rss, unwanted) {
			t.Fatalf("rss contains excluded entry %q:\n%s", unwanted, rss)
		}
	}
}

func TestMessagesCalendarGroupsByShanghaiDate(t *testing.T) {
	db := setupUserServiceTestDB(t)
	admin := mustCreateUser(t, models.User{Username: "calendar-admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	messages := []models.Message{
		{
			Content:   "early morning in shanghai",
			Username:  admin.Username,
			UserID:    admin.ID,
			CreatedAt: time.Date(2026, 5, 31, 23, 30, 0, 0, time.UTC), // 2026-06-01 07:30 Asia/Shanghai
		},
		{
			Content:   "same local day",
			Username:  admin.Username,
			UserID:    admin.ID,
			CreatedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	calendar, err := GetMessagesGroupByDate(nil, false, nil)
	if err != nil {
		t.Fatalf("get calendar: %v", err)
	}
	counts := messageDateCounts(calendar)
	if got := counts["2026-06-01"]; got != 2 {
		t.Fatalf("2026-06-01 count = %d, want 2; rows=%#v", got, calendar)
	}
	if got := counts["2026-05-31"]; got != 0 {
		t.Fatalf("2026-05-31 count = %d, want 0; rows=%#v", got, calendar)
	}
}

func TestMessagesCalendarRespectsVisibilityAndPersonalScope(t *testing.T) {
	db := setupUserServiceTestDB(t)
	admin := mustCreateUser(t, models.User{Username: "calendar-admin-visibility", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "calendar-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "calendar-bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32)})
	day := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	messages := []models.Message{
		{Content: "alice public", Username: alice.Username, UserID: alice.ID, CreatedAt: day},
		{Content: "alice private", Username: alice.Username, UserID: alice.ID, Private: true, CreatedAt: day},
		{Content: "bob public", Username: bob.Username, UserID: bob.ID, CreatedAt: day},
		{Content: "bob private", Username: bob.Username, UserID: bob.ID, Private: true, CreatedAt: day},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	aliceID := alice.ID
	bobID := bob.ID
	tests := []struct {
		name     string
		userID   *uint
		isAdmin  bool
		authorID *uint
		want     int
	}{
		{name: "guest latest sees public only", want: 2},
		{name: "member latest sees public and own private", userID: &aliceID, want: 3},
		{name: "admin latest sees every message", userID: &admin.ID, isAdmin: true, want: 4},
		{name: "member personal includes own private", userID: &aliceID, authorID: &aliceID, want: 2},
		{name: "member viewing another author only sees public", userID: &aliceID, authorID: &bobID, want: 1},
		{name: "admin author scope sees that author's private messages", userID: &admin.ID, isAdmin: true, authorID: &bobID, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calendar, err := GetMessagesGroupByDate(tt.userID, tt.isAdmin, tt.authorID)
			if err != nil {
				t.Fatalf("get calendar: %v", err)
			}
			counts := messageDateCounts(calendar)
			if got := counts["2026-06-02"]; got != tt.want {
				t.Fatalf("2026-06-02 count = %d, want %d; rows=%#v", got, tt.want, calendar)
			}
		})
	}
}

func messageDateCounts(rows []MessageDateCount) map[string]int {
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.Date] = row.Count
	}
	return counts
}
