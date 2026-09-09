package services

import (
	"encoding/json"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"strconv"
	"strings"
)

// RSS settings own defaults, member selection normalization and the public configuration view.
type RSSConfig struct {
	Enabled          bool
	MemberIDs        []uint
	AvailableMembers []map[string]interface{}
	Title            string
	Description      string
	AuthorName       string
	FaviconURL       string
}

func defaultRSSConfigValues() RSSConfig {
	return RSSConfig{
		Enabled:     false,
		MemberIDs:   []uint{},
		Title:       neutralRSSTitle,
		Description: neutralRSSDescription,
		AuthorName:  neutralOwnerName,
		FaviconURL:  "/favicon-32x32.png",
	}
}

func parseRSSMemberIDString(raw string) ([]uint, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}

	var ids []uint
	if err := json.Unmarshal([]byte(trimmed), &ids); err == nil {
		return ids, true
	}

	return []uint{}, true
}

func parseRSSMemberIDValue(raw interface{}) ([]uint, bool) {
	switch v := raw.(type) {
	case []uint:
		return v, true
	case []int:
		ids := make([]uint, 0, len(v))
		for _, id := range v {
			if id > 0 {
				ids = append(ids, uint(id))
			}
		}
		return ids, true
	case []float64:
		ids := make([]uint, 0, len(v))
		for _, id := range v {
			if id > 0 {
				ids = append(ids, uint(id))
			}
		}
		return ids, true
	case []interface{}:
		ids := make([]uint, 0, len(v))
		for _, item := range v {
			switch id := item.(type) {
			case float64:
				if id > 0 {
					ids = append(ids, uint(id))
				}
			case int:
				if id > 0 {
					ids = append(ids, uint(id))
				}
			case uint:
				if id > 0 {
					ids = append(ids, id)
				}
			case string:
				if parsed, err := strconv.ParseUint(strings.TrimSpace(id), 10, 64); err == nil && parsed > 0 {
					ids = append(ids, uint(parsed))
				}
			}
		}
		return ids, true
	case string:
		return parseRSSMemberIDString(v)
	default:
		return []uint{}, true
	}
}

func normalizeRSSMemberIDs(db *gorm.DB, ids []uint) ([]uint, error) {
	requested := make([]uint, 0, len(ids))
	seen := map[uint]struct{}{}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		requested = append(requested, id)
	}
	if len(requested) == 0 {
		return []uint{}, nil
	}

	var users []models.User
	if err := db.Select("id").Where("id IN ?", requested).Find(&users).Error; err != nil {
		return nil, err
	}
	valid := map[uint]struct{}{}
	for _, user := range users {
		valid[user.ID] = struct{}{}
	}

	normalized := make([]uint, 0, len(requested))
	for _, id := range requested {
		if _, ok := valid[id]; ok {
			normalized = append(normalized, id)
		}
	}
	return normalized, nil
}

func defaultRSSMemberIDs(db *gorm.DB) ([]uint, error) {
	var primary models.User
	if err := db.Select("id").Where("id = ? AND is_admin = ?", models.PrimaryAdminUserID, true).First(&primary).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return []uint{}, nil
		}
		return nil, err
	}
	return []uint{primary.ID}, nil
}

func resolveRSSMemberIDs(db *gorm.DB, raw string) ([]uint, error) {
	ids, explicit := parseRSSMemberIDString(raw)
	if !explicit {
		return defaultRSSMemberIDs(db)
	}
	return normalizeRSSMemberIDs(db, ids)
}

func getRSSAvailableMembers(db *gorm.DB) ([]map[string]interface{}, error) {
	var users []models.User
	if err := db.Select("id, username, is_admin, avatar_url").Order("is_admin DESC, id ASC").Find(&users).Error; err != nil {
		return nil, err
	}

	members := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		members = append(members, map[string]interface{}{
			"id":        user.ID,
			"username":  user.Username,
			"isAdmin":   user.IsAdmin,
			"avatarURL": strings.TrimSpace(user.AvatarURL),
		})
	}
	return members, nil
}

func buildRSSConfig(db *gorm.DB, config models.SiteConfig) (RSSConfig, error) {
	defaults := defaultRSSConfigValues()
	memberIDs, err := resolveRSSMemberIDs(db, config.RSSMemberIDs)
	if err != nil {
		return defaults, err
	}
	availableMembers, err := getRSSAvailableMembers(db)
	if err != nil {
		return defaults, err
	}

	return RSSConfig{
		Enabled:          config.RSSEnabled && len(memberIDs) > 0,
		MemberIDs:        memberIDs,
		AvailableMembers: availableMembers,
		Title:            choose(config.RSSTitle, defaults.Title),
		Description:      choose(config.RSSDescription, defaults.Description),
		AuthorName:       choose(config.RSSAuthorName, defaults.AuthorName),
		FaviconURL:       choose(config.RSSFaviconURL, defaults.FaviconURL),
	}, nil
}

func GetRSSConfig() (RSSConfig, error) {
	db, err := database.GetDB()
	if err != nil {
		return RSSConfig{}, err
	}

	var config models.SiteConfig
	if err := db.Table("site_configs").First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return defaultRSSConfigValues(), nil
		}
		return RSSConfig{}, err
	}

	return buildRSSConfig(db, config)
}
