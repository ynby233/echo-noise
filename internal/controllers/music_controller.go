package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/music"
)

type MusicController struct {
	service *music.Service
}

func NewMusicController(service *music.Service) *MusicController {
	return &MusicController{service: service}
}

// Decode the complete body before doing any work, including trailing objects.
func decodeMusicJSON(c *gin.Context, target interface{}) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return music.ErrInvalid
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return music.ErrInvalid
	}
	return nil
}

func musicFailure(c *gin.Context, err error, public bool) {
	status, message := http.StatusInternalServerError, "音乐服务暂时不可用"
	switch {
	case errors.Is(err, music.ErrInvalid):
		status, message = http.StatusBadRequest, music.ErrInvalid.Error()
	case errors.Is(err, music.ErrForbidden):
		status, message = http.StatusForbidden, music.ErrForbidden.Error()
	case errors.Is(err, music.ErrConflict):
		status, message = http.StatusConflict, music.ErrConflict.Error()
	case errors.Is(err, music.ErrUnprocessable):
		status, message = http.StatusUnprocessableEntity, music.ErrUnprocessable.Error()
	case errors.Is(err, music.ErrNotFound):
		status, message = http.StatusNotFound, music.ErrNotFound.Error()
	case errors.Is(err, music.ErrUnavailable), errors.Is(err, music.ErrBusy):
		status, message = http.StatusServiceUnavailable, music.ErrUnavailable.Error()
		c.Header("Retry-After", "2")
	}
	if public && status != http.StatusServiceUnavailable {
		status, message = http.StatusNotFound, music.ErrNotFound.Error()
	}
	if c.Request.Method == http.MethodHead {
		c.Status(status)
		return
	}
	c.JSON(status, dto.Fail[any](message))
}

func musicActor(c *gin.Context) (uint, error) {
	user, err := checkUser(c)
	if err != nil || user == nil {
		return 0, music.ErrForbidden
	}
	return user.ID, nil
}

func musicURL(c *gin.Context, value string, admin bool) string {
	if value == "" {
		return ""
	}
	if admin && strings.HasPrefix(c.Request.URL.Path, "/api/token/") {
		value = strings.Replace(value, "/api/music/", "/api/token/music/", 1)
	}
	prefix := strings.TrimSpace(c.GetHeader("X-Forwarded-Prefix"))
	if prefix == "" {
		prefix = strings.TrimSpace(os.Getenv("BASE_PATH"))
	}
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" || !strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "..") || strings.ContainsAny(prefix, "\\?#\r\n") || strings.Contains(prefix, "//") {
		return value
	}
	return prefix + value
}

func adminMusicURLs(c *gin.Context, tracks []music.AdminTrack) {
	for i := range tracks {
		tracks[i].CoverURL = musicURL(c, tracks[i].CoverURL, true)
	}
}

func (m *MusicController) GetConfig(c *gin.Context) {
	result, err := m.service.GetAdminConfig(c.Request.Context())
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	adminMusicURLs(c, result.Playlist)
	c.JSON(http.StatusOK, dto.OK(result, "音乐配置"))
}

func (m *MusicController) SaveConfig(c *gin.Context) {
	var input struct {
		Version             *uint64                `json:"version"`
		FrontendSettings    map[string]interface{} `json:"frontendSettings"`
		ScanIntervalMinutes *int                   `json:"scanIntervalMinutes"`
		TrackIDs            *[]string              `json:"trackIDs"`
	}
	if err := decodeMusicJSON(c, &input); err != nil || input.Version == nil || input.FrontendSettings == nil || input.ScanIntervalMinutes == nil || input.TrackIDs == nil {
		musicFailure(c, music.ErrInvalid, false)
		return
	}
	actorID, err := musicActor(c)
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	result, err := m.service.SaveConfig(c.Request.Context(), actorID, music.SaveConfigRequest{
		Version: *input.Version, FrontendSettings: input.FrontendSettings,
		ScanIntervalMinutes: *input.ScanIntervalMinutes, TrackIDs: *input.TrackIDs,
	})
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	middleware.MarkSemanticAuditWritten(c)
	adminMusicURLs(c, result.Playlist)
	c.JSON(http.StatusOK, dto.OK(result, "音乐配置已保存"))
}

