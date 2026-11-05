package controllers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/music"
	"github.com/rcy1314/echo-noise/internal/services"
)

func noteContainsMusic(c *gin.Context, messageID uint, trackID string) bool {
	viewer, _ := currentMessageViewer(c)
	message, err := services.GetMessageByIDForViewer(messageID, viewer)
	if err != nil || message == nil || message.DeletedAt != nil || message.IsTombstone {
		return false
	}
	ids, err := music.NoteTrackIDs(message.Content)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if id == trackID {
			return true
		}
	}
	return false
}

func (m *MusicController) noteSignature(trackID, messageID, expires string) string {
	mac := hmac.New(sha256.New, m.noteKey[:])
	_, _ = mac.Write([]byte(trackID + "\x00" + messageID + "\x00" + expires))
	return hex.EncodeToString(mac.Sum(nil))
}

func (m *MusicController) ResolveNote(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	messageID, err := strconv.ParseUint(c.Query("messageID"), 10, 32)
	if err != nil || messageID == 0 || !noteContainsMusic(c, uint(messageID), c.Param("id")) {
		musicFailure(c, music.ErrNotFound, true)
		return
	}
	result, err := m.service.ResolveNoteTrack(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, dto.Fail[any](music.ErrNoteTrackInvalid.Error()))
		return
	}
	id := strconv.FormatUint(messageID, 10)
	expires := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)
	values := url.Values{"messageID": {id}, "expires": {expires}, "signature": {m.noteSignature(result.TrackID, id, expires)}}
	signURL := func(value string) string {
		if value == "" {
			return ""
		}
		separator := "?"
		if strings.Contains(value, "?") {
			separator = "&"
		}
		return musicURL(c, value+separator+values.Encode(), false)
	}
	result.CoverURL = signURL(result.CoverURL)
	result.LyricsURL = signURL(result.LyricsURL)
	result.StreamURL = signURL(result.StreamURL)
	result.FallbackStreamURL = signURL(result.FallbackStreamURL)
	c.JSON(http.StatusOK, dto.OK(result, "笔记音乐"))
}

// Preview signatures have a separate domain from published-note signatures.
func (m *MusicController) previewSignature(trackID, actorID, expires string) string {
	mac := hmac.New(sha256.New, m.noteKey[:])
	_, _ = mac.Write([]byte("preview\x00" + trackID + "\x00" + actorID + "\x00" + expires))
	return hex.EncodeToString(mac.Sum(nil))
}

// Preview access is read-only and rechecked for every media request. In
// particular, a signed URL does not preserve a revoked music.manage grant.
func previewMusicAccess(c *gin.Context) (uint, error) {
	user, ok := currentReadUser(c)
	if !ok || user.ID == 0 {
		return 0, music.ErrForbidden
	}
	db, err := database.GetDB()
	if err != nil {
		return 0, music.ErrUnavailable
	}
	if !authorization.New(db).Authorize(user.ID, authorization.CapabilityMusicManage, nil).Allowed {
		return 0, music.ErrForbidden
	}
	var config models.MusicConfig
	var site models.SiteConfig
	if err := db.WithContext(c.Request.Context()).Select("source").First(&config, 1).Error; err != nil {
		return 0, music.ErrUnavailable
	}
	if err := db.WithContext(c.Request.Context()).Select("music_enabled").First(&site, 1).Error; err != nil {
		return 0, music.ErrUnavailable
	}
	if !site.MusicEnabled || (config.Source != "netease" && config.Source != "local") {
		return 0, music.ErrNotFound
	}
	return user.ID, nil
}

func (m *MusicController) Preview(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	actorID, err := previewMusicAccess(c)
	if err != nil {
		musicFailure(c, err, false)
		return
	}
	result, err := m.service.ResolveNoteTrack(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, dto.Fail[any](music.ErrNoteTrackInvalid.Error()))
		return
	}
	actor := strconv.FormatUint(uint64(actorID), 10)
	expires := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)
	values := url.Values{"previewActorID": {actor}, "expires": {expires}, "signature": {m.previewSignature(result.TrackID, actor, expires)}}
	signURL := func(value string) string {
		if value == "" {
			return ""
		}
		separator := "?"
		if strings.Contains(value, "?") {
			separator = "&"
		}
		return musicURL(c, value+separator+values.Encode(), false)
	}
	result.CoverURL = signURL(result.CoverURL)
	result.LyricsURL = signURL(result.LyricsURL)
	result.StreamURL = signURL(result.StreamURL)
	result.FallbackStreamURL = signURL(result.FallbackStreamURL)
	c.JSON(http.StatusOK, dto.OK(result, "音乐预览"))
}

