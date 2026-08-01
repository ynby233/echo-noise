// Package authorization centralizes delegated administrator decisions.
package authorization

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

type Capability string

const (
	CapabilityAuthorizationManage        Capability = "authorization.manage"
	CapabilityAdminRolesManage           Capability = "admin_roles.manage"
	CapabilityAdminAccountsManage        Capability = "admin_accounts.manage"
	CapabilityAuditView                  Capability = "audit.view"
	CapabilityUsersView                  Capability = "users.view"
	CapabilityUsersManage                Capability = "users.manage"
	CapabilityUsersResetPassword         Capability = "users.reset_password"
	CapabilityUsersDelete                Capability = "users.delete"
	CapabilityRegistrationView           Capability = "registration.view"
	CapabilityRegistrationReview         Capability = "registration.review"
	CapabilityCommentsView               Capability = "comments.view"
	CapabilityCommentsEdit               Capability = "comments.edit"
	CapabilityCommentsDelete             Capability = "comments.delete"
	CapabilityAttachmentsView            Capability = "attachments.view"
	CapabilityAttachmentsDownload        Capability = "attachments.download"
	CapabilityAttachmentsDeleteReference Capability = "attachments.delete_reference"
	CapabilityAttachmentsPurgeBlob       Capability = "attachments.purge_blob"
	CapabilityStorageView                Capability = "storage.view"
	CapabilityStorageManage              Capability = "storage.manage"
	CapabilityDatabaseView               Capability = "database.view"
	CapabilityDatabaseBackup             Capability = "database.backup"
	CapabilityDatabaseRestore            Capability = "database.restore"
	CapabilityVersionView                Capability = "version.view"
	CapabilityVersionUpdate              Capability = "version.update"
	CapabilitySecurityView               Capability = "security.view"
	CapabilitySecurityManage             Capability = "security.manage"
	CapabilitySecurityClearLogs          Capability = "security.clear_logs"
	CapabilityAccessLogsView             Capability = "access_logs.view"
	CapabilityAccessLogsClear            Capability = "access_logs.clear"
	CapabilitySiteVisitsView             Capability = "site_visits.view"
	CapabilitySiteVisitsClear            Capability = "site_visits.clear"
	CapabilityLoginAuditsView            Capability = "login_audits.view"
	CapabilitySiteSettingsView           Capability = "site_settings.view"
	CapabilitySiteSettingsManage         Capability = "site_settings.manage"
	CapabilityAnnouncementsView          Capability = "announcements.view"
	CapabilityAnnouncementsManage        Capability = "announcements.manage"
	CapabilityAnnouncementsPush          Capability = "announcements.push"
	CapabilityFeedView                   Capability = "feed.view"
	CapabilityFeedManage                 Capability = "feed.manage"
	CapabilityRSSView                    Capability = "rss.view"
	CapabilityRSSManage                  Capability = "rss.manage"
	CapabilityNotificationsView          Capability = "notifications.view"
	CapabilityNotificationsManage        Capability = "notifications.manage"
	CapabilityEmailView                  Capability = "email.view"
	CapabilityEmailManage                Capability = "email.manage"
	CapabilityNotesView                  Capability = "notes.view"
	CapabilityNotesEdit                  Capability = "notes.edit"
	CapabilityNotesVisibility            Capability = "notes.change_visibility"
	CapabilityNotesPublishTime           Capability = "notes.change_publish_time"
	CapabilityNotesPinGlobal             Capability = "notes.pin_global"
	CapabilityNotesTrash                 Capability = "notes.trash"
	CapabilityNotesRestore               Capability = "notes.restore"
	CapabilityNotesDelete                Capability = "notes.delete_permanently"
)

type Definition struct {
	Capability Capability `json:"capability"`
	Module     string     `json:"module"`
	Label      string     `json:"label"`
	Grantable  bool       `json:"grantable"`
}

