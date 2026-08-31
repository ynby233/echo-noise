package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const (
	DeletionEventTrashed            = "trashed"
	DeletionEventPermanentlyDeleted = "permanently_deleted"
)

type DeletionSnapshotItem struct {
	TargetType          string     `json:"target_type"`
	TargetID            uint       `json:"target_id"`
	ContentHTML         string     `json:"content_html"`
	ContentText         string     `json:"content_text"`
	ContextText         string     `json:"context_text,omitempty"`
	ReasonCode          string     `json:"reason_code"`
	DeletedAt           *time.Time `json:"deleted_at,omitempty"`
	ScheduledDeletionAt *time.Time `json:"scheduled_deletion_at,omitempty"`
}

type deletionNotificationTarget struct {
	OwnerID     uint
	TargetType  string
	TargetID    uint
	Content     string
	ContextText string
	ReasonCode  string
	DeletedAt   *time.Time
}

var (
	managedMarkdownImage = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]*(?:/api/|/refs/)[^)]*)\)`)
	managedMarkdownLink  = regexp.MustCompile(`\[([^\]]+)\]\(([^)]*(?:/api/|/refs/)[^)]*)\)`)
	markdownSyntax       = regexp.MustCompile(`(?m)(!?)\[([^\]]*)\]\([^)]+\)|[*_~` + "`" + `>#-]+`)
	htmlTagPattern       = regexp.MustCompile(`<[^>]+>`)
)

func snapshotMarkdown(content string) (string, string) {
	content = managedMarkdownImage.ReplaceAllString(content, "[附件：$1（已不可用）]")
	content = managedMarkdownLink.ReplaceAllString(content, "$1（附件已不可用）")
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.Tables | parser.Strikethrough)
	doc := p.Parse([]byte(content))
	flags := mdhtml.CommonFlags | mdhtml.SkipHTML | mdhtml.Safelink | mdhtml.NofollowLinks | mdhtml.NoopenerLinks | mdhtml.NoreferrerLinks
	renderer := mdhtml.NewRenderer(mdhtml.RendererOptions{Flags: flags})
	htmlSnapshot := string(markdown.Render(doc, renderer))
	plain := markdownSyntax.ReplaceAllString(content, "$2")
	plain = htmlTagPattern.ReplaceAllString(plain, "")
	plain = strings.Join(strings.Fields(plain), " ")
	return htmlSnapshot, plain
}

func deletionActorLabel(tx *gorm.DB, actorID uint, system bool) string {
	if system {
		return "系统定时清理"
	}
	var actor models.User
	if err := tx.Select("id,username,is_admin").First(&actor, actorID).Error; err != nil {
		return "内容管理方"
	}
	if actor.IsAdmin {
		return "内容管理员"
	}
	if strings.TrimSpace(actor.Username) == "" {
		return "内容所有者"
	}
	return strings.TrimSpace(actor.Username)
}

func shouldNotifyDeletion(tx *gorm.DB, actorID, ownerID uint, targetType string, system bool) bool {
	if ownerID == 0 {
		return false
	}
	if system {
		return true
	}
	if actorID == ownerID {
		return false
	}
	var actor models.User
	if err := tx.Select("id,is_admin").First(&actor, actorID).Error; err != nil {
		return false
	}
	if !actor.IsAdmin {
		return true
	}
	if actor.ID == models.PrimaryAdminUserID {
		var cfg models.SiteConfig
		if err := tx.Table("site_configs").First(&cfg).Error; err != nil {
			return false
		}
		if targetType == "note" {
			return cfg.NotifyNoteDeletionByPrimary
		}
		return cfg.NotifyCommentDeletionByPrimary
	}
	capability := authorization.CapabilityCommentsNotifyDeletion
	if targetType == "note" {
		capability = authorization.CapabilityNotesNotifyDeletion
	}
	return authorization.New(tx).Authorize(actorID, capability, &ownerID).Allowed
}

func createDeletionNotificationsTx(tx *gorm.DB, actorID uint, event, batchID string, targets []deletionNotificationTarget, system bool) error {
	grouped := map[uint][]DeletionSnapshotItem{}
	for _, target := range targets {
		if !shouldNotifyDeletion(tx, actorID, target.OwnerID, target.TargetType, system) {
			continue
		}
		htmlSnapshot, textSnapshot := snapshotMarkdown(target.Content)
		retention := 0
		var cfg models.SiteConfig
		if err := tx.Table("site_configs").First(&cfg).Error; err == nil {
			if target.TargetType == "note" {
				retention = cfg.RecycleBinRetentionDays
			} else {
				retention = cfg.CommentRecycleBinRetentionDays
			}
		}
		deadline := CalculateRecycleDeadline(target.DeletedAt, retention, time.Now().UTC())
		grouped[target.OwnerID] = append(grouped[target.OwnerID], DeletionSnapshotItem{
			TargetType: target.TargetType, TargetID: target.TargetID, ContentHTML: htmlSnapshot,
			ContentText: textSnapshot, ContextText: target.ContextText, ReasonCode: target.ReasonCode,
			DeletedAt: target.DeletedAt, ScheduledDeletionAt: deadline.ScheduledDeletionAt,
		})
	}
	recipients := make([]uint, 0, len(grouped))
	for recipient := range grouped {
		recipients = append(recipients, recipient)
	}
	sort.Slice(recipients, func(i, j int) bool { return recipients[i] < recipients[j] })
	for _, recipient := range recipients {
		items := grouped[recipient]
		snapshot, err := json.Marshal(items)
		if err != nil {
			return err
		}
		var earliest *time.Time
		for _, item := range items {
			if item.ScheduledDeletionAt != nil && (earliest == nil || item.ScheduledDeletionAt.Before(*earliest)) {
				value := *item.ScheduledDeletionAt
				earliest = &value
			}
		}
		actor := actorID
		if system {
			actor = models.PrimaryAdminUserID
		}
		notification := models.UserNotification{
			RecipientUserID: recipient, ActorUserID: &actor, Type: models.UserNotificationTypeContentDeletion,
			DeletionEvent: event, DeletionReason: func() string {
				if system {
					return CommentDeletionReasonSystem
				}
				return items[0].ReasonCode
			}(),
			DeletionBatchID: batchID, DeletionActorLabel: deletionActorLabel(tx, actorID, system),
			DeletionSnapshotJSON: string(snapshot), ScheduledDeletionAt: earliest,
		}
		if err := createUserNotificationWithWebPush(tx, &notification); err != nil {
			return fmt.Errorf("create deletion notification: %w", err)
		}
	}
	return nil
}
