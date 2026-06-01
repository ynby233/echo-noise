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
