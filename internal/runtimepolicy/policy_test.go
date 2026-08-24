package runtimepolicy

import (
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestResolveDerivesTheThreeRuntimeStatesFromOneConfiguredMode(t *testing.T) {
	tests := []struct {
		name   string
		config models.SiteConfig
		want   Policy
	}{
		{
			name: "local mode never calls VoceChat",
			config: models.SiteConfig{
				RuntimeMode:                 string(ModeLocal),
				RuntimeModeMigrationVersion: 1,
				VoceChatBaseURL:             "https://vc.example.test",
				VoceChatAdminToken:          "configured-token",
				VoceChatLastHealthStatus:    "ok",
			},
			want: Policy{
				ConfiguredMode:        ModeLocal,
				RuntimeState:          StateLocal,
				AllowPasswordMutation: true,
			},
		},
		{
			name: "healthy VoceChat mode enables integrated behavior",
			config: models.SiteConfig{
				RuntimeMode:                 string(ModeVoceChat),
				RuntimeModeMigrationVersion: 1,
				VoceChatBaseURL:             "https://vc.example.test",
				VoceChatAdminToken:          "configured-token",
				VoceChatBotAPIKey:           "configured-bot-key",
				VoceChatLastHealthStatus:    "ok",
			},
			want: Policy{
				ConfiguredMode:          ModeVoceChat,
				RuntimeState:            StateVoceChatNormal,
				VerifyVoceChatLogin:     true,
				AllowPasswordMutation:   true,
				UseVoceChatRegistration: true,
				UseVoceChatContacts:     true,
				SendVoceChatPush:        true,
			},
		},
		{
			name: "failed VoceChat health enters automatic degradation",
			config: models.SiteConfig{
				RuntimeMode:                 string(ModeVoceChat),
				RuntimeModeMigrationVersion: 1,
				VoceChatBaseURL:             "https://vc.example.test",
				VoceChatAdminToken:          "configured-token",
				VoceChatBotAPIKey:           "configured-bot-key",
				VoceChatLastHealthStatus:    "failed",
			},
			want: Policy{
				ConfiguredMode:          ModeVoceChat,
				RuntimeState:            StateVoceChatDegraded,
				AllowLocalFallbackLogin: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Resolve(test.config); got != test.want {
				t.Fatalf("Resolve() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestEffectiveConfiguredModeMapsOnlyUnmigratedLegacyConfiguration(t *testing.T) {
	legacy := models.SiteConfig{VoceChatEnabled: true}
	if got := EffectiveConfiguredMode(legacy); got != ModeVoceChat {
		t.Fatalf("legacy enabled mode = %q, want %q", got, ModeVoceChat)
	}

	migrated := models.SiteConfig{
		RuntimeMode:                 string(ModeLocal),
		RuntimeModeMigrationVersion: 1,
		VoceChatEnabled:             true,
	}
	if got := EffectiveConfiguredMode(migrated); got != ModeLocal {
		t.Fatalf("migrated mode = %q, want %q", got, ModeLocal)
	}
}
