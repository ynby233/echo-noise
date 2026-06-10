package vocechat

import (
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
)

const (
	DefaultEmailDomain             = "vc.com"
	DefaultContactsCacheTTLSeconds = 60
)

type Config struct {
	Enabled                  bool
	BaseURL                  string
	AdminUsername            string
	AdminPassword            string
	AdminToken               string
	ThirdPartySecret         string
	NotificationEnabled      bool
	BotAPIKey                string
	EmailDomain              string
	LoginVerificationEnabled bool
	LocalFallbackEnabled     bool
	ContactsEnabled          bool
	ContactsCacheTTLSeconds  int
}

func NormalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func NormalizeAPIBaseURL(baseURL string) string {
	baseURL = NormalizeBaseURL(baseURL)
	if baseURL == "" || strings.HasSuffix(strings.ToLower(baseURL), "/api") {
		return baseURL
	}
	return baseURL + "/api"
}

func NormalizeEmailDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimPrefix(domain, "@")
	if domain == "" {
		return DefaultEmailDomain
	}
	return domain
}

func FromSiteConfig(config models.SiteConfig) Config {
	ttl := config.VoceChatContactsCacheTTLSeconds
	if ttl <= 0 {
		ttl = DefaultContactsCacheTTLSeconds
	}

	return Config{
		Enabled:                  config.VoceChatEnabled,
		BaseURL:                  NormalizeBaseURL(config.VoceChatBaseURL),
		AdminUsername:            strings.TrimSpace(config.VoceChatAdminUsername),
		AdminPassword:            strings.TrimSpace(config.VoceChatAdminPassword),
		AdminToken:               strings.TrimSpace(config.VoceChatAdminToken),
		ThirdPartySecret:         strings.TrimSpace(config.VoceChatThirdPartySecret),
		NotificationEnabled:      config.VoceChatNotificationEnabled,
		BotAPIKey:                strings.TrimSpace(config.VoceChatBotAPIKey),
		EmailDomain:              NormalizeEmailDomain(config.VoceChatEmailDomain),
		LoginVerificationEnabled: config.VoceChatLoginVerificationEnabled,
		LocalFallbackEnabled:     config.VoceChatLocalFallbackEnabled,
		ContactsEnabled:          config.VoceChatContactsEnabled,
		ContactsCacheTTLSeconds:  ttl,
	}
}

func (c Config) HasAdminCredential() bool {
	return c.AdminToken != "" || (c.AdminUsername != "" && c.AdminPassword != "")
}

func (c Config) IsReady() bool {
	return c.Enabled && c.BaseURL != "" && c.HasAdminCredential()
}

func (c Config) IsNotificationReady() bool {
	return c.Enabled && c.NotificationEnabled && c.BaseURL != "" && strings.TrimSpace(c.BotAPIKey) != ""
}

func PublicConfigFromSiteConfig(config models.SiteConfig) map[string]interface{} {
	cfg := FromSiteConfig(config)
	return map[string]interface{}{
		"enabled":                    cfg.Enabled,
		"configured":                 cfg.IsReady(),
		"baseURLConfigured":          cfg.BaseURL != "",
		"adminCredentialConfigured":  cfg.HasAdminCredential(),
		"thirdPartySecretConfigured": cfg.ThirdPartySecret != "",
		"notificationEnabled":        cfg.NotificationEnabled,
		"botApiKeyConfigured":        cfg.BotAPIKey != "",
		"emailDomain":                cfg.EmailDomain,
		"loginVerificationEnabled":   cfg.LoginVerificationEnabled,
		"localFallbackEnabled":       cfg.LocalFallbackEnabled,
		"contactsEnabled":            cfg.ContactsEnabled,
		"contactsCacheTTLSeconds":    cfg.ContactsCacheTTLSeconds,
		"lastHealthStatus":           strings.TrimSpace(config.VoceChatLastHealthStatus),
		"lastHealthError":            strings.TrimSpace(config.VoceChatLastHealthError),
		"lastHealthCheckAt": func() string {
			if config.VoceChatLastHealthCheckAt == nil {
				return ""
			}
			return config.VoceChatLastHealthCheckAt.Format("2006-01-02T15:04:05Z07:00")
		}(),
	}
}

func DefaultPublicConfig() map[string]interface{} {
	return PublicConfigFromSiteConfig(models.SiteConfig{})
}
