package music

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var musicColumns = map[string]string{
	"musicEnabled": "music_enabled", "musicPlaylistId": "music_playlist_id", "musicSongId": "music_song_id",
	"musicPosition": "music_position", "musicTheme": "music_theme", "musicLyric": "music_lyric",
	"musicAutoplay": "music_autoplay", "musicDefaultMinimized": "music_default_minimized", "musicEmbed": "music_embed",
	"musicHideOnMobile": "music_hide_on_mobile", "musicCssCdnURL": "music_css_cdn_url", "musicJsCdnURL": "music_js_cdn_url",
}

// EnsureDefaults uses the caller's database/transaction and never overwrites existing rows.
func EnsureDefaults(tx *gorm.DB) error {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.MusicConfig{ID: 1, Source: "netease", ScanIntervalMinutes: 60, Version: 1}).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.MusicScanState{ID: 1, State: "idle"}).Error
}

func defaultSite() models.SiteConfig {
	return models.SiteConfig{Model: gorm.Model{ID: 1}, MusicPosition: "bottom-left", MusicTheme: "auto", MusicLyric: true, MusicDefaultMinimized: true, MusicHideOnMobile: true}
}

func loadSite(tx *gorm.DB, site *models.SiteConfig) error {
	if err := tx.First(site, 1).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	seed := defaultSite()
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return err
	}
	return tx.First(site, 1).Error
}

func validateSettings(fields map[string]interface{}) (map[string]interface{}, string, error) {
	columns := make(map[string]interface{})
	source := ""
	for key, value := range fields {
		if key == "musicSource" {
			v, ok := value.(string)
			if !ok || (v != "netease" && v != "local") {
				return nil, "", ErrInvalid
			}
			source = v
			continue
		}
		column, ok := musicColumns[key]
		if !ok {
			return nil, "", ErrInvalid
		}
		switch key {
		case "musicEnabled", "musicLyric", "musicAutoplay", "musicDefaultMinimized", "musicEmbed", "musicHideOnMobile":
			var b bool
			switch v := value.(type) {
			case bool:
				b = v
			case string:
				if v != "true" && v != "false" {
					return nil, "", ErrInvalid
				}
				b = v == "true"
			default:
				return nil, "", ErrInvalid
			}
			columns[column] = b
		default:
			v, ok := value.(string)
			if !ok || strings.ContainsRune(v, 0) {
				return nil, "", ErrInvalid
			}
			switch key {
			case "musicPosition":
				if !oneOf(v, "static", "top-left", "top-right", "bottom-left", "bottom-right") {
					return nil, "", ErrInvalid
				}
			case "musicTheme":
				if !oneOf(v, "auto", "light", "dark") {
					return nil, "", ErrInvalid
				}
			case "musicPlaylistId", "musicSongId":
				if len(v) > 50 {
					return nil, "", ErrInvalid
				}
				for _, c := range v {
					if c < '0' || c > '9' {
						return nil, "", ErrInvalid
					}
				}
			case "musicCssCdnURL", "musicJsCdnURL":
				if len(v) > 255 {
					return nil, "", ErrInvalid
				}
				if v != "" {
					u, err := url.Parse(v)
					if err != nil || !oneOf(u.Scheme, "http", "https") || u.Host == "" || u.User != nil {
						return nil, "", ErrInvalid
					}
				}
			}
			columns[column] = v
		}
	}
	return columns, source, nil
}

func oneOf(value string, choices ...string) bool {
	for _, v := range choices {
		if v == value {
			return true
		}
	}
	return false
}

// UpdateFrontendSettingsTx is the only configuration Version increment. The caller
// must commit or roll back tx, and omit all music columns from any subsequent Save.
func UpdateFrontendSettingsTx(tx *gorm.DB, site *models.SiteConfig, fields map[string]interface{}, expectedVersion *uint64) (uint64, error) {
	if tx == nil || site == nil {
		return 0, ErrInvalid
	}
	columns, source, err := validateSettings(fields)
	if err != nil {
		return 0, err
	}
	if err = EnsureDefaults(tx); err != nil {
		return 0, err
	}
	var config models.MusicConfig
	if err = tx.First(&config, 1).Error; err != nil {
		return 0, err
	}
	version := config.Version
	if expectedVersion != nil {
		version = *expectedVersion
	}
	if version == 0 || version != config.Version {
		return 0, ErrConflict
	}
	update := map[string]interface{}{"version": gorm.Expr("version + 1")}
	if source != "" {
		update["source"] = source
	}
	result := tx.Model(&models.MusicConfig{}).Where("id = ? AND version = ?", 1, version).Updates(update)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, ErrConflict
	}
	if err = loadSite(tx, site); err != nil {
		return 0, err
	}
	if len(columns) > 0 {
		if err = tx.Model(&models.SiteConfig{}).Where("id = ?", 1).Updates(columns).Error; err != nil {
			return 0, err
		}
		if err = tx.First(site, 1).Error; err != nil {
			return 0, err
		}
	}
	return version + 1, nil
}

func frontendSettings(site models.SiteConfig, source string, public bool) map[string]interface{} {
	position, theme := site.MusicPosition, site.MusicTheme
	if position == "" {
		position = "bottom-left"
	}
	if theme == "" {
		theme = "auto"
	}
	out := map[string]interface{}{
		"musicEnabled": site.MusicEnabled, "musicSource": source, "musicPosition": position, "musicTheme": theme,
		"musicLyric": site.MusicLyric, "musicAutoplay": site.MusicAutoplay, "musicDefaultMinimized": site.MusicDefaultMinimized,
		"musicEmbed": site.MusicEmbed, "musicHideOnMobile": site.MusicHideOnMobile,
	}
	if !public || source != "local" {
		out["musicPlaylistId"] = site.MusicPlaylistId
		out["musicSongId"] = site.MusicSongId
		out["musicCssCdnURL"] = site.MusicCssCdnURL
		out["musicJsCdnURL"] = site.MusicJsCdnURL
	}
	return out
}

