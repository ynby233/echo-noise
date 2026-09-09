package services

import (
	"fmt"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

// Widget settings own the per-user and guest visibility value objects, allowed keys and persistence.
type lifeCountdownSettings struct {
	Enabled             bool
	BirthDate           string
	LifeExpectancyYears int
}

// WidgetVisibilitySettings is the single seven-field visibility contract used
// for both an authenticated viewer and the anonymous guest default. Countdown
// details remain in their existing dedicated records, while this value object
// keeps every caller on the same visibility vocabulary.
type WidgetVisibilitySettings struct {
	LifeCountdownEnabled bool `json:"lifeCountdownEnabled"`
	HitokotoEnabled      bool `json:"hitokotoEnabled"`
	HomeStatsEnabled     bool `json:"homeStatsEnabled"`
	PopularTagsEnabled   bool `json:"popularTagsEnabled"`
	CalendarEnabled      bool `json:"calendarEnabled"`
	LatestGalleryEnabled bool `json:"latestGalleryEnabled"`
	HeatmapEnabled       bool `json:"heatmapEnabled"`
}

func DefaultAccountWidgetVisibilitySettings() WidgetVisibilitySettings {
	return WidgetVisibilitySettings{
		LifeCountdownEnabled: false,
		HitokotoEnabled:      true,
		HomeStatsEnabled:     true,
		PopularTagsEnabled:   true,
		CalendarEnabled:      true,
		LatestGalleryEnabled: true,
		HeatmapEnabled:       true,
	}
}

func guestWidgetVisibilitySettings(config models.SiteConfig) WidgetVisibilitySettings {
	return WidgetVisibilitySettings{
		LifeCountdownEnabled: config.LifeCountdownEnabled,
		HitokotoEnabled:      config.HitokotoEnabled,
		HomeStatsEnabled:     config.HomeStatsEnabled,
		PopularTagsEnabled:   config.PopularTagsEnabled,
		CalendarEnabled:      config.CalendarEnabled,
		LatestGalleryEnabled: config.LatestGalleryEnabled,
		HeatmapEnabled:       config.HeatmapEnabled,
	}
}

var lifeCountdownSettingKeys = map[string]struct{}{
	"lifeCountdownEnabled":   {},
	"lifeCountdownBirthDate": {},
	"lifeExpectancyYears":    {},
}

var userFrontendPreferenceSettingKeys = map[string]struct{}{
	"hitokotoEnabled":      {},
	"homeStatsEnabled":     {},
	"popularTagsEnabled":   {},
	"calendarEnabled":      {},
	"latestGalleryEnabled": {},
	"heatmapEnabled":       {},
}

func HasLifeCountdownSettings(frontendSettings map[string]interface{}) bool {
	for key := range lifeCountdownSettingKeys {
		if _, ok := frontendSettings[key]; ok {
			return true
		}
	}
	return false
}

var musicSettingKeys = map[string]struct{}{
	"musicEnabled": {}, "musicPlaylistId": {}, "musicSongId": {}, "musicPosition": {}, "musicTheme": {},
	"musicLyric": {}, "musicAutoplay": {}, "musicDefaultMinimized": {}, "musicEmbed": {}, "musicHideOnMobile": {},
	"musicCssCdnURL": {}, "musicJsCdnURL": {},
}

func HasMusicSettings(frontendSettings map[string]interface{}) bool {
	for key := range frontendSettings {
		if _, ok := musicSettingKeys[key]; ok {
			return true
		}
	}
	return false
}

// IsMusicSettingsOnly accepts a non-empty subset of the music configuration.
// The dedicated music route must never carry unrelated site settings.
func IsMusicSettingsOnly(frontendSettings map[string]interface{}) bool {
	if len(frontendSettings) == 0 {
		return false
	}
	for key := range frontendSettings {
		if _, ok := musicSettingKeys[key]; !ok {
			return false
		}
	}
	return true
}

func IsLifeCountdownSettingsOnly(frontendSettings map[string]interface{}) bool {
	if len(frontendSettings) == 0 {
		return false
	}
	for key := range frontendSettings {
		if _, ok := lifeCountdownSettingKeys[key]; !ok {
			return false
		}
	}
	return true
}

func HasUserFrontendPreferenceSettings(frontendSettings map[string]interface{}) bool {
	for key := range userFrontendPreferenceSettingKeys {
		if _, ok := frontendSettings[key]; ok {
			return true
		}
	}
	return false
}

func IsUserFrontendSettingsOnly(frontendSettings map[string]interface{}) bool {
	if len(frontendSettings) == 0 {
		return false
	}
	for key := range frontendSettings {
		if _, ok := lifeCountdownSettingKeys[key]; ok {
			continue
		}
		if _, ok := userFrontendPreferenceSettingKeys[key]; !ok {
			return false
		}
	}
	return true
}

// IsGuestWidgetSettingsOnly accepts exactly the seven widget fields and the
// guest countdown details. It is deliberately separate from the much broader
// site-settings payload so a primary-admin guest-default save cannot carry
// unrelated site configuration by accident.
func IsGuestWidgetSettingsOnly(frontendSettings map[string]interface{}) bool {
	if len(frontendSettings) == 0 {
		return false
	}
	for key := range frontendSettings {
		if _, ok := lifeCountdownSettingKeys[key]; ok {
			continue
		}
		if _, ok := userFrontendPreferenceSettingKeys[key]; !ok {
			return false
		}
	}
	return true
}

func StripLifeCountdownSettings(frontendSettings map[string]interface{}) map[string]interface{} {
	stripped := make(map[string]interface{}, len(frontendSettings))
	for key, value := range frontendSettings {
		if _, ok := lifeCountdownSettingKeys[key]; ok {
			continue
		}
		stripped[key] = value
	}
	return stripped
}

func UpdateUserLifeCountdownConfig(userID uint, frontendSettings map[string]interface{}) error {
	if userID == 0 || !HasLifeCountdownSettings(frontendSettings) {
		return nil
	}

	db, err := database.GetDB()
	if err != nil {
		return err
	}

	var config models.UserLifeCountdownConfig
	err = db.Where("user_id = ?", userID).First(&config).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return err
		}
		config = models.UserLifeCountdownConfig{UserID: userID}
	}

	if v, ok := frontendSettings["lifeCountdownEnabled"]; ok {
		if parsed, ok := parseBoolSetting(v); ok {
			config.Enabled = parsed
		}
	}
	if v, ok := frontendSettings["lifeCountdownBirthDate"]; ok {
		birthDate, err := normalizeLifeCountdownBirthDate(v)
		if err != nil {
			return err
		}
		config.BirthDate = birthDate
	}
	if v, ok := frontendSettings["lifeExpectancyYears"]; ok {
		years, err := normalizeLifeExpectancyYears(v)
		if err != nil {
			return err
		}
		config.LifeExpectancyYears = years
	}

	if config.ID == 0 {
		return db.Create(&config).Error
	}
	return db.Save(&config).Error
}

