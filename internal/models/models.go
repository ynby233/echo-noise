package models

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const PrimaryAdminUserID uint = 1

var DB *gorm.DB

type UserStatus struct {
	ID                          uint   `json:"id"`
	Username                    string `json:"username"`
	IsAdmin                     bool   `json:"is_admin"`
	AvatarURL                   string `json:"avatar_url,omitempty"`
	VoceChatEmail               string `json:"voce_chat_email,omitempty"`
	VoceChatNotificationEnabled *bool  `json:"voce_chat_notification_enabled,omitempty"`
	Status                      Status `json:"status"`
}

type Message struct {
	ID               uint       `gorm:"primaryKey;index:idx_msg_global_pin_order,priority:3;index:idx_msg_personal_pin_order,priority:4" json:"id"`
	Content          string     `gorm:"type:text;not null" json:"content"`
	Username         string     `gorm:"type:varchar(100)" json:"username,omitempty"`
	ImageURL         string     `gorm:"type:text" json:"image_url,omitempty"`
	Private          bool       `gorm:"default:false" json:"private"`
	Visibility       string     `gorm:"type:varchar(20);not null;default:public;index" json:"visibility"`
	UserID           uint       `gorm:"not null;index;index:idx_msg_personal_pin_order,priority:1" json:"user_id"`
	IsGuestbook      bool       `gorm:"default:false;index" json:"-"`
	CreatedAt        time.Time  `gorm:"index:idx_msg_global_pin_order,priority:2;index:idx_msg_personal_pin_order,priority:3" json:"created_at"`
	DeletedAt        *time.Time `gorm:"index" json:"-"`
	DeletedByUserID  *uint      `gorm:"index" json:"-"`
	DeletedReason    string     `gorm:"type:varchar(255)" json:"-"`
	Notify           bool       `gorm:"default:false" json:"notify"` // 新增推送通知字段
	Pinned           bool       `gorm:"default:false;index:idx_msg_global_pin_order,priority:1" json:"pinned"`
	PinnedAt         *time.Time `gorm:"index" json:"-"`
	PersonalPinned   bool       `gorm:"default:false;index:idx_msg_personal_pin_order,priority:2" json:"personal_pinned"`
	PersonalPinnedAt *time.Time `gorm:"index" json:"-"`
	LikeCount        int        `gorm:"default:0" json:"like_count"`
	Liked            bool       `gorm:"-" json:"liked"`
	CanInteract      bool       `gorm:"-" json:"can_interact"`
}

type CloudAttachmentObject struct {
	ID                   uint      `gorm:"primaryKey" json:"id"`
	PublicID             string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"public_id"`
	ObjectKey            string    `gorm:"type:varchar(1024);not null;uniqueIndex" json:"-"`
	OriginalName         string    `gorm:"type:varchar(255);not null" json:"original_name"`
	ContentType          string    `gorm:"type:varchar(191)" json:"content_type"`
	ContentHash          string    `gorm:"type:varchar(64);index" json:"-"`
	LegacyObjectKey      string    `gorm:"type:varchar(1024);index" json:"-"`
	LegacyCleanupPending bool      `gorm:"default:false;index" json:"-"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// AttachmentBlob is one physical object addressed by its content hash. Access
// is never granted through this record directly; callers must resolve an
// AttachmentReference first.
type AttachmentBlob struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	StorageBackend string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_attachment_blob_backend_hash" json:"storage_backend"`
	StorageKey     string    `gorm:"type:varchar(1024);not null;uniqueIndex" json:"-"`
	ContentHash    string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_attachment_blob_backend_hash" json:"-"`
	Size           int64     `gorm:"not null;default:0" json:"size"`
	ContentType    string    `gorm:"type:varchar(191)" json:"content_type"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AttachmentReference is the logical attachment shown to users and admins.
// Multiple references may safely share one AttachmentBlob while retaining
// separate opaque ids, owners, and original names.
type AttachmentReference struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	PublicID     string         `gorm:"type:varchar(64);not null;uniqueIndex" json:"public_id"`
	BlobID       uint           `gorm:"not null;index" json:"blob_id"`
	OwnerUserID  uint           `gorm:"not null;index" json:"owner_user_id"`
	Kind         string         `gorm:"type:varchar(20);not null;index" json:"kind"`
	OriginalName string         `gorm:"type:varchar(255);not null" json:"original_name"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Blob         AttachmentBlob `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;foreignKey:BlobID" json:"-"`
}

