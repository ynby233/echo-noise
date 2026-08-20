package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateDBRemovesRetiredThirdPartyAuthenticationAndCommentColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Exec(`CREATE TABLE site_configs (
		id integer primary key,
		github_o_auth_enabled numeric,
		github_client_id text,
		github_client_secret text,
		github_callback_url text,
		comment_system text
	)`).Error; err != nil {
		t.Fatalf("create legacy site config: %v", err)
	}

	if err := MigrateDB(db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	for _, column := range []string{
		"github_o_auth_enabled",
		"github_client_id",
		"github_client_secret",
		"github_callback_url",
		"comment_system",
	} {
		if db.Migrator().HasColumn("site_configs", column) {
			t.Fatalf("retired column %q still exists", column)
		}
	}
}

func TestSiteConfigBackgroundsConfigSupportsLegacyStrings(t *testing.T) {
	config := SiteConfig{Backgrounds: `["https://example.com/a.jpg","https://example.com/b.jpg"]`}

	backgrounds := config.GetBackgroundsConfig()
	if len(backgrounds) != 2 {
		t.Fatalf("expected 2 backgrounds, got %d", len(backgrounds))
	}
	if backgrounds[0].URL != "https://example.com/a.jpg" || backgrounds[1].URL != "https://example.com/b.jpg" {
		t.Fatalf("unexpected backgrounds: %#v", backgrounds)
	}
}

func TestSiteConfigBackgroundsConfigSupportsPerImageTextStyle(t *testing.T) {
	config := SiteConfig{Backgrounds: `[{"url":"https://example.com/a.jpg","titleColor":"#111111","titleOpacity":0.45,"subtitleColor":"#eeeeee","subtitleOpacity":0.75}]`}

	backgrounds := config.GetBackgroundsConfig()
	if len(backgrounds) != 1 {
		t.Fatalf("expected 1 background, got %d", len(backgrounds))
	}
	bg := backgrounds[0]
	if bg.URL != "https://example.com/a.jpg" {
		t.Fatalf("unexpected url: %q", bg.URL)
	}
	if bg.TitleColor != "#111111" || bg.TitleOpacity != 0.45 || bg.SubtitleColor != "#eeeeee" || bg.SubtitleOpacity != 0.75 {
		t.Fatalf("unexpected text style: %#v", bg)
	}
}

func TestSiteConfigBackgroundsConfigDefaultsMissingOpacity(t *testing.T) {
	config := SiteConfig{Backgrounds: `[{"url":"https://example.com/a.jpg","titleColor":"#111111","subtitleColor":"#eeeeee"}]`}

	backgrounds := config.GetBackgroundsConfig()
	if len(backgrounds) != 1 {
		t.Fatalf("expected 1 background, got %d", len(backgrounds))
	}
	if backgrounds[0].TitleOpacity != 1 || backgrounds[0].SubtitleOpacity != 1 {
		t.Fatalf("expected default opacity 1, got %#v", backgrounds[0])
	}
}

func TestSiteConfigBackgroundsConfigPreservesZeroOpacity(t *testing.T) {
	config := SiteConfig{Backgrounds: `[{"url":"https://example.com/a.jpg","titleOpacity":0,"subtitleOpacity":0}]`}

	backgrounds := config.GetBackgroundsConfig()
	if len(backgrounds) != 1 {
		t.Fatalf("expected 1 background, got %d", len(backgrounds))
	}
	if backgrounds[0].TitleOpacity != 0 || backgrounds[0].SubtitleOpacity != 0 {
		t.Fatalf("expected zero opacity to be preserved, got %#v", backgrounds[0])
	}
}
