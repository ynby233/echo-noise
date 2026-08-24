package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/runtimepolicy"
)

func TestSwitchConfiguredModeIsPrimaryOnlyPersistentAndIdempotent(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{Username: "primary-runtime", Password: models.HashPassword("primary"), IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "delegated-runtime", Password: models.HashPassword("delegated"), IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "ordinary-runtime", Password: models.HashPassword("ordinary")})
	config := models.SiteConfig{
		RuntimeMode:                 models.RuntimeModeLocal,
		RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent,
		VoceChatBaseURL:             "https://vc.example.test",
		VoceChatAdminToken:          "configured-token",
		VoceChatBotAPIKey:           "configured-bot-key",
	}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}

	healthCalls := 0
	previousHealthCheck := runtimeModeHealthCheck
	runtimeModeHealthCheck = func(context.Context) error {
		healthCalls++
		return db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(map[string]interface{}{
			"voce_chat_last_health_status": "ok",
			"voce_chat_last_health_error":  "",
		}).Error
	}
	t.Cleanup(func() { runtimeModeHealthCheck = previousHealthCheck })

	if _, err := SwitchConfiguredMode(context.Background(), delegated.ID, runtimepolicy.ModeVoceChat); !errors.Is(err, ErrRuntimeModePrimaryAdminRequired) {
		t.Fatalf("delegated switch error = %v, want primary-only error", err)
	}
	policy, err := SwitchConfiguredMode(context.Background(), primary.ID, runtimepolicy.ModeVoceChat)
	if err != nil {
		t.Fatalf("switch to VoceChat: %v", err)
	}
	if policy.RuntimeState != runtimepolicy.StateVoceChatNormal || !policy.VerifyVoceChatLogin || !policy.UseVoceChatContacts || !policy.SendVoceChatPush {
		t.Fatalf("VoceChat policy = %#v", policy)
	}
	if healthCalls != 1 {
		t.Fatalf("health calls = %d, want 1", healthCalls)
	}
	var stored models.SiteConfig
	if err := db.First(&stored, config.ID).Error; err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if stored.RuntimeMode != models.RuntimeModeVoceChat || stored.RuntimeModeMigrationVersion != models.RuntimeModeMigrationVersionCurrent {
		t.Fatalf("stored runtime mode = %q version=%d", stored.RuntimeMode, stored.RuntimeModeMigrationVersion)
	}
	var queued models.User
	if err := db.First(&queued, ordinary.ID).Error; err != nil {
		t.Fatalf("reload ordinary user: %v", err)
	}
	if queued.VoceChatSyncStatus != models.VoceChatSyncStatusPending {
		t.Fatalf("ordinary user state = %q, want pending", queued.VoceChatSyncStatus)
	}

	policy, err = SwitchConfiguredMode(context.Background(), primary.ID, runtimepolicy.ModeVoceChat)
	if err != nil {
		t.Fatalf("repeat VoceChat switch: %v", err)
	}
	if policy.RuntimeState != runtimepolicy.StateVoceChatNormal || healthCalls != 1 {
		t.Fatalf("idempotent switch policy=%#v healthCalls=%d", policy, healthCalls)
	}
	var switchAudits int64
	if err := db.Model(&models.AdminAuditLog{}).Where("module = ? AND action = ?", "runtime_mode", "switch").Count(&switchAudits).Error; err != nil {
		t.Fatalf("count mode audits: %v", err)
	}
	if switchAudits != 1 {
		t.Fatalf("switch audit count = %d, want 1", switchAudits)
	}
	var audit models.AdminAuditLog
	if err := db.Where("module = ? AND action = ?", "runtime_mode", "switch").First(&audit).Error; err != nil {
		t.Fatalf("load mode audit: %v", err)
	}
	for _, secret := range []string{config.VoceChatAdminToken, config.VoceChatBotAPIKey} {
		if strings.Contains(audit.Summary, secret) {
			t.Fatalf("audit summary leaked a credential: %q", audit.Summary)
		}
	}
}

func TestSwitchConfiguredModeKeepsLocalModeWhenHealthCheckFails(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{Username: "primary-runtime-failure", Password: models.HashPassword("primary"), IsAdmin: true})
	config := models.SiteConfig{
		RuntimeMode:                 models.RuntimeModeLocal,
		RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent,
		VoceChatBaseURL:             "https://vc.example.test",
		VoceChatAdminToken:          "configured-token",
	}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}
	previousHealthCheck := runtimeModeHealthCheck
	runtimeModeHealthCheck = func(context.Context) error { return errors.New("upstream unavailable with secret detail") }
	t.Cleanup(func() { runtimeModeHealthCheck = previousHealthCheck })

	if _, err := SwitchConfiguredMode(context.Background(), primary.ID, runtimepolicy.ModeVoceChat); !errors.Is(err, ErrRuntimeModeHealthCheckFailed) {
		t.Fatalf("switch error = %v, want health-check error", err)
	}
	var stored models.SiteConfig
	if err := db.First(&stored, config.ID).Error; err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if stored.RuntimeMode != models.RuntimeModeLocal {
		t.Fatalf("failed switch changed mode to %q", stored.RuntimeMode)
	}
}
