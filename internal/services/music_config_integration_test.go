package services

import (
	"errors"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/music"
	"gorm.io/gorm"
)

func TestMixedFrontendMusicWritePreservesSiteFieldsAndOneRevision(t *testing.T) {
	db := setupUserServiceTestDB(t)
	admin := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "music-config-admin", IsAdmin: true})
	if err := db.Create(&models.SiteConfig{SiteTitle: "before", MusicPlaylistId: "123", MusicSongId: "456", MusicCssCdnURL: "https://cdn.example/music.css", MusicJsCdnURL: "https://cdn.example/music.js"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := UpdateFrontendSetting(admin.ID, map[string]interface{}{"frontendSettings": map[string]interface{}{"siteTitle": "after", "musicSource": "local", "musicEnabled": true}}); err != nil {
		t.Fatal(err)
	}
	var site models.SiteConfig
	var config models.MusicConfig
	if err := db.First(&site, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&config, 1).Error; err != nil {
		t.Fatal(err)
	}
	if site.SiteTitle != "after" || !site.MusicEnabled || site.MusicPlaylistId != "123" || site.MusicSongId != "456" {
		t.Fatalf("mixed write lost fields: %#v", site)
	}
	if config.Source != "local" || config.Version != 2 {
		t.Fatalf("music revision = %#v", config)
	}
	public, err := GetFrontendConfig()
	if err != nil {
		t.Fatal(err)
	}
	frontend := public["frontendSettings"].(map[string]interface{})
	if frontend["musicSource"] != "local" {
		t.Fatalf("public source = %v", frontend["musicSource"])
	}
	for _, key := range []string{"musicPlaylistId", "musicSongId", "musicCssCdnURL", "musicJsCdnURL"} {
		if _, exists := frontend[key]; exists {
			t.Errorf("local public config exposes %s", key)
		}
	}
	if err := UpdateFrontendSetting(admin.ID, map[string]interface{}{"frontendSettings": map[string]interface{}{"siteTitle": "generic"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&config, 1).Error; err != nil {
		t.Fatal(err)
	}
	if config.Version != 2 {
		t.Fatalf("generic write incremented revision: %d", config.Version)
	}
	if err := UpdateFrontendSetting(admin.ID, map[string]interface{}{"frontendSettings": map[string]interface{}{"musicSource": "netease"}}); err != nil {
		t.Fatal(err)
	}
	public, err = GetFrontendConfig()
	if err != nil {
		t.Fatal(err)
	}
	frontend = public["frontendSettings"].(map[string]interface{})
	if frontend["musicPlaylistId"] != "123" || frontend["musicJsCdnURL"] != "https://cdn.example/music.js" {
		t.Fatalf("switch back lost online settings: %#v", frontend)
	}
}

func TestMusicFrontendTransactionRejectsStaleVersionAndRollsBack(t *testing.T) {
	db := setupUserServiceTestDB(t)
	if err := db.Create(&models.SiteConfig{MusicTheme: "auto"}).Error; err != nil {
		t.Fatal(err)
	}
	version := uint64(1)
	err := db.Transaction(func(tx *gorm.DB) error {
		var site models.SiteConfig
		if _, err := music.UpdateFrontendSettingsTx(tx, &site, map[string]interface{}{"musicTheme": "dark"}, &version); err != nil {
			return err
		}
		_, err := music.UpdateFrontendSettingsTx(tx, &site, map[string]interface{}{"musicTheme": "light"}, &version)
		return err
	})
	if !errors.Is(err, music.ErrConflict) {
		t.Fatalf("stale version error = %v", err)
	}
	var config models.MusicConfig
	var site models.SiteConfig
	if err := db.First(&config, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&site, 1).Error; err != nil {
		t.Fatal(err)
	}
	if config.Version != 1 || site.MusicTheme != "auto" {
		t.Fatalf("failed transaction persisted: version=%d theme=%s", config.Version, site.MusicTheme)
	}
}
