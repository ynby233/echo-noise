package music

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func (s *Service) readConfiguration(ctx context.Context) (models.MusicConfig, models.SiteConfig, error) {
	var cfg models.MusicConfig
	var site models.SiteConfig
	if s.db == nil {
		return cfg, site, ErrUnavailable
	}
	if err := s.db.WithContext(ctx).First(&cfg, 1).Error; err != nil {
		return cfg, site, err
	}
	if err := s.db.WithContext(ctx).First(&site, 1).Error; err != nil {
		return cfg, site, err
	}
	return cfg, site, nil
}

func (s *Service) playlistTracks(ctx context.Context) ([]models.MusicTrack, error) {
	tracks := []models.MusicTrack{}
	err := s.db.WithContext(ctx).Table("music_tracks").Select("music_tracks.*").Joins("JOIN music_playlist_items ON music_playlist_items.track_id = music_tracks.id").Order("music_playlist_items.position ASC, music_playlist_items.id ASC").Find(&tracks).Error
	return tracks, err
}

func (s *Service) adminTrack(track models.MusicTrack, selected bool) AdminTrack {
	available := track.Available && s.fingerprintFresh(track)
	cover := ""
	if available && track.CoverKind != "none" && track.CoverKind != "" {
		cover = "/api/music/library/" + track.ID + "/cover"
	}
	code := track.FailureCode
	if !available && code == "" {
		code = "missing_source"
	}
	return AdminTrack{TrackID: track.ID, Title: track.Title, Artist: track.Artist, Album: track.Album, DurationMS: track.DurationMS, Format: track.Format, CoverURL: cover, LyricsAvailable: track.LyricsKind != "none" && track.LyricsKind != "", Available: available, FailureCode: code, CueTrackNumber: track.CueTrackNumber, Selected: selected}
}

func (s *Service) GetPlaylist(ctx context.Context) (PlaylistResponse, error) {
	cfg, _, err := s.readConfiguration(ctx)
	if err != nil {
		return PlaylistResponse{}, err
	}
	tracks, err := s.playlistTracks(ctx)
	if err != nil {
		return PlaylistResponse{}, err
	}
	result := PlaylistResponse{Version: cfg.Version, Items: []AdminTrack{}}
	for _, track := range tracks {
		result.Items = append(result.Items, s.adminTrack(track, true))
	}
	return result, nil
}

func (s *Service) GetAdminConfig(ctx context.Context) (AdminConfig, error) {
	cfg, site, err := s.readConfiguration(ctx)
	if err != nil {
		return AdminConfig{}, err
	}
	playlist, err := s.GetPlaylist(ctx)
	if err != nil {
		return AdminConfig{}, err
	}
	var counts LibraryCounts
	if err = s.db.WithContext(ctx).Model(&models.MusicTrack{}).Count(&counts.Total).Error; err != nil {
		return AdminConfig{}, err
	}
	if err = s.db.WithContext(ctx).Model(&models.MusicTrack{}).Where("available = ?", true).Count(&counts.Available).Error; err != nil {
		return AdminConfig{}, err
	}
	counts.Unavailable = counts.Total - counts.Available
	root, rootErr := openRoot(s.opts.RootDir)
	if rootErr == nil {
		root.Close()
	}
	s.mu.Lock()
	ready := s.toolsReady
	s.mu.Unlock()
	return AdminConfig{Version: cfg.Version, FrontendSettings: frontendSettings(site, cfg.Source, false), ScanIntervalMinutes: cfg.ScanIntervalMinutes, Playlist: playlist.Items, Scan: s.GetScanStatus(), RootReadable: rootErr == nil, ToolsReady: ready, Counts: counts}, nil
}

