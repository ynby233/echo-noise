package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"gorm.io/gorm"
)

type AnnouncementPushSendResult struct {
	RecipientVoceChatUserID string
	Skipped                 bool
	Detail                  string
}

type AnnouncementPushSender interface {
	Send(ctx context.Context, announcement models.Announcement, recipient models.User) (AnnouncementPushSendResult, error)
}

type voceChatAnnouncementPushSender struct {
	db *gorm.DB
}

func NewVoceChatAnnouncementPushSender(db *gorm.DB) AnnouncementPushSender {
	return &voceChatAnnouncementPushSender{db: db}
}

func (s *voceChatAnnouncementPushSender) Send(ctx context.Context, announcement models.Announcement, recipient models.User) (AnnouncementPushSendResult, error) {
	if s == nil || s.db == nil {
		return AnnouncementPushSendResult{}, errors.New("公告 VoceChat 推送未初始化")
	}
	var siteConfig models.SiteConfig
	if err := s.db.First(&siteConfig).Error; err != nil {
		return AnnouncementPushSendResult{}, fmt.Errorf("读取 VoceChat 配置失败: %w", err)
	}
	vcConfig := vocechat.FromSiteConfig(siteConfig)
	if !vcConfig.IsNotificationReady() {
		return AnnouncementPushSendResult{}, errors.New("VoceChat 公告推送配置未就绪")
	}
	client, err := vocechat.NewClient(vcConfig)
	if err != nil {
		return AnnouncementPushSendResult{}, err
	}
	recipientID, err := resolveNotificationRecipientVoceChatUserID(ctx, client, vcConfig, recipient)
	if err != nil {
		return AnnouncementPushSendResult{}, err
	}
	if strings.TrimSpace(recipientID) == "" {
		return AnnouncementPushSendResult{Skipped: true, Detail: "用户未绑定可推送的 VoceChat 账号"}, nil
	}
	if err := client.SendMarkdownToUser(ctx, vcConfig.BotAPIKey, recipientID, buildAnnouncementPushMarkdown(siteConfig, announcement)); err != nil {
		return AnnouncementPushSendResult{}, err
	}
	return AnnouncementPushSendResult{RecipientVoceChatUserID: recipientID}, nil
}

func buildAnnouncementPushMarkdown(siteConfig models.SiteConfig, announcement models.Announcement) string {
	siteTitle := strings.TrimSpace(siteConfig.SiteTitle)
	if siteTitle == "" {
		siteTitle = neutralSiteTitle
	}
	title := strings.Join(strings.Fields(strings.TrimSpace(announcement.Title)), " ")
	content := strings.TrimSpace(announcement.Content)
	return strings.Join([]string{
		fmt.Sprintf("**%s 公告**", siteTitle),
		"",
		fmt.Sprintf("### %s", title),
		"",
		content,
	}, "\n")
}

