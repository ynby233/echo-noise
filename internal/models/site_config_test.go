package models

import "testing"

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