func UpdateUserFrontendPreferenceConfig(userID uint, frontendSettings map[string]interface{}) error {
	if userID == 0 || !HasUserFrontendPreferenceSettings(frontendSettings) {
		return nil
	}

	db, err := database.GetDB()
	if err != nil {
		return err
	}

	var preference models.UserFrontendPreference
	err = db.Where("user_id = ?", userID).First(&preference).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return err
		}
		preference = models.UserFrontendPreference{UserID: userID}
	}
	updates := []struct {
		key   string
		label string
		set   func(*bool)
	}{
		{"hitokotoEnabled", "每日一言", func(value *bool) { preference.HitokotoEnabled = value }},
		{"homeStatsEnabled", "数据统计", func(value *bool) { preference.HomeStatsEnabled = value }},
		{"popularTagsEnabled", "热门标签", func(value *bool) { preference.PopularTagsEnabled = value }},
		{"calendarEnabled", "日历", func(value *bool) { preference.CalendarEnabled = value }},
		{"latestGalleryEnabled", "最新图集", func(value *bool) { preference.LatestGalleryEnabled = value }},
		{"heatmapEnabled", "热力图", func(value *bool) { preference.HeatmapEnabled = value }},
	}
	for _, update := range updates {
		raw, exists := frontendSettings[update.key]
		if !exists {
			continue
		}
		value, ok := parseBoolSetting(raw)
		if !ok {
			return fmt.Errorf("%s开关格式无效", update.label)
		}
		update.set(&value)
	}
	if preference.ID == 0 {
		return db.Create(&preference).Error
	}
	return db.Save(&preference).Error
}

// UpdateUserWidgetPreferences writes only the authenticated user's seven
// widgets. The caller never supplies a target user ID, which prevents cross-
// account preference updates at the controller seam.
func UpdateUserWidgetPreferences(userID uint, frontendSettings map[string]interface{}) error {
	if userID == 0 || !IsUserFrontendSettingsOnly(frontendSettings) {
		return fmt.Errorf("小组件设置包含不允许的字段")
	}
	if err := UpdateUserLifeCountdownConfig(userID, frontendSettings); err != nil {
		return err
	}
	return UpdateUserFrontendPreferenceConfig(userID, frontendSettings)
}

func UpdateGuestWidgetPreferences(frontendSettings map[string]interface{}) error {
	if !IsGuestWidgetSettingsOnly(frontendSettings) {
		return fmt.Errorf("访客默认小组件设置包含不允许的字段")
	}
	return UpdateFrontendSetting(0, map[string]interface{}{"frontendSettings": frontendSettings})
}
