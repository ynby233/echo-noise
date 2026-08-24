package runtimepolicy

import (
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
)

type ConfiguredMode string

const (
	ModeLocal    ConfiguredMode = models.RuntimeModeLocal
	ModeVoceChat ConfiguredMode = models.RuntimeModeVoceChat

	CurrentMigrationVersion = models.RuntimeModeMigrationVersionCurrent
)

type RuntimeState string

const (
	StateLocal            RuntimeState = "local"
	StateVoceChatNormal   RuntimeState = "vocechat_normal"
	StateVoceChatDegraded RuntimeState = "vocechat_degraded"
)

type Policy struct {
	ConfiguredMode          ConfiguredMode `json:"configured_mode"`
	RuntimeState            RuntimeState   `json:"runtime_state"`
	VerifyVoceChatLogin     bool           `json:"verify_vocechat_login"`
	AllowLocalFallbackLogin bool           `json:"allow_local_fallback_login"`
	AllowPasswordMutation   bool           `json:"allow_password_mutation"`
	UseVoceChatRegistration bool           `json:"use_vocechat_registration"`
	UseVoceChatContacts     bool           `json:"use_vocechat_contacts"`
	SendVoceChatPush        bool           `json:"send_vocechat_push"`
	LegacyConfiguration     bool           `json:"-"`
}

func ParseConfiguredMode(value string) (ConfiguredMode, bool) {
	switch ConfiguredMode(strings.ToLower(strings.TrimSpace(value))) {
	case ModeLocal:
		return ModeLocal, true
	case ModeVoceChat:
		return ModeVoceChat, true
	default:
		return "", false
	}
}

func EffectiveConfiguredMode(config models.SiteConfig) ConfiguredMode {
	if config.RuntimeModeMigrationVersion < CurrentMigrationVersion {
		if config.VoceChatEnabled {
			return ModeVoceChat
		}
		return ModeLocal
	}
	if mode, ok := ParseConfiguredMode(config.RuntimeMode); ok {
		return mode
	}
	return ModeLocal
}

func ConnectionConfigured(config models.SiteConfig) bool {
	baseURL := strings.TrimSpace(config.VoceChatBaseURL)
	token := strings.TrimSpace(config.VoceChatAdminToken)
	username := strings.TrimSpace(config.VoceChatAdminUsername)
	password := strings.TrimSpace(config.VoceChatAdminPassword)
	return baseURL != "" && (token != "" || (username != "" && password != ""))
}

func Resolve(config models.SiteConfig) Policy {
	mode := EffectiveConfiguredMode(config)
	legacyConfiguration := config.RuntimeModeMigrationVersion < CurrentMigrationVersion
	if mode == ModeLocal {
		policy := Policy{
			ConfiguredMode:        ModeLocal,
			RuntimeState:          StateLocal,
			AllowPasswordMutation: true,
			LegacyConfiguration:   legacyConfiguration,
		}
		if legacyConfiguration {
			policy.AllowLocalFallbackLogin = config.VoceChatLocalFallbackEnabled
		}
		return policy
	}

	healthStatus := strings.TrimSpace(config.VoceChatLastHealthStatus)
	healthy := strings.EqualFold(healthStatus, "ok") || (legacyConfiguration && !strings.EqualFold(healthStatus, "failed"))
	connectionReady := ConnectionConfigured(config) || legacyConfiguration
	if !connectionReady || !healthy {
		return Policy{
			ConfiguredMode:          ModeVoceChat,
			RuntimeState:            StateVoceChatDegraded,
			AllowLocalFallbackLogin: true,
			LegacyConfiguration:     legacyConfiguration,
		}
	}

	policy := Policy{
		ConfiguredMode:          ModeVoceChat,
		RuntimeState:            StateVoceChatNormal,
		VerifyVoceChatLogin:     true,
		AllowPasswordMutation:   true,
		UseVoceChatRegistration: true,
		UseVoceChatContacts:     true,
		SendVoceChatPush:        strings.TrimSpace(config.VoceChatBotAPIKey) != "",
		LegacyConfiguration:     legacyConfiguration,
	}
	if legacyConfiguration {
		policy.VerifyVoceChatLogin = config.VoceChatLoginVerificationEnabled
		policy.AllowLocalFallbackLogin = config.VoceChatLocalFallbackEnabled
		policy.UseVoceChatContacts = config.VoceChatContactsEnabled
		policy.SendVoceChatPush = config.VoceChatNotificationEnabled && strings.TrimSpace(config.VoceChatBotAPIKey) != ""
	}
	return policy
}