var catalog = []Definition{
	{CapabilityAuthorizationManage, "account_security", "管理员授权", false}, {CapabilityAdminRolesManage, "account_security", "管理员身份", false}, {CapabilityAdminAccountsManage, "account_security", "管理员账号", false},
	{CapabilityAuditView, "audit", "查看管理员审计", true},
	{CapabilityUsersView, "users", "查看用户", true}, {CapabilityUsersManage, "users", "管理普通用户", true}, {CapabilityUsersResetPassword, "users", "重置普通用户密码", true}, {CapabilityUsersDelete, "users", "删除普通用户", true},
	{CapabilityRegistrationView, "registration", "查看注册申请", true}, {CapabilityRegistrationReview, "registration", "审核注册申请", true},
	{CapabilityCommentsView, "comments", "查看评论", true}, {CapabilityCommentsEdit, "comments", "编辑评论", true}, {CapabilityCommentsDelete, "comments", "删除评论", true},
	{CapabilityAttachmentsView, "attachments", "查看附件", true}, {CapabilityAttachmentsDownload, "attachments", "下载附件", true}, {CapabilityAttachmentsDeleteReference, "attachments", "删除附件引用", true}, {CapabilityAttachmentsPurgeBlob, "attachments", "彻底删除附件文件", true},
	{CapabilityStorageView, "storage", "查看存储", true}, {CapabilityStorageManage, "storage", "管理存储", true}, {CapabilityDatabaseView, "database", "查看数据库", true}, {CapabilityDatabaseBackup, "database", "备份数据库", true}, {CapabilityDatabaseRestore, "database", "恢复数据库", true}, {CapabilityVersionView, "version", "查看版本", true}, {CapabilityVersionUpdate, "version", "更新版本", true},
	{CapabilitySecurityView, "security", "查看安全策略", true}, {CapabilitySecurityManage, "security", "管理安全策略", true}, {CapabilitySecurityClearLogs, "security", "清理攻击记录", true}, {CapabilityAccessLogsView, "access_logs", "查看访问日志", true}, {CapabilityAccessLogsClear, "access_logs", "清理访问日志", true}, {CapabilitySiteVisitsView, "site_visits", "查看访客记录", true}, {CapabilitySiteVisitsClear, "site_visits", "清理访客记录", true}, {CapabilityLoginAuditsView, "login_audits", "查看登录审计", true},
	{CapabilitySiteSettingsView, "site", "查看站点设置", true}, {CapabilitySiteSettingsManage, "site", "管理站点设置", true}, {CapabilityAnnouncementsView, "announcements", "查看公告", true}, {CapabilityAnnouncementsManage, "announcements", "管理公告", true}, {CapabilityAnnouncementsPush, "announcements", "推送公告", true}, {CapabilityFeedView, "feed", "查看信息流", true}, {CapabilityFeedManage, "feed", "刷新信息流", true}, {CapabilityRSSView, "rss", "查看 RSS", true}, {CapabilityRSSManage, "rss", "管理 RSS", true}, {CapabilityNotificationsView, "notifications", "查看通知设置", true}, {CapabilityNotificationsManage, "notifications", "管理通知设置", true}, {CapabilityEmailView, "email", "查看邮件设置", true}, {CapabilityEmailManage, "email", "管理邮件设置", true},
	{CapabilityNotesView, "notes", "查看笔记", true}, {CapabilityNotesEdit, "notes", "编辑笔记", true}, {CapabilityNotesVisibility, "notes", "调整笔记可见范围", true}, {CapabilityNotesPublishTime, "notes", "调整笔记发布时间", true}, {CapabilityNotesPinGlobal, "notes", "全站置顶笔记", true}, {CapabilityNotesTrash, "notes", "移入笔记回收站", true}, {CapabilityNotesRestore, "notes", "恢复笔记", true}, {CapabilityNotesDelete, "notes", "永久删除笔记", true},
}

type DenialReason string

const (
	DenialNone              DenialReason = ""
	DenialNotAdministrator  DenialReason = "not_administrator"
	DenialMissingGrant      DenialReason = "missing_grant"
	DenialProtectedContent  DenialReason = "protected_content"
	DenialUnknownCapability DenialReason = "unknown_capability"
)

type Decision struct {
	Allowed bool
	Reason  DenialReason
}
type Authorizer struct{ db *gorm.DB }

func New(db *gorm.DB) *Authorizer { return &Authorizer{db: db} }
func Catalog() []Definition       { return append([]Definition(nil), catalog...) }
func DefinitionFor(capability Capability) (Definition, bool) {
	for _, definition := range catalog {
		if definition.Capability == capability {
			return definition, true
		}
	}
	return Definition{}, false
}

func (a *Authorizer) Authorize(actorID uint, capability Capability, targetOwnerUserID *uint) Decision {
	definition, known := DefinitionFor(capability)
	if !known {
		return Decision{Reason: DenialUnknownCapability}
	}
	var actor models.User
	if err := a.db.Select("id,is_admin").First(&actor, actorID).Error; err != nil || !actor.IsAdmin {
		return Decision{Reason: DenialNotAdministrator}
	}
	if actor.ID == models.PrimaryAdminUserID {
		return Decision{Allowed: true}
	}
	var grants int64
	if err := a.db.Model(&models.AdminCapabilityGrant{}).Where("user_id = ? AND capability = ?", actor.ID, capability).Count(&grants).Error; err != nil || grants != 1 {
		return Decision{Reason: DenialMissingGrant}
	}
	if targetOwnerUserID != nil && *targetOwnerUserID == models.PrimaryAdminUserID && isMutation(definition.Capability) {
		return Decision{Reason: DenialProtectedContent}
	}
	return Decision{Allowed: true}
}

func isMutation(capability Capability) bool {
	switch capability {
	case CapabilityCommentsView, CapabilityAttachmentsView, CapabilityAttachmentsDownload, CapabilityUsersView, CapabilityRegistrationView, CapabilityStorageView, CapabilityDatabaseView, CapabilityVersionView, CapabilitySecurityView, CapabilityAccessLogsView, CapabilitySiteVisitsView, CapabilityLoginAuditsView, CapabilitySiteSettingsView, CapabilityAnnouncementsView, CapabilityFeedView, CapabilityRSSView, CapabilityNotificationsView, CapabilityEmailView, CapabilityNotesView, CapabilityAuditView:
		return false
	}
	return true
}

