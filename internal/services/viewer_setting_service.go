package services

import (
	"fmt"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

// Viewer settings resolve life-countdown and widget values for one authenticated or guest viewer.
func normalizeLifeCountdownBirthDate(value interface{}) (string, error) {
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	if text == "" || text == "<nil>" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", text); err != nil {
		return "", fmt.Errorf("生日格式无效，请使用 YYYY-MM-DD")
	}
	return text, nil
}

func normalizeLifeExpectancyYears(value interface{}) (int, error) {
	var years int
	switch v := value.(type) {
	case int:
		years = v
	case int64:
		years = int(v)
	case float64:
		years = int(v)
		if float64(years) != v {
			return 0, fmt.Errorf("预期寿命必须是整数")
		}
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(text)
		if err != nil {
			return 0, fmt.Errorf("预期寿命必须是整数")
		}
		years = parsed
	default:
		return 0, fmt.Errorf("预期寿命格式无效")
	}
	if years != 0 && (years < 1 || years > 150) {
		return 0, fmt.Errorf("预期寿命必须在 1-150 年之间")
	}
	return years, nil
}

func resolveLifeCountdownSettings(db *gorm.DB, viewerUserID uint, siteConfig models.SiteConfig) lifeCountdownSettings {
	if viewerUserID == 0 {
		return lifeCountdownSettings{
			Enabled:             siteConfig.LifeCountdownEnabled,
			BirthDate:           strings.TrimSpace(siteConfig.LifeCountdownBirthDate),
			LifeExpectancyYears: siteConfig.LifeExpectancyYears,
		}
	}
	if viewerUserID != 0 {
		var config models.UserLifeCountdownConfig
		if err := db.Where("user_id = ?", viewerUserID).First(&config).Error; err == nil {
			return lifeCountdownSettings{
				Enabled:             config.Enabled,
				BirthDate:           strings.TrimSpace(config.BirthDate),
				LifeExpectancyYears: config.LifeExpectancyYears,
			}
		}
	}

	return lifeCountdownSettings{
		Enabled:             siteConfig.LifeCountdownEnabled,
		BirthDate:           strings.TrimSpace(siteConfig.LifeCountdownBirthDate),
		LifeExpectancyYears: siteConfig.LifeExpectancyYears,
	}
}

func resolveWidgetVisibilitySettings(db *gorm.DB, viewerUserID uint, siteConfig models.SiteConfig) WidgetVisibilitySettings {
	if viewerUserID == 0 {
		return guestWidgetVisibilitySettings(siteConfig)
	}
	resolved := guestWidgetVisibilitySettings(siteConfig)
	var preference models.UserFrontendPreference
	if err := db.Where("user_id = ?", viewerUserID).First(&preference).Error; err != nil {
		return resolved
	}
	if preference.HitokotoEnabled != nil {
		resolved.HitokotoEnabled = *preference.HitokotoEnabled
	}
	if preference.HomeStatsEnabled != nil {
		resolved.HomeStatsEnabled = *preference.HomeStatsEnabled
	}
	if preference.PopularTagsEnabled != nil {
		resolved.PopularTagsEnabled = *preference.PopularTagsEnabled
	}
	if preference.CalendarEnabled != nil {
		resolved.CalendarEnabled = *preference.CalendarEnabled
	}
	if preference.LatestGalleryEnabled != nil {
		resolved.LatestGalleryEnabled = *preference.LatestGalleryEnabled
	}
	if preference.HeatmapEnabled != nil {
		resolved.HeatmapEnabled = *preference.HeatmapEnabled
	}
	return resolved
}

// GetFrontendConfig 获取前端配置