func (m *MusicController) authorizePreviewMedia(c *gin.Context) bool {
	actor := c.Query("previewActorID")
	expires := c.Query("expires")
	actorID, idErr := strconv.ParseUint(actor, 10, 32)
	deadline, timeErr := strconv.ParseInt(expires, 10, 64)
	signature, signErr := hex.DecodeString(c.Query("signature"))
	expected, _ := hex.DecodeString(m.previewSignature(c.Param("trackID"), actor, expires))
	// Mixed note/preview credentials must never bypass the note visibility check.
	if c.Request.URL.Query().Has("messageID") || idErr != nil || actorID == 0 || timeErr != nil || deadline <= time.Now().Unix() || signErr != nil || !hmac.Equal(signature, expected) {
		musicFailure(c, music.ErrNotFound, true)
		return false
	}
	currentActor, err := previewMusicAccess(c)
	if err != nil || uint64(currentActor) != actorID {
		musicFailure(c, music.ErrNotFound, true)
		return false
	}
	c.Request = c.Request.WithContext(music.WithNoteTrackAccess(c.Request.Context(), c.Param("trackID")))
	return true
}

// A signature permits only this note/track pair. Visibility and the reference
// are checked again on every media request, so deleting or hiding a note revokes
// its URLs immediately. Private-note URLs still need the viewer's session/token.
func (m *MusicController) authorizeNoteMedia(c *gin.Context) bool {
	if c.Request.URL.Query().Has("previewActorID") {
		return m.authorizePreviewMedia(c)
	}
	if !c.Request.URL.Query().Has("messageID") && !c.Request.URL.Query().Has("signature") && !c.Request.URL.Query().Has("expires") {
		return true
	}
	id := c.Query("messageID")
	expires := c.Query("expires")
	messageID, idErr := strconv.ParseUint(id, 10, 32)
	deadline, timeErr := strconv.ParseInt(expires, 10, 64)
	signature, signErr := hex.DecodeString(c.Query("signature"))
	expected, _ := hex.DecodeString(m.noteSignature(c.Param("trackID"), id, expires))
	if idErr != nil || messageID == 0 || timeErr != nil || deadline <= time.Now().Unix() || signErr != nil || !hmac.Equal(signature, expected) || !noteContainsMusic(c, uint(messageID), c.Param("trackID")) {
		musicFailure(c, music.ErrNotFound, true)
		return false
	}
	c.Request = c.Request.WithContext(music.WithNoteTrackAccess(c.Request.Context(), c.Param("trackID")))
	return true
}

// Only new occurrences need authorization/availability validation: ordinary
// editors may keep or remove the original IDs, including a now-stale marker.
func validateMessageMusic(c *gin.Context, actorID uint, previous, content string) bool {
	ids, err := music.NoteTrackIDs(content)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any](err.Error()))
		return false
	}
	oldIDs, _ := music.NoteTrackIDs(previous)
	remaining := map[string]int{}
	for _, id := range oldIDs {
		remaining[id]++
	}
	added := []string{}
	for _, id := range ids {
		if remaining[id] > 0 {
			remaining[id]--
		} else {
			added = append(added, id)
		}
	}
	if len(added) == 0 {
		return true
	}
	db, dbErr := database.GetDB()
	if dbErr != nil {
		c.JSON(http.StatusServiceUnavailable, dto.Fail[any]("音乐服务暂时不可用"))
		return false
	}
	if !authorization.New(db).Authorize(actorID, authorization.CapabilityMusicManage, nil).Allowed {
		c.JSON(http.StatusForbidden, dto.Fail[any](music.ErrForbidden.Error()))
		return false
	}
	value, exists := c.Get("note_music_service")
	service, ok := value.(*music.Service)
	if !exists || !ok || service == nil {
		c.JSON(http.StatusServiceUnavailable, dto.Fail[any]("音乐服务暂时不可用"))
		return false
	}
	for _, id := range added {
		if _, err := service.ResolveNoteTrack(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusBadRequest, dto.Fail[any](music.ErrNoteTrackInvalid.Error()))
			return false
		}
	}
	return true
}