func authorizeMusic(tx *gorm.DB, actorID uint) error {
	if !authorization.New(tx).Authorize(actorID, authorization.CapabilityMusicManage, nil).Allowed {
		return ErrForbidden
	}
	return nil
}

func writeMusicAudit(tx *gorm.DB, actorID uint, action string, details map[string]interface{}) error {
	encoded, _ := json.Marshal(details)
	actorType := "user"
	if actorID == 0 {
		actorType = "system"
	}
	return authorization.New(tx).WriteAudit(models.AdminAuditLog{ActorUserID: actorID, ActorType: actorType, Capability: string(authorization.CapabilityMusicManage), Module: "music", Action: action, TargetType: "music", TargetID: "1", Result: "success", Summary: "音乐配置操作", ChangesJSON: string(encoded)})
}

func replacePlaylist(tx *gorm.DB, ids []string) error {
	if ids == nil || len(ids) > 1000 {
		return ErrInvalid
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || len(id) > 36 || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	var existing []models.MusicPlaylistItem
	if err := tx.Find(&existing).Error; err != nil {
		return err
	}
	prior := make(map[string]bool, len(existing))
	for _, item := range existing {
		prior[item.TrackID] = true
	}
	if len(ids) > 0 {
		var tracks []models.MusicTrack
		if err := tx.Where("id IN ?", ids).Find(&tracks).Error; err != nil {
			return err
		}
		if len(tracks) != len(ids) {
			return ErrUnprocessable
		}
		for _, track := range tracks {
			if !track.Available && !prior[track.ID] {
				return ErrUnprocessable
			}
		}
	}
	if err := tx.Where("1 = 1").Delete(&models.MusicPlaylistItem{}).Error; err != nil {
		return err
	}
	items := make([]models.MusicPlaylistItem, 0, len(ids))
	for i, id := range ids {
		items = append(items, models.MusicPlaylistItem{TrackID: id, Position: i})
	}
	if len(items) > 0 {
		return tx.CreateInBatches(items, 100).Error
	}
	return nil
}

func (s *Service) SaveConfig(ctx context.Context, actorID uint, request SaveConfigRequest) (AdminConfig, error) {
	if request.Version == 0 || request.FrontendSettings == nil || request.TrackIDs == nil || !validInterval(request.ScanIntervalMinutes) {
		return AdminConfig{}, ErrInvalid
	}
	for key := range musicColumns {
		if _, ok := request.FrontendSettings[key]; !ok {
			return AdminConfig{}, ErrInvalid
		}
	}
	if _, ok := request.FrontendSettings["musicSource"]; !ok {
		return AdminConfig{}, ErrInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeMusic(tx, actorID); err != nil {
			return err
		}
		var site models.SiteConfig
		version, err := UpdateFrontendSettingsTx(tx, &site, request.FrontendSettings, &request.Version)
		if err != nil {
			return err
		}
		if err = replacePlaylist(tx, request.TrackIDs); err != nil {
			return err
		}
		if err = tx.Model(&models.MusicConfig{}).Where("id = ?", 1).Update("scan_interval_minutes", request.ScanIntervalMinutes).Error; err != nil {
			return err
		}
		var state models.MusicScanState
		if err = tx.First(&state, 1).Error; err != nil {
			return err
		}
		base := s.opts.Now()
		if state.FinishedAt != nil {
			base = *state.FinishedAt
		}
		if err = tx.Model(&state).Update("next_auto_scan_at", base.Add(time.Duration(request.ScanIntervalMinutes)*time.Minute)).Error; err != nil {
			return err
		}
		return writeMusicAudit(tx, actorID, "configure", map[string]interface{}{"source": request.FrontendSettings["musicSource"], "count": len(request.TrackIDs), "version": version})
	})
	if err != nil {
		return AdminConfig{}, err
	}
	s.notifySchedule()
	return s.GetAdminConfig(ctx)
}

func (s *Service) SavePlaylist(ctx context.Context, actorID uint, version uint64, trackIDs []string) (PlaylistResponse, error) {
	if version == 0 {
		return PlaylistResponse{}, ErrInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeMusic(tx, actorID); err != nil {
			return err
		}
		var site models.SiteConfig
		next, err := UpdateFrontendSettingsTx(tx, &site, map[string]interface{}{}, &version)
		if err != nil {
			return err
		}
		if err = replacePlaylist(tx, trackIDs); err != nil {
			return err
		}
		return writeMusicAudit(tx, actorID, "playlist", map[string]interface{}{"count": len(trackIDs), "version": next})
	})
	if err != nil {
		return PlaylistResponse{}, err
	}
	return s.GetPlaylist(ctx)
}

func validInterval(v int) bool { return v == 15 || v == 30 || v == 60 || v == 360 || v == 1440 }

func scanErrorMessage(code string) string {
	switch code {
	case "root_missing":
		return "音乐目录未挂载"
	case "root_unreadable":
		return "音乐目录不可读"
	case "tool_missing", "tool_error":
		return "音频解析工具不可用"
	case "scan_timeout":
		return "音乐扫描超时"
	case "interrupted":
		return "音乐扫描已中断"
	case "io_error":
		return "音乐目录读取失败"
	}
	return ""
}

func revisionKey(version, library uint64, identity string) string {
	return fmt.Sprintf("%s-%s-%s", strconv.FormatUint(version, 10), strconv.FormatUint(library, 10), identity)
}