func (a *Authorizer) CapabilitiesFor(actorID uint) ([]Capability, error) {
	var actor models.User
	if err := a.db.Select("id,is_admin").First(&actor, actorID).Error; err != nil {
		return nil, err
	}
	if !actor.IsAdmin {
		return []Capability{}, nil
	}
	if actor.ID == models.PrimaryAdminUserID {
		out := make([]Capability, 0, len(catalog))
		for _, definition := range catalog {
			out = append(out, definition.Capability)
		}
		return out, nil
	}
	var grants []models.AdminCapabilityGrant
	if err := a.db.Where("user_id = ?", actor.ID).Find(&grants).Error; err != nil {
		return nil, err
	}
	out := make([]Capability, 0, len(grants))
	for _, grant := range grants {
		out = append(out, Capability(grant.Capability))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (a *Authorizer) ReplaceGrants(actorID, targetID uint, capabilities []Capability) error {
	if decision := a.Authorize(actorID, CapabilityAuthorizationManage, nil); !decision.Allowed {
		return fmt.Errorf("authorization denied: %s", decision.Reason)
	}
	if targetID == models.PrimaryAdminUserID {
		return errors.New("primary administrator has implicit capabilities")
	}
	wanted := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		definition, ok := DefinitionFor(capability)
		if !ok || !definition.Grantable {
			return fmt.Errorf("capability is not grantable: %s", capability)
		}
		wanted[capability] = struct{}{}
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		var target models.User
		if err := tx.Select("id,is_admin").First(&target, targetID).Error; err != nil {
			return err
		}
		if !target.IsAdmin {
			return errors.New("target is not a delegated administrator")
		}
		var existing []models.AdminCapabilityGrant
		if err := tx.Where("user_id = ?", targetID).Find(&existing).Error; err != nil {
			return err
		}
		current := make(map[Capability]models.AdminCapabilityGrant, len(existing))
		for _, grant := range existing {
			current[Capability(grant.Capability)] = grant
		}
		removed, granted := []string{}, []string{}
		for capability, grant := range current {
			if _, keep := wanted[capability]; !keep {
				if err := tx.Delete(&grant).Error; err != nil {
					return err
				}
				removed = append(removed, string(capability))
			}
		}
		for capability := range wanted {
			if _, exists := current[capability]; !exists {
				if err := tx.Create(&models.AdminCapabilityGrant{UserID: targetID, Capability: string(capability), GrantedByUserID: actorID}).Error; err != nil {
					return err
				}
				granted = append(granted, string(capability))
			}
		}
		sort.Strings(removed)
		sort.Strings(granted)
		changes, _ := json.Marshal(map[string][]string{"granted": granted, "revoked": removed})
		return a.writeAudit(tx, models.AdminAuditLog{ActorUserID: actorID, Capability: string(CapabilityAuthorizationManage), Module: "account_security", Action: "replace_grants", TargetType: "user", TargetID: fmt.Sprint(targetID), Result: "success", Summary: "replaced delegated administrator capabilities", ChangesJSON: string(changes)})
	})
}

func (a *Authorizer) SetAuditEnabled(actorID uint, enabled bool) error {
	if actorID != models.PrimaryAdminUserID {
		return errors.New("only primary administrator may change audit logging")
	}
	return a.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.AdminAuditConfig{}).Where("id = ?", 1).Update("enabled", enabled)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 0 {
			return nil
		}
		return tx.Model(&models.AdminAuditConfig{}).Create(map[string]interface{}{"id": 1, "enabled": enabled}).Error
	})
}
func (a *Authorizer) WriteAudit(record models.AdminAuditLog) error { return a.writeAudit(a.db, record) }
func (a *Authorizer) WriteDeniedBestEffort(record models.AdminAuditLog) {
	_ = a.writeAudit(a.db, record)
}
func (a *Authorizer) writeAudit(db *gorm.DB, record models.AdminAuditLog) error {
	if record.ActorUserID == 0 {
		return errors.New("audit actor is required")
	}
	var config models.AdminAuditConfig
	if err := db.First(&config, 1).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if config.ID == 0 {
		config = models.AdminAuditConfig{ID: 1, Enabled: true}
		if err := db.Create(&config).Error; err != nil {
			return err
		}
	}
	if !config.Enabled {
		return nil
	}
	var actor models.User
	if err := db.Select("id,username").First(&actor, record.ActorUserID).Error; err != nil {
		return err
	}
	record.ActorUsername = actor.Username
	record.ActorIsPrimary = actor.ID == models.PrimaryAdminUserID
	record.Summary = strings.TrimSpace(record.Summary)
	if record.Result == "" {
		record.Result = "success"
	}
	if record.Result != "success" && record.Result != "denied" && record.Result != "failure" {
		return errors.New("invalid audit result")
	}
	return db.Create(&record).Error
}