func (s *Service) ListLibrary(ctx context.Context, q LibraryQuery) (LibraryPage, error) {
	if s.db == nil {
		return LibraryPage{}, ErrUnavailable
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 25
	}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || q.Page > 100000 || len(q.Q) > 1024 {
		return LibraryPage{}, ErrInvalid
	}
	if q.Lyrics == "" {
		q.Lyrics = "all"
	}
	if q.Availability == "" {
		q.Availability = "all"
	}
	if q.Selected == "" {
		q.Selected = "all"
	}
	if !oneOf(q.Lyrics, "all", "yes", "no") || !oneOf(q.Availability, "all", "available", "unavailable") || !oneOf(q.Selected, "all", "yes", "no") {
		return LibraryPage{}, ErrInvalid
	}
	query := s.db.WithContext(ctx).Model(&models.MusicTrack{})
	if strings.TrimSpace(q.Q) != "" {
		term := "%" + strings.ToLower(strings.TrimSpace(q.Q)) + "%"
		query = query.Where("LOWER(title) LIKE ? OR LOWER(artist) LIKE ? OR LOWER(album) LIKE ?", term, term, term)
	}
	if q.Format != "" {
		format := strings.ToLower(strings.TrimPrefix(q.Format, "."))
		if _, err := mediaDemuxer(format); err != nil {
			return LibraryPage{}, ErrInvalid
		}
		query = query.Where("format = ?", format)
	}
	if q.Lyrics == "yes" {
		query = query.Where("lyrics_kind <> ? AND lyrics_kind <> ?", "none", "")
	}
	if q.Lyrics == "no" {
		query = query.Where("lyrics_kind = ? OR lyrics_kind = ?", "none", "")
	}
	if q.Availability != "all" {
		query = query.Where("available = ?", q.Availability == "available")
	}
	if q.Selected != "all" {
		clause := "EXISTS"
		if q.Selected == "no" {
			clause = "NOT EXISTS"
		}
		query = query.Where(clause + " (SELECT 1 FROM music_playlist_items WHERE music_playlist_items.track_id = music_tracks.id)")
	}
	result := LibraryPage{Items: []AdminTrack{}, Page: q.Page, PageSize: q.PageSize}
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var tracks []models.MusicTrack
	if err := query.Order("title ASC, artist ASC, id ASC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&tracks).Error; err != nil {
		return result, err
	}
	var selected []models.MusicPlaylistItem
	if err := s.db.WithContext(ctx).Find(&selected).Error; err != nil {
		return result, err
	}
	set := map[string]bool{}
	for _, item := range selected {
		set[item.TrackID] = true
	}
	for _, track := range tracks {
		result.Items = append(result.Items, s.adminTrack(track, set[track.ID]))
	}
	return result, nil
}

func (s *Service) checkTrack(ctx context.Context, id string, admin bool) (models.MusicTrack, error) {
	var track models.MusicTrack
	if s.db == nil || len(id) != 36 {
		return track, ErrNotFound
	}
	if err := s.db.WithContext(ctx).First(&track, "id = ?", id).Error; err != nil {
		return track, ErrNotFound
	}
	if !track.Available || !s.fingerprintFresh(track) {
		return track, ErrNotFound
	}
	if !admin {
		cfg, site, err := s.readConfiguration(ctx)
		if err != nil || !site.MusicEnabled || cfg.Source != "local" {
			return track, ErrNotFound
		}
		var count int64
		if err = s.db.WithContext(ctx).Model(&models.MusicPlaylistItem{}).Where("track_id = ?", id).Count(&count).Error; err != nil || count != 1 {
			return track, ErrNotFound
		}
	}
	return track, nil
}

func (s *Service) GetPublic(ctx context.Context) (PublicMusic, error) {
	cfg, site, err := s.readConfiguration(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PublicMusic{Source: "netease", Revision: "initial", FrontendSettings: frontendSettings(defaultSite(), "netease", true), Tracks: []PublicTrack{}}, nil
	}
	if err != nil {
		return PublicMusic{}, err
	}
	out := PublicMusic{Source: cfg.Source, FrontendSettings: frontendSettings(site, cfg.Source, true), Tracks: []PublicTrack{}}
	identities := strings.Builder{}
	if cfg.Source == "local" && site.MusicEnabled {
		tracks, err := s.playlistTracks(ctx)
		if err != nil {
			return out, err
		}
		for _, track := range tracks {
			if !track.Available || !s.fingerprintFresh(track) {
				continue
			}
			rep := ResolveRepresentation(track, false)
			cover := ""
			if track.CoverKind != "none" && track.CoverKind != "" {
				cover = "/api/music/cover/" + track.ID
			}
			out.Tracks = append(out.Tracks, PublicTrack{TrackID: track.ID, Title: track.Title, Artist: track.Artist, Album: track.Album, DurationMS: track.DurationMS, Format: track.Format, MIMEType: rep.MIME, CoverURL: cover, LyricsAvailable: track.LyricsKind != "none" && track.LyricsKind != "", LyricsURL: "/api/music/lyrics/" + track.ID, StreamURL: "/api/music/stream/" + track.ID, FallbackStreamURL: "/api/music/stream/" + track.ID + "?compat=1", MediaVersion: rep.Key})
			fmt.Fprintf(&identities, "%s:%s;", track.ID, rep.Key)
		}
	}
	hash := sha256.Sum256([]byte(identities.String()))
	out.Revision = revisionKey(cfg.Version, cfg.LibraryVersion, hex.EncodeToString(hash[:8]))
	return out, nil
}
