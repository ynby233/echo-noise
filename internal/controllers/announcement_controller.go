package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	announcementDeviceCookieName = "echo_announcement_device"
	announcementDeviceMaxAge     = 60 * 60 * 24 * 365 * 2
)

type unreadAnnouncementPayload struct {
	UnreadCount int                   `json:"unread_count"`
	Items       []models.Announcement `json:"items"`
}

type announcementListItem struct {
	models.Announcement
	Read bool `json:"read"`
}

type announcementListPayload struct {
	Items       []announcementListItem `json:"items"`
	Page        int                    `json:"page"`
	PageSize    int                    `json:"page_size"`
	Total       int64                  `json:"total"`
	UnreadCount int                    `json:"unread_count"`
}

type adminAnnouncementListItem struct {
	models.Announcement
	PushSummary services.AnnouncementPushSummary `json:"push_summary"`
}

type adminAnnouncementListPayload struct {
	Items    []adminAnnouncementListItem `json:"items"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
	Total    int64                       `json:"total"`
}

type announcementReaderIdentity struct {
	ReaderType string
	ReaderKey  string
}

type createAnnouncementRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type publishAnnouncementRequest struct {
	PushEnabled bool `json:"push_enabled"`
}

type updateAnnouncementRequest struct {
	Title    string `json:"title"`
	Content  string `json:"content"`
	Renotify bool   `json:"renotify"`
}

type batchDeleteAnnouncementsRequest struct {
	IDs []uint `json:"ids"`
}

type batchDeleteAnnouncementsPayload struct {
	DeletedCount int    `json:"deleted_count"`
	SkippedIDs   []uint `json:"skipped_ids"`
}

func announcementDeviceReaderKey(c *gin.Context) string {
	token, err := c.Cookie(announcementDeviceCookieName)
	token = strings.TrimSpace(token)
	if err != nil || token == "" {
		token = models.GenerateToken(64)
		c.SetSameSite(http.SameSiteLaxMode)
		secure := c.Request.TLS != nil || strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
		c.SetCookie(announcementDeviceCookieName, token, announcementDeviceMaxAge, "/", "", secure, true)
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func announcementReaderIdentities(c *gin.Context) []announcementReaderIdentity {
	identities := []announcementReaderIdentity{{
		ReaderType: models.AnnouncementReaderDevice,
		ReaderKey:  announcementDeviceReaderKey(c),
	}}
	if user, ok := currentReadUser(c); ok && user.ID != 0 {
		identities = append(identities, announcementReaderIdentity{
			ReaderType: models.AnnouncementReaderUser,
			ReaderKey:  strconv.FormatUint(uint64(user.ID), 10),
		})
	}
	return identities
}

func upsertAnnouncementRead(db *gorm.DB, announcementID uint, revision uint, identity announcementReaderIdentity, readAt time.Time) error {
	read := models.AnnouncementRead{
		AnnouncementID: announcementID,
		ReaderType:     identity.ReaderType,
		ReaderKey:      identity.ReaderKey,
		Revision:       revision,
		ReadAt:         readAt,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "announcement_id"}, {Name: "reader_type"}, {Name: "reader_key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"revision":   revision,
			"read_at":    readAt,
			"updated_at": readAt,
		}),
	}).Create(&read).Error
}

func loadAndMergeAnnouncementReadRevisions(db *gorm.DB, identities []announcementReaderIdentity) (map[uint]uint, error) {
	readRevisions := make(map[uint]uint)
	if len(identities) == 0 {
		return readRevisions, nil
	}
	query := db.Model(&models.AnnouncementRead{})
	for index, identity := range identities {
		condition := "reader_type = ? AND reader_key = ?"
		if index == 0 {
			query = query.Where(condition, identity.ReaderType, identity.ReaderKey)
		} else {
			query = query.Or(condition, identity.ReaderType, identity.ReaderKey)
		}
	}
	var reads []models.AnnouncementRead
	if err := query.Find(&reads).Error; err != nil {
		return nil, err
	}
	for _, read := range reads {
		if read.Revision > readRevisions[read.AnnouncementID] {
			readRevisions[read.AnnouncementID] = read.Revision
		}
	}
	if len(identities) < 2 || len(readRevisions) == 0 {
		return readRevisions, nil
	}
	now := time.Now()
	if err := db.Transaction(func(tx *gorm.DB) error {
		for announcementID, revision := range readRevisions {
			for _, identity := range identities {
				if err := upsertAnnouncementRead(tx, announcementID, revision, identity, now); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return readRevisions, nil
}

func GetUnreadAnnouncements(c *gin.Context) {
	identities := announcementReaderIdentities(c)
	var announcements []models.Announcement
	if err := database.DB.
		Where("status = ?", models.AnnouncementStatusPublished).
		Order("published_at DESC, id DESC").
		Find(&announcements).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取公告失败"))
		return
	}

	if len(announcements) > 0 {
		readRevisions, err := loadAndMergeAnnouncementReadRevisions(database.DB, identities)
		if err != nil {
			c.JSON(http.StatusOK, dto.Fail[any]("获取公告已读状态失败"))
			return
		}
		unread := announcements[:0]
		for _, announcement := range announcements {
			if readRevisions[announcement.ID] < announcement.Revision {
				unread = append(unread, announcement)
			}
		}
		announcements = unread
	}

	c.JSON(http.StatusOK, dto.OK(unreadAnnouncementPayload{
		UnreadCount: len(announcements),
		Items:       announcements,
	}, "获取未读公告成功"))
}

func parseAnnouncementPagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func ListAnnouncements(c *gin.Context) {
	identities := announcementReaderIdentities(c)
	readRevisions, err := loadAndMergeAnnouncementReadRevisions(database.DB, identities)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取公告已读状态失败"))
		return
	}

	var revisions []models.Announcement
	if err := database.DB.
		Select("id, revision").
		Where("status = ?", models.AnnouncementStatusPublished).
		Find(&revisions).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取公告失败"))
		return
	}
	unreadCount := 0
	for _, announcement := range revisions {
		if readRevisions[announcement.ID] < announcement.Revision {
			unreadCount++
		}
	}

	page, pageSize := parseAnnouncementPagination(c)
	var announcements []models.Announcement
	if err := database.DB.
		Where("status = ?", models.AnnouncementStatusPublished).
		Order("published_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&announcements).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取公告失败"))
		return
	}
	items := make([]announcementListItem, 0, len(announcements))
	for _, announcement := range announcements {
		items = append(items, announcementListItem{
			Announcement: announcement,
			Read:         readRevisions[announcement.ID] >= announcement.Revision,
		})
	}
	c.JSON(http.StatusOK, dto.OK(announcementListPayload{
		Items:       items,
		Page:        page,
		PageSize:    pageSize,
		Total:       int64(len(revisions)),
		UnreadCount: unreadCount,
	}, "获取公告成功"))
}

func MarkAnnouncementRead(c *gin.Context) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的公告 ID"))
		return
	}
	var announcement models.Announcement
	if err := database.DB.First(&announcement, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, dto.OK[any](nil, "公告已下架，无需标记已读"))
			return
		}
		c.JSON(http.StatusOK, dto.Fail[any]("读取公告失败"))
		return
	}
	if announcement.Status != models.AnnouncementStatusPublished {
		c.JSON(http.StatusOK, dto.OK[any](nil, "公告已下架，无需标记已读"))
		return
	}
	now := time.Now()
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		for _, identity := range announcementReaderIdentities(c) {
			if err := upsertAnnouncementRead(tx, announcement.ID, announcement.Revision, identity, now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("标记公告已读失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "公告已读"))
}

func MarkAllAnnouncementsRead(c *gin.Context) {
	var announcements []models.Announcement
	if err := database.DB.
		Select("id, revision").
		Where("status = ?", models.AnnouncementStatusPublished).
		Find(&announcements).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取未读公告失败"))
		return
	}
	identities := announcementReaderIdentities(c)
	now := time.Now()
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		for _, announcement := range announcements {
			for _, identity := range identities {
				if err := upsertAnnouncementRead(tx, announcement.ID, announcement.Revision, identity, now); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("全部标记已读失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"unread_count": 0}, "全部公告已读"))
}

func CreateAnnouncement(c *gin.Context) {
	adminID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	var request createAnnouncementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告参数错误"))
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	request.Content = strings.TrimSpace(request.Content)
	if request.Title == "" || request.Content == "" {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告标题和正文不能为空"))
		return
	}
	if utf8.RuneCountInString(request.Title) > 100 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告标题不能超过 100 个字符"))
		return
	}
	announcement := models.Announcement{
		Title:        request.Title,
		Content:      request.Content,
		Status:       models.AnnouncementStatusDraft,
		Revision:     1,
		AuthorUserID: adminID,
	}
	if err := database.DB.Create(&announcement).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("创建公告失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(announcement, "公告草稿已创建"))
}

func ListAdminAnnouncements(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	page, pageSize := parseAnnouncementPagination(c)
	status := strings.TrimSpace(c.Query("status"))
	query := database.DB.Model(&models.Announcement{})
	if status != "" && status != "all" {
		switch status {
		case models.AnnouncementStatusDraft, models.AnnouncementStatusPublished, models.AnnouncementStatusWithdrawn:
			query = query.Where("status = ?", status)
		default:
			c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的公告状态"))
			return
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("统计公告失败"))
		return
	}
	var announcements []models.Announcement
	if err := query.Order("updated_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&announcements).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取公告管理列表失败"))
		return
	}
	items := make([]adminAnnouncementListItem, 0, len(announcements))
	for _, announcement := range announcements {
		summary, err := services.GetAnnouncementPushSummary(database.DB, announcement.ID)
		if err != nil {
			c.JSON(http.StatusOK, dto.Fail[any]("获取公告推送统计失败"))
			return
		}
		items = append(items, adminAnnouncementListItem{Announcement: announcement, PushSummary: summary})
	}
	c.JSON(http.StatusOK, dto.OK(adminAnnouncementListPayload{
		Items: items, Page: page, PageSize: pageSize, Total: total,
	}, "获取公告管理列表成功"))
}

func PublishAnnouncement(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的公告 ID"))
		return
	}
	var request publishAnnouncementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告发布参数错误"))
		return
	}
	var announcement models.Announcement
	if err := database.DB.First(&announcement, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, dto.Fail[any]("公告不存在"))
		return
	}
	if announcement.Status == models.AnnouncementStatusPublished {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告已经发布"))
		return
	}
	if announcement.Status != models.AnnouncementStatusDraft && announcement.Status != models.AnnouncementStatusWithdrawn {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告状态不允许发布"))
		return
	}
	firstPublication := announcement.Status == models.AnnouncementStatusDraft && announcement.PublishedAt == nil
	now := time.Now()
	announcement.Status = models.AnnouncementStatusPublished
	announcement.WithdrawnAt = nil
	if firstPublication {
		announcement.PublishedAt = &now
		announcement.PushEnabled = request.PushEnabled
	}
	queueVoceChatPush := firstPublication && request.PushEnabled && services.VoceChatPushEnabled(database.DB)
	queueWebPush := firstPublication && request.PushEnabled
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&announcement).Error; err != nil {
			return err
		}
		if queueVoceChatPush {
			var recipients []models.User
			if err := tx.
				Select("id, voce_chat_user_id").
				Where("voce_chat_notification_enabled = ?", true).
				Find(&recipients).Error; err != nil {
				return err
			}
			deliveries := make([]models.AnnouncementPushDelivery, 0, len(recipients))
			for _, recipient := range recipients {
				deliveries = append(deliveries, models.AnnouncementPushDelivery{
					AnnouncementID:          announcement.ID,
					RecipientUserID:         recipient.ID,
					RecipientVoceChatUserID: strings.TrimSpace(recipient.VoceChatUserID),
					Status:                  models.AnnouncementPushPending,
				})
			}
			if len(deliveries) > 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&deliveries).Error; err != nil {
					return err
				}
			}
		}
		if queueWebPush {
			return services.QueueWebPushForAnnouncement(tx, announcement)
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("发布公告失败"))
		return
	}
	if queueVoceChatPush {
		services.WakeAnnouncementPushDispatcher()
	}
	c.JSON(http.StatusOK, dto.OK(announcement, "公告已发布"))
}

func UpdateAnnouncement(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的公告 ID"))
		return
	}
	var request updateAnnouncementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告参数错误"))
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	request.Content = strings.TrimSpace(request.Content)
	if request.Title == "" || request.Content == "" {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告标题和正文不能为空"))
		return
	}
	if utf8.RuneCountInString(request.Title) > 100 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告标题不能超过 100 个字符"))
		return
	}
	var announcement models.Announcement
	if err := database.DB.First(&announcement, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, dto.Fail[any]("公告不存在"))
		return
	}
	updates := map[string]interface{}{
		"title":   request.Title,
		"content": request.Content,
	}
	if request.Renotify && announcement.Status == models.AnnouncementStatusPublished {
		updates["revision"] = gorm.Expr("revision + 1")
	}
	if err := database.DB.Model(&announcement).Updates(updates).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("更新公告失败"))
		return
	}
	if err := database.DB.First(&announcement, announcement.ID).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("读取更新后的公告失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(announcement, "公告已更新"))
}

func WithdrawAnnouncement(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的公告 ID"))
		return
	}
	now := time.Now()
	result := database.DB.Model(&models.Announcement{}).
		Where("id = ? AND status = ?", uint(id), models.AnnouncementStatusPublished).
		Updates(map[string]interface{}{
			"status":       models.AnnouncementStatusWithdrawn,
			"withdrawn_at": now,
		})
	if result.Error != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("撤回公告失败"))
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("公告不存在或当前状态不可撤回"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "公告已撤回"))
}

func BatchDeleteAnnouncements(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	var request batchDeleteAnnouncementsRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.IDs) == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("请选择要删除的公告"))
		return
	}
	requested := make([]uint, 0, len(request.IDs))
	seen := make(map[uint]struct{}, len(request.IDs))
	for _, id := range request.IDs {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		requested = append(requested, id)
	}
	if len(requested) == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("请选择有效的公告"))
		return
	}
	var deletable []models.Announcement
	if err := database.DB.
		Select("id").
		Where("id IN ? AND status IN ?", requested, []string{models.AnnouncementStatusDraft, models.AnnouncementStatusWithdrawn}).
		Find(&deletable).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("查询可删除公告失败"))
		return
	}
	deletableIDs := make([]uint, 0, len(deletable))
	deletableSet := make(map[uint]struct{}, len(deletable))
	for _, announcement := range deletable {
		deletableIDs = append(deletableIDs, announcement.ID)
		deletableSet[announcement.ID] = struct{}{}
	}
	skipped := make([]uint, 0)
	for _, id := range requested {
		if _, ok := deletableSet[id]; !ok {
			skipped = append(skipped, id)
		}
	}
	if len(deletableIDs) > 0 {
		if err := database.DB.Transaction(func(tx *gorm.DB) error {
			return deleteAnnouncementRecords(tx, deletableIDs)
		}); err != nil {
			c.JSON(http.StatusOK, dto.Fail[any]("删除公告失败"))
			return
		}
	}
	c.JSON(http.StatusOK, dto.OK(batchDeleteAnnouncementsPayload{
		DeletedCount: len(deletableIDs),
		SkippedIDs:   skipped,
	}, "公告批量删除完成"))
}

func deleteAnnouncementRecords(tx *gorm.DB, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Where("announcement_id IN ?", ids).Delete(&models.AnnouncementRead{}).Error; err != nil {
		return err
	}
	if err := tx.Where("announcement_id IN ?", ids).Delete(&models.AnnouncementPushDelivery{}).Error; err != nil {
		return err
	}
	return tx.Where("id IN ?", ids).Delete(&models.Announcement{}).Error
}

func DeleteAnnouncement(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的公告 ID"))
		return
	}
	var announcement models.Announcement
	if err := database.DB.First(&announcement, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, dto.Fail[any]("公告不存在"))
		return
	}
	if announcement.Status == models.AnnouncementStatusPublished {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("已发布公告必须先撤回后才能删除"))
		return
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		return deleteAnnouncementRecords(tx, []uint{announcement.ID})
	}); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("删除公告失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "公告已删除"))
}

func GetAnnouncementPushSummary(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的公告 ID"))
		return
	}
	var exists int64
	if err := database.DB.Model(&models.Announcement{}).Where("id = ?", uint(id)).Count(&exists).Error; err != nil || exists == 0 {
		c.JSON(http.StatusNotFound, dto.Fail[any]("公告不存在"))
		return
	}
	summary, err := services.GetAnnouncementPushSummary(database.DB, uint(id))
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取公告推送统计失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(summary, "获取公告推送统计成功"))
}

func RetryAnnouncementPush(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的公告 ID"))
		return
	}
	var announcement models.Announcement
	if err := database.DB.First(&announcement, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, dto.Fail[any]("公告不存在"))
		return
	}
	if announcement.Status != models.AnnouncementStatusPublished {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("仅已发布公告可以重试失败推送"))
		return
	}
	retried, err := services.RetryFailedAnnouncementPushes(database.DB, uint(id))
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("重新排队失败的公告推送失败"))
		return
	}
	if retried > 0 {
		services.WakeAnnouncementPushDispatcher()
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"retried_count": retried}, "失败推送已重新排队"))
}