// LocalAttachmentGrant persists the last visibility under which a message
// referenced a local file. Records intentionally survive message deletion or
// reference removal so formerly restricted files cannot become public again.
type LocalAttachmentGrant struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Kind        string    `gorm:"type:varchar(20);not null;uniqueIndex:idx_local_attachment_grant" json:"kind"`
	Name        string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_local_attachment_grant" json:"name"`
	MessageID   uint      `gorm:"not null;uniqueIndex:idx_local_attachment_grant;index" json:"message_id"`
	OwnerUserID uint      `gorm:"not null;index" json:"owner_user_id"`
	Visibility  string    `gorm:"type:varchar(20);not null;default:private;index" json:"visibility"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MessageLike 点赞记录（用于幂等与取消点赞）
type MessageLike struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	MessageID uint      `gorm:"index;not null" json:"message_id"`
	UserID    *uint     `gorm:"index" json:"user_id,omitempty"`
	SessionID string    `gorm:"type:varchar(191);index" json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
}

type CommentUserInfo struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type Comment struct {
	ID          uint             `gorm:"primaryKey" json:"id"`
	MessageID   uint             `gorm:"index;not null" json:"message_id"`
	UserID      *uint            `gorm:"index" json:"user_id,omitempty"`
	User        *CommentUserInfo `gorm:"-" json:"user,omitempty"`
	CanInteract bool             `gorm:"-" json:"can_interact"`
	Content     string           `gorm:"type:text;not null" json:"content"`
	Visibility  string           `gorm:"type:varchar(20);not null;default:public;index" json:"visibility"`
	ParentID    *uint            `json:"parent_id"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

const (
	UserNotificationTypeLike      = "like"
	UserNotificationTypeComment   = "comment"
	UserNotificationTypeReply     = "reply"
	UserNotificationTypeGuestbook = "guestbook"
)

type UserNotification struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	RecipientUserID uint       `gorm:"not null;index" json:"recipient_user_id"`
	ActorUserID     *uint      `gorm:"index" json:"actor_user_id,omitempty"`
	Type            string     `gorm:"type:varchar(30);not null;index" json:"type"`
	MessageID       *uint      `gorm:"index" json:"message_id,omitempty"`
	CommentID       *uint      `gorm:"index" json:"comment_id,omitempty"`
	ParentCommentID *uint      `gorm:"index" json:"parent_comment_id,omitempty"`
	ReadAt          *time.Time `gorm:"index" json:"read_at,omitempty"`
	CreatedAt       time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type User struct {
	ID                          uint       `gorm:"primaryKey" json:"id"`
	Username                    string     `gorm:"type:varchar(191);not null;uniqueIndex" json:"username"`
	Password                    string     `gorm:"type:varchar(191);not null" json:"password"`
	IsAdmin                     bool       `json:"is_admin"`
	Token                       string     `gorm:"type:varchar(191)" json:"token"`
	AvatarURL                   string     `gorm:"type:varchar(191)" json:"avatar_url"`
	Description                 string     `gorm:"type:varchar(191)" json:"description"`
	Email                       string     `gorm:"type:varchar(191)" json:"email"`
	EmailVerified               bool       `json:"email_verified"`
	EmailPending                string     `gorm:"type:varchar(191)" json:"-"`
	EmailVerifyCode             string     `gorm:"type:varchar(20)" json:"-"`
	EmailVerifyExpires          *time.Time `json:"-"`
	VoceChatUserID              string     `gorm:"type:varchar(191);index" json:"voce_chat_user_id,omitempty"`
	VoceChatEmail               string     `gorm:"type:varchar(191);index" json:"voce_chat_email,omitempty"`
	VoceChatUsername            string     `gorm:"type:varchar(191)" json:"voce_chat_username,omitempty"`
	VoceChatLinkedAt            *time.Time `json:"voce_chat_linked_at,omitempty"`
	VoceChatSyncStatus          string     `gorm:"type:varchar(30);default:none;index" json:"voce_chat_sync_status,omitempty"`
	VoceChatSyncError           string     `gorm:"type:text" json:"-"`
	VoceChatLastSyncAt          *time.Time `json:"voce_chat_last_sync_at,omitempty"`
	VoceChatNotificationEnabled bool       `gorm:"default:false;not null" json:"voce_chat_notification_enabled"`
}

const (
	RegistrationApplicationStatusPending  = "pending"
	RegistrationApplicationStatusApproved = "approved"
	RegistrationApplicationStatusRejected = "rejected"

	VoceChatSyncStatusNone       = "none"
	VoceChatSyncStatusPending    = "pending"
	VoceChatSyncStatusCreated    = "created"
	VoceChatSyncStatusLinked     = "linked"
	VoceChatSyncStatusFailed     = "failed"
	VoceChatSyncStatusConflicted = "conflicted"

	VoceChatContactSyncStatusOK     = "ok"
	VoceChatContactSyncStatusFailed = "failed"
)

type RegistrationApplication struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	ApplicationID      string     `gorm:"type:varchar(64);not null;uniqueIndex" json:"application_id"`
	Username           string     `gorm:"type:varchar(191);not null;index" json:"username"`
	PasswordHash       string     `gorm:"type:varchar(191);not null" json:"-"`
	Status             string     `gorm:"type:varchar(20);not null;default:pending;index" json:"status"`
	VoceChatUserID     string     `gorm:"type:varchar(191);index" json:"voce_chat_user_id,omitempty"`
	VoceChatEmail      string     `gorm:"type:varchar(191);index" json:"voce_chat_email,omitempty"`
	VoceChatSyncStatus string     `gorm:"type:varchar(30);default:none;index" json:"voce_chat_sync_status,omitempty"`
	VoceChatSyncError  string     `gorm:"type:text" json:"-"`
	LocalUserID        *uint      `gorm:"index" json:"local_user_id,omitempty"`
	ReviewerUserID     *uint      `gorm:"index" json:"reviewer_user_id,omitempty"`
	ReviewNote         string     `gorm:"type:text" json:"review_note,omitempty"`
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type VoceChatContactCache struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	UserID            uint      `gorm:"not null;index;uniqueIndex:idx_voce_contact_pair" json:"user_id"`
	ContactUserID     uint      `gorm:"not null;index;uniqueIndex:idx_voce_contact_pair" json:"contact_user_id"`
	VoceChatUserID    string    `gorm:"type:varchar(191);index" json:"voce_chat_user_id"`
	ContactVoceChatID string    `gorm:"type:varchar(191);index" json:"contact_voce_chat_id"`
	Source            string    `gorm:"type:varchar(30);default:vocechat" json:"source"`
	SyncedAt          time.Time `gorm:"index" json:"synced_at"`
	ExpiresAt         time.Time `gorm:"index" json:"expires_at"`
	LastSyncStatus    string    `gorm:"type:varchar(30);default:ok;index" json:"last_sync_status"`
	LastSyncError     string    `gorm:"type:text" json:"-"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// 生成 Token 的工具函数
func GenerateToken(length int) string {
	b := make([]byte, length/2)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type Status struct {
	SysAdminID        uint         `json:"sys_admin_id"`
	Username          string       `json:"username"`
	Users             []UserStatus `json:"users"`
	TotalMessages     int          `json:"total_messages"`
	PersonalMessages  int          `json:"personal_messages"`
	TotalUsers        int          `json:"total_users"`
	TotalComments     int          `json:"total_comments"`
	TotalReplies      int          `json:"total_replies"`
	TotalGuestbook    int          `json:"total_guestbook"`
	ReceivedLikes     int          `json:"received_likes"`
	ReceivedComments  int          `json:"received_comments"`
	ReceivedReplies   int          `json:"received_replies"`
	ReceivedGuestbook int          `json:"received_guestbook"`
	AutoBanEnabled    *bool        `json:"auto_ban_enabled,omitempty"`
}

type UserSession struct {
	UserID    uint      `json:"user_id"`
	Username  string    `json:"username"`
	IsAdmin   bool      `json:"is_admin"`
	LoginTime time.Time `json:"login_time"`
}

type Setting struct {
	gorm.Model
	AllowRegistration       bool `gorm:"default:true"`
	AutoApproveRegistration bool `gorm:"default:false"`
}

type SiteConfig struct {
	gorm.Model
	SiteTitle        string `gorm:"type:varchar(100)"`
	SubtitleText     string `gorm:"type:varchar(191)"`
	AvatarURL        string `gorm:"type:varchar(191)"`
	Username         string `gorm:"type:varchar(50)"`
	Description      string `gorm:"type:varchar(191)"`
	Backgrounds      string `gorm:"type:text"`
	PageFooterHTML   string `gorm:"type:text"`
	RSSTitle         string `gorm:"type:varchar(100)"`
	RSSDescription   string `gorm:"type:varchar(191)"`
	RSSAuthorName    string `gorm:"type:varchar(50)"`
	RSSFaviconURL    string `gorm:"type:varchar(191)"`
	RSSEnabled       bool   `gorm:"default:false"`
	RSSMemberIDs     string `gorm:"type:text"`
	EnableGithubCard bool   `gorm:"default:false"`
	// 推送模块总开关（与具体推送渠道配置解耦）
	NotifyEnabled bool `gorm:"default:false"`
	// PWA 配置
	PwaEnabled     bool   `gorm:"default:true"`
	PwaTitle       string `gorm:"type:varchar(100)"`
	PwaDescription string `gorm:"type:varchar(191)"`
	PwaIconURL     string `gorm:"type:varchar(191)"`
	// 主题默认模式: dark 或 light
	ContentThemeDefault string `gorm:"type:varchar(10)"`
	HomeLayoutDefault   string `gorm:"type:varchar(10)"`
	AnnouncementText    string `gorm:"type:varchar(191)"`
	AnnouncementEnabled bool   `gorm:"default:true"`
	Version             int    `json:"version"`
	// RecycleBinRetentionDays controls automatic permanent deletion of notes.
	// Zero disables automatic cleanup; supported values are 7, 30, 90, 180 and 365.
	RecycleBinRetentionDays int    `gorm:"default:0" json:"recycleBinRetentionDays"`
	SmtpEnabled             bool   `gorm:"default:false" json:"smtpEnabled"`
	SmtpDriver              string `gorm:"type:varchar(50)" json:"smtpDriver"`
	SmtpHost                string `gorm:"type:varchar(191)" json:"smtpHost"`
	SmtpPort                int    `json:"smtpPort"`
	SmtpUser                string `gorm:"type:varchar(191)" json:"smtpUser"`
	SmtpPass                string `gorm:"type:varchar(191)" json:"smtpPass"`
	SmtpFrom                string `gorm:"type:varchar(191)" json:"smtpFrom"`
	SmtpEncryption          string `gorm:"type:varchar(20)" json:"smtpEncryption"`
	SmtpTLS                 bool   `gorm:"default:false" json:"smtpTLS"`
	// GitHub OAuth
	GithubOAuthEnabled bool   `gorm:"default:false"`
	GithubClientId     string `gorm:"type:varchar(191)"`
	GithubClientSecret string `gorm:"type:varchar(191)"`
	GithubCallbackURL  string `gorm:"type:varchar(191)"`
	// VoceChat 外挂配置
	VoceChatEnabled                  bool       `gorm:"default:false"`
	VoceChatBaseURL                  string     `gorm:"type:varchar(191)"`
	VoceChatAdminUsername            string     `gorm:"type:varchar(191)"`
	VoceChatAdminPassword            string     `gorm:"type:varchar(191)" json:"-"`
	VoceChatAdminToken               string     `gorm:"type:text" json:"-"`
	VoceChatThirdPartySecret         string     `gorm:"type:varchar(191)" json:"-"`
	VoceChatNotificationEnabled      bool       `gorm:"default:false;not null" json:"voceChatNotificationEnabled"`
	VoceChatBotAPIKey                string     `gorm:"type:text" json:"-"`
	VoceChatEmailDomain              string     `gorm:"type:varchar(100);default:vc.com"`
	VoceChatLoginVerificationEnabled bool       `gorm:"default:false"`
	VoceChatLocalFallbackEnabled     bool       `gorm:"default:false"`
	VoceChatContactsEnabled          bool       `gorm:"default:false"`
	VoceChatContactsCacheTTLSeconds  int        `gorm:"default:60"`
	VoceChatLastHealthStatus         string     `gorm:"type:varchar(30)"`
	VoceChatLastHealthError          string     `gorm:"type:text"`
	VoceChatLastHealthCheckAt        *time.Time `json:"voceChatLastHealthCheckAt"`
	// 云存储（S3/R2）备份设置
	StorageEnabled       bool   `gorm:"default:false"`
	StorageProvider      string `gorm:"type:varchar(20)"` // s3 或 r2
	StorageEndpoint      string `gorm:"type:varchar(191)"`
	StorageRegion        string `gorm:"type:varchar(100)"`
	StorageBucket        string `gorm:"type:varchar(191)"`
	StorageAccessKey     string `gorm:"type:varchar(191)"`
	StorageSecretKey     string `gorm:"type:varchar(191)"`
	StorageUsePathStyle  bool   `gorm:"default:true"`
	StoragePublicBaseURL string `gorm:"type:varchar(191)"`

	// 附件存储专用配置
	AttachmentStorageEnabled       bool   `gorm:"default:false"`
	AttachmentStorageProvider      string `gorm:"type:varchar(20)"` // s3 或 r2
	AttachmentStorageEndpoint      string `gorm:"type:varchar(191)"`
	AttachmentStorageRegion        string `gorm:"type:varchar(100)"`
	AttachmentStorageBucket        string `gorm:"type:varchar(191)"`
	AttachmentStorageAccessKey     string `gorm:"type:varchar(191)"`
	AttachmentStorageSecretKey     string `gorm:"type:varchar(191)"`
	AttachmentStorageUsePathStyle  bool   `gorm:"default:true"`
	AttachmentStoragePublicBaseURL string `gorm:"type:varchar(191)"`

	// 附件压缩配置
	EnableCompression bool `gorm:"default:false"`

	// 云同步角色：primary(主节点，执行上传) / secondary(备节点，不上传)
	StorageSyncRole string `gorm:"type:varchar(20)"`
	// 云存储自动同步
	StorageAutoSyncEnabled       bool       `gorm:"default:false"`
	StorageSyncMode              string     `gorm:"type:varchar(20)"` // instant 或 scheduled
	StorageSyncIntervalMinute    int        `gorm:"default:15"`
	StorageLastSyncTime          *time.Time `json:"storageLastSyncTime"`
	StorageSyncConfirmed         bool       `gorm:"default:false"`
	StorageSyncConfirmInstanceID string     `gorm:"type:varchar(191)"`
	StorageLastRemoteETag        string     `gorm:"type:varchar(191)"`
	StorageLastRemoteModified    *time.Time
	// 音乐播放器配置（NeteaseMiniPlayer）
	MusicEnabled          bool   `gorm:"default:false"`
	MusicPlaylistId       string `gorm:"type:varchar(50)"`
	MusicSongId           string `gorm:"type:varchar(50)"`
	MusicPosition         string `gorm:"type:varchar(30)"` // static/top-left/top-right/bottom-left/bottom-right
	MusicTheme            string `gorm:"type:varchar(20)"` // auto/light/dark
	MusicLyric            bool   `gorm:"default:true"`
	MusicAutoplay         bool   `gorm:"default:false"`
	MusicDefaultMinimized bool   `gorm:"default:true"`
	MusicEmbed            bool   `gorm:"default:false"`
	MusicHideOnMobile     bool   `gorm:"default:true"`
	MusicCssCdnURL        string `gorm:"type:varchar(255)"`
	MusicJsCdnURL         string `gorm:"type:varchar(255)"`
	// 评论系统配置
	CommentEnabled                bool   `gorm:"default:true"`
	CommentSystem                 string `gorm:"type:varchar(20)"` // legacy storage field; runtime is always builtin
	CommentEmailEnabled           bool   `gorm:"default:false"`
	CommentEmailAdminNotifyAll    bool   `gorm:"default:true;not null" json:"commentEmailAdminNotifyAll"`
	CommentLoginRequired          bool   `gorm:"default:true"`
	CommentEmailReplyName         string `gorm:"type:varchar(100)"`
	CommentEmailAdminPrefix       string `gorm:"type:varchar(50)"`
	CommentEmailReplyPrefix       string `gorm:"type:varchar(50)"`
	CommentEmailReplyTemplate     string `gorm:"type:text"`
	CommentEmailAdminTemplate     string `gorm:"type:text"`
	CommentEmailSiteURL           string `gorm:"type:varchar(191)"`
	CommentEmailReplyTemplateHTML string `gorm:"type:text"`
	CommentEmailAdminTemplateHTML string `gorm:"type:text"`
	// 扩展组件开关
	CalendarEnabled        bool   `gorm:"default:true"`
	TimeEnabled            bool   `gorm:"default:true"`
	HitokotoEnabled        bool   `gorm:"default:true"`
	LifeCountdownEnabled   bool   `gorm:"default:false"`
	LifeCountdownBirthDate string `gorm:"type:varchar(20)"`
	LifeExpectancyYears    int    `gorm:"default:0"`
	// 社交链接组件
	SocialLinksEnabled bool   `gorm:"default:true"`
	SocialLinks        string `gorm:"type:text"`
	// 系统欢迎组件（左栏头像卡片专用，脱离用户资料）
	WelcomeAvatarURL   string `gorm:"type:varchar(255)"`
	WelcomeName        string `gorm:"type:varchar(100)"`
	WelcomeDescription string `gorm:"type:varchar(255)"`
	WelcomeUseAdmin    bool   `gorm:"default:true"`
	// 广告位配置（左侧）
	LeftAdEnabled     bool   `gorm:"default:true"`
	LeftAds           string `gorm:"type:text"`
	LeftAdsIntervalMs int    `gorm:"default:4000"`
	// 关于/友链/留言等页面配置
	LinksTitle                  string `gorm:"type:varchar(100)"`
	LinksDescription            string `gorm:"type:varchar(191)"`
	CommentPageTitle            string `gorm:"type:varchar(100)"`
	CommentPageDescription      string `gorm:"type:varchar(191)"`
	NotificationPageTitle       string `gorm:"type:varchar(100)"`
	NotificationPageDescription string `gorm:"type:varchar(191)"`
	AnnouncementPageTitle       string `gorm:"type:varchar(100)"`
	AnnouncementPageDescription string `gorm:"type:varchar(191)"`
	AboutPageTitle              string `gorm:"type:varchar(100)"`
	AboutPageDescription        string `gorm:"type:varchar(191)"`
	AboutMarkdown               string `gorm:"type:text"`
	LoginExpireDays             int    `gorm:"default:3"`
	LoginExpireHours            int    `gorm:"default:0"`
	// 信息流聚合配置
	FeedEnabled            bool   `gorm:"default:false"`
	FeedPageTitle          string `gorm:"type:varchar(100)"`
	FeedPageDescription    string `gorm:"type:varchar(191)"`
	FeedSources            string `gorm:"type:text"`
	FeedLimit              int    `gorm:"default:100"`
	FeedRefreshSeconds     int    `gorm:"default:7200"`
	LinksApplyTitle        string `gorm:"type:varchar(100)"`
	LinksApplyText         string `gorm:"type:text"`
	FriendLinkEmailEnabled bool   `gorm:"default:false"`
}

type HeaderBackground struct {
	URL             string  `json:"url"`
	TitleColor      string  `json:"titleColor,omitempty"`
	TitleOpacity    float64 `json:"titleOpacity"`
	SubtitleColor   string  `json:"subtitleColor,omitempty"`
	SubtitleOpacity float64 `json:"subtitleOpacity"`
}

func normalizeHeaderBackgroundOpacity(value interface{}) float64 {
	if num, ok := value.(float64); ok {
		if num < 0 {
			return 0
		}
		if num > 1 {
			return 1
		}
		return num
	}
	return 1
}

func normalizeHeaderBackgroundFromMap(raw map[string]interface{}) (HeaderBackground, bool) {
	url, _ := raw["url"].(string)
	url = strings.TrimSpace(url)
	if url == "" {
		return HeaderBackground{}, false
	}
	titleColor, _ := raw["titleColor"].(string)
	subtitleColor, _ := raw["subtitleColor"].(string)
	return HeaderBackground{
		URL:             url,
		TitleColor:      strings.TrimSpace(titleColor),
		TitleOpacity:    normalizeHeaderBackgroundOpacity(raw["titleOpacity"]),
		SubtitleColor:   strings.TrimSpace(subtitleColor),
		SubtitleOpacity: normalizeHeaderBackgroundOpacity(raw["subtitleOpacity"]),
	}, true
}

func (s *SiteConfig) GetBackgroundsConfig() []HeaderBackground {
	if s.Backgrounds == "" {
		return []HeaderBackground{}
	}

	var legacy []string
	if err := json.Unmarshal([]byte(s.Backgrounds), &legacy); err == nil {
		backgrounds := make([]HeaderBackground, 0, len(legacy))
		for _, url := range legacy {
			url = strings.TrimSpace(url)
			if url != "" {
				backgrounds = append(backgrounds, HeaderBackground{URL: url, TitleOpacity: 1, SubtitleOpacity: 1})
			}
		}
		return backgrounds
	}

	var rawBackgrounds []map[string]interface{}
	if err := json.Unmarshal([]byte(s.Backgrounds), &rawBackgrounds); err != nil {
		return []HeaderBackground{}
	}
	backgrounds := make([]HeaderBackground, 0, len(rawBackgrounds))
	for _, raw := range rawBackgrounds {
		if bg, ok := normalizeHeaderBackgroundFromMap(raw); ok {
			backgrounds = append(backgrounds, bg)
		}
	}
	return backgrounds
}

func (s *SiteConfig) GetBackgroundsList() []string {
	backgrounds := s.GetBackgroundsConfig()
	urls := make([]string, 0, len(backgrounds))
	for _, bg := range backgrounds {
		urls = append(urls, bg.URL)
	}
	return urls
}
func UpdateMessage(id string, content string) error {
	// 先查询消息是否存在
	var message Message
	result := DB.First(&message, id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return errors.New("消息不存在")
		}
		return result.Error
	}

	// 更新消息内容
	result = DB.Model(&message).Updates(map[string]interface{}{
		"content": content,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("更新失败")
	}

	return nil
}

type UserLifeCountdownConfig struct {
	gorm.Model
	UserID              uint   `json:"user_id" gorm:"uniqueIndex;not null"`
	Enabled             bool   `json:"lifeCountdownEnabled" gorm:"default:false"`
	BirthDate           string `json:"lifeCountdownBirthDate"`
	LifeExpectancyYears int    `json:"lifeExpectancyYears" gorm:"default:0"`
}

type UserFrontendPreference struct {
	gorm.Model
	UserID          uint  `json:"user_id" gorm:"uniqueIndex;not null"`
	HitokotoEnabled *bool `json:"hitokotoEnabled"`
}

type FriendLink struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Title       string    `gorm:"type:varchar(100)" json:"title"`
	Link        string    `gorm:"type:varchar(255)" json:"link"`
	Icon        string    `gorm:"type:varchar(100)" json:"icon"`
	Description string    `gorm:"type:varchar(255)" json:"description"`
	Email       string    `gorm:"type:varchar(191)" json:"email"`
	CreatedAt   time.Time `json:"created_at"`
}
type FriendLinkApply struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Title       string    `gorm:"type:varchar(100)" json:"title"`
	Link        string    `gorm:"type:varchar(255);index" json:"link"`
	Icon        string    `gorm:"type:varchar(100)" json:"icon"`
	Description string    `gorm:"type:varchar(255)" json:"description"`
	Email       string    `gorm:"type:varchar(191)" json:"email"`
	Status      string    `gorm:"type:varchar(20);index" json:"status"`
	Feedback    string    `gorm:"type:text" json:"feedback"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