func musicPageParameter(c *gin.Context, name string, fallback, max int) (int, error) {
	raw, exists := c.GetQuery(name)
	if !exists {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || (max > 0 && n > max) {
		return 0, music.ErrInvalid
	}
	return n, nil
}

func musicFilter(c *gin.Context, key string, allowed ...string) (string, error) {
	value := c.DefaultQuery(key, "all")
	for _, candidate := range allowed {
		if value == candidate {
			return value, nil
		}
	}
	return "", music.ErrInvalid
}

func (m *MusicController) ListLibrary(c *gin.Context) {
	page, err := musicPageParameter(c, "page", 1, 0)
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	pageSize, err := musicPageParameter(c, "pageSize", 25, 100)
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	lyrics, err := musicFilter(c, "lyrics", "all", "yes", "no")
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	availability, err := musicFilter(c, "availability", "all", "available", "unavailable")
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	selected, err := musicFilter(c, "selected", "all", "yes", "no")
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	result, err := m.service.ListLibrary(c.Request.Context(), music.LibraryQuery{
		Page: page, PageSize: pageSize, Q: c.Query("q"), Format: c.Query("format"),
		Lyrics: lyrics, Availability: availability, Selected: selected,
	})
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	adminMusicURLs(c, result.Items)
	c.JSON(http.StatusOK, dto.OK(result, "音乐库"))
}

func (m *MusicController) Refresh(c *gin.Context) {
	actorID, err := musicActor(c)
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	result, started, err := m.service.RequestScan(c.Request.Context(), actorID)
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	middleware.MarkSemanticAuditWritten(c)
	status := http.StatusOK
	if started {
		status = http.StatusAccepted
	}
	c.JSON(status, dto.OK(gin.H{"scan": result, "started": started}, "音乐库刷新"))
}

func (m *MusicController) ScanStatus(c *gin.Context) {
	c.JSON(http.StatusOK, dto.OK(m.service.GetScanStatus(), "音乐库刷新状态"))
}

func (m *MusicController) GetPlaylist(c *gin.Context) {
	result, err := m.service.GetPlaylist(c.Request.Context())
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	adminMusicURLs(c, result.Items)
	c.JSON(http.StatusOK, dto.OK(result, "全站歌单"))
}

func (m *MusicController) SavePlaylist(c *gin.Context) {
	var input struct {
		Version  *uint64   `json:"version"`
		TrackIDs *[]string `json:"trackIDs"`
	}
	if err := decodeMusicJSON(c, &input); err != nil || input.Version == nil || input.TrackIDs == nil {
		musicFailure(c, music.ErrInvalid, false)
		return
	}
	actorID, err := musicActor(c)
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	result, err := m.service.SavePlaylist(c.Request.Context(), actorID, *input.Version, *input.TrackIDs)
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	middleware.MarkSemanticAuditWritten(c)
	adminMusicURLs(c, result.Items)
	c.JSON(http.StatusOK, dto.OK(result, "全站歌单已保存"))
}

func (m *MusicController) Public(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	result, err := m.service.GetPublic(c.Request.Context())
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	for i := range result.Tracks {
		track := &result.Tracks[i]
		track.CoverURL = musicURL(c, track.CoverURL, false)
		track.LyricsURL = musicURL(c, track.LyricsURL, false)
		track.StreamURL = musicURL(c, track.StreamURL, false)
		track.FallbackStreamURL = musicURL(c, track.FallbackStreamURL, false)
	}
	c.JSON(http.StatusOK, dto.OK(result, "公开音乐"))
}

func (m *MusicController) serveMedia(c *gin.Context, kind string, admin bool) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	compat := c.Query("compat") == "1"
	file, err := m.service.OpenMedia(c.Request.Context(), c.Param("trackID"), kind, compat, admin)
	if err != nil {
		musicFailure(c, err, !admin)
		return
	}
	defer file.Close()
	c.Header("Content-Type", file.MIME)
	c.Header("ETag", file.ETag)
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": file.Name}))
	http.ServeContent(c.Writer, c.Request, file.Name, file.Info.ModTime(), file.File)
}

func (m *MusicController) Stream(c *gin.Context)     { m.serveMedia(c, "stream", false) }
func (m *MusicController) Cover(c *gin.Context)      { m.serveMedia(c, "cover", false) }
func (m *MusicController) AdminCover(c *gin.Context) { m.serveMedia(c, "cover", true) }
func (m *MusicController) Lyrics(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	result, err := m.service.GetLyrics(c.Request.Context(), c.Param("trackID"))
	if err != nil {
		musicFailure(c, err, true)
		return
	}
	c.JSON(http.StatusOK, dto.OK(result, "歌词"))
}