type AnnouncementPushDispatchResult struct {
	Processed int `json:"processed"`
	Sent      int `json:"sent"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

type AnnouncementPushSummary struct {
	Total      int64 `json:"total"`
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	Sent       int64 `json:"sent"`
	Failed     int64 `json:"failed"`
	Skipped    int64 `json:"skipped"`
}

var (
	announcementPushDispatcherOnce sync.Once
	announcementPushWake           = make(chan struct{}, 1)
)

func WakeAnnouncementPushDispatcher() {
	select {
	case announcementPushWake <- struct{}{}:
	default:
	}
}

func StartAnnouncementPushDispatcher(ctx context.Context, db *gorm.DB) {
	if ctx == nil || db == nil {
		return
	}
	announcementPushDispatcherOnce.Do(func() {
		go runAnnouncementPushDispatcher(ctx, db)
	})
}

func runAnnouncementPushDispatcher(ctx context.Context, db *gorm.DB) {
	if recovered, err := RecoverStaleAnnouncementPushDeliveries(db, time.Now().Add(-2*time.Minute)); err != nil {
		log.Printf("恢复公告 VoceChat 推送队列失败: %v", err)
	} else if recovered > 0 {
		log.Printf("已恢复 %d 条中断的公告 VoceChat 推送", recovered)
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	sender := NewVoceChatAnnouncementPushSender(db)
	for {
		for {
			batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			result, err := DispatchPendingAnnouncementPushes(batchCtx, db, sender, 20)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("处理公告 VoceChat 推送队列失败: %v", err)
			}
			if err != nil || result.Processed < 20 {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-announcementPushWake:
		}
	}
}

func DispatchPendingAnnouncementPushes(ctx context.Context, db *gorm.DB, sender AnnouncementPushSender, limit int) (AnnouncementPushDispatchResult, error) {
	result := AnnouncementPushDispatchResult{}
	if db == nil || sender == nil {
		return result, errors.New("公告推送依赖未初始化")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var pending []models.AnnouncementPushDelivery
	if err := db.Where("status = ?", models.AnnouncementPushPending).Order("id ASC").Limit(limit).Find(&pending).Error; err != nil {
		return result, err
	}
	for _, delivery := range pending {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		now := time.Now()
		claim := db.Model(&models.AnnouncementPushDelivery{}).
			Where("id = ? AND status = ?", delivery.ID, models.AnnouncementPushPending).
			Updates(map[string]interface{}{
				"status":          models.AnnouncementPushSending,
				"attempt_count":   gorm.Expr("attempt_count + 1"),
				"last_attempt_at": now,
				"last_error":      "",
			})
		if claim.Error != nil {
			return result, claim.Error
		}
		if claim.RowsAffected == 0 {
			continue
		}
		result.Processed++

		var announcement models.Announcement
		if err := db.First(&announcement, delivery.AnnouncementID).Error; err != nil {
			if updateErr := finishAnnouncementPushDelivery(db, delivery.ID, models.AnnouncementPushSkipped, "公告已不存在", "", nil); updateErr != nil {
				return result, updateErr
			}
			result.Skipped++
			continue
		}
		if announcement.Status != models.AnnouncementStatusPublished {
			if err := finishAnnouncementPushDelivery(db, delivery.ID, models.AnnouncementPushSkipped, "公告已撤回或未发布", "", nil); err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}
		var recipient models.User
		if err := db.First(&recipient, delivery.RecipientUserID).Error; err != nil {
			if updateErr := finishAnnouncementPushDelivery(db, delivery.ID, models.AnnouncementPushSkipped, "接收用户已不存在", "", nil); updateErr != nil {
				return result, updateErr
			}
			result.Skipped++
			continue
		}
		// The delivery row is the publish-time recipient snapshot. Do not re-check
		// the user's current opt-in state, and prefer the persisted VoceChat UID so
		// a later profile change cannot silently retarget an already queued push.
		if persistedRecipientID := strings.TrimSpace(delivery.RecipientVoceChatUserID); persistedRecipientID != "" {
			recipient.VoceChatUserID = persistedRecipientID
		}
		sendResult, sendErr := sender.Send(ctx, announcement, recipient)
		if sendErr != nil {
			if err := finishAnnouncementPushDelivery(db, delivery.ID, models.AnnouncementPushFailed, sendErr.Error(), "", nil); err != nil {
				return result, err
			}
			result.Failed++
			continue
		}
		resolvedID := strings.TrimSpace(sendResult.RecipientVoceChatUserID)
		if sendResult.Skipped {
			if err := finishAnnouncementPushDelivery(db, delivery.ID, models.AnnouncementPushSkipped, sendResult.Detail, resolvedID, nil); err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}
		sentAt := time.Now()
		if err := finishAnnouncementPushDelivery(db, delivery.ID, models.AnnouncementPushSent, "", resolvedID, &sentAt); err != nil {
			return result, err
		}
		result.Sent++
	}
	return result, nil
}

func finishAnnouncementPushDelivery(db *gorm.DB, deliveryID uint, status string, detail string, recipientVoceChatUserID string, sentAt *time.Time) error {
	updates := map[string]interface{}{
		"status":     status,
		"last_error": strings.TrimSpace(detail),
		"sent_at":    sentAt,
	}
	if strings.TrimSpace(recipientVoceChatUserID) != "" {
		updates["recipient_voce_chat_user_id"] = strings.TrimSpace(recipientVoceChatUserID)
	}
	return db.Model(&models.AnnouncementPushDelivery{}).Where("id = ?", deliveryID).Updates(updates).Error
}

func GetAnnouncementPushSummary(db *gorm.DB, announcementID uint) (AnnouncementPushSummary, error) {
	summary := AnnouncementPushSummary{}
	if db == nil || announcementID == 0 {
		return summary, errors.New("公告推送统计参数无效")
	}
	type statusCount struct {
		Status string
		Count  int64
	}
	var counts []statusCount
	if err := db.Model(&models.AnnouncementPushDelivery{}).
		Select("status, COUNT(*) AS count").
		Where("announcement_id = ?", announcementID).
		Group("status").
		Scan(&counts).Error; err != nil {
		return summary, err
	}
	for _, item := range counts {
		summary.Total += item.Count
		switch item.Status {
		case models.AnnouncementPushPending:
			summary.Pending += item.Count
		case models.AnnouncementPushSending:
			summary.Processing += item.Count
		case models.AnnouncementPushSent:
			summary.Sent += item.Count
		case models.AnnouncementPushFailed:
			summary.Failed += item.Count
		case models.AnnouncementPushSkipped:
			summary.Skipped += item.Count
		}
	}
	return summary, nil
}

func RetryFailedAnnouncementPushes(db *gorm.DB, announcementID uint) (int64, error) {
	if db == nil || announcementID == 0 {
		return 0, errors.New("公告推送重试参数无效")
	}
	result := db.Model(&models.AnnouncementPushDelivery{}).
		Where("announcement_id = ? AND status = ?", announcementID, models.AnnouncementPushFailed).
		Updates(map[string]interface{}{
			"status":     models.AnnouncementPushPending,
			"last_error": "",
			"sent_at":    nil,
		})
	return result.RowsAffected, result.Error
}

func RecoverStaleAnnouncementPushDeliveries(db *gorm.DB, staleBefore time.Time) (int64, error) {
	if db == nil || staleBefore.IsZero() {
		return 0, errors.New("公告推送恢复参数无效")
	}
	result := db.Model(&models.AnnouncementPushDelivery{}).
		Where("status = ? AND (last_attempt_at IS NULL OR last_attempt_at < ?)", models.AnnouncementPushSending, staleBefore).
		Updates(map[string]interface{}{
			"status":     models.AnnouncementPushPending,
			"last_error": "发送进程中断，已自动恢复队列",
		})
	return result.RowsAffected, result.Error
}
