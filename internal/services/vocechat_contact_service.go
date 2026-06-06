package services

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

type voceChatListContactsFunc func(ctx context.Context, config vocechat.Config, apiKey string) ([]vocechat.UserContact, error)

var voceChatListContacts voceChatListContactsFunc = defaultVoceChatListContacts

func defaultVoceChatListContacts(ctx context.Context, config vocechat.Config, apiKey string) ([]vocechat.UserContact, error) {
	client, err := vocechat.NewClient(config)
	if err != nil {
		return nil, err
	}
	return client.ListContacts(ctx, apiKey)
}

func voceChatContactCacheTTL(config vocechat.Config) time.Duration {
	seconds := config.ContactsCacheTTLSeconds
	if seconds <= 0 {
		seconds = vocechat.DefaultContactsCacheTTLSeconds
	}
	return time.Duration(seconds) * time.Second
}

func markVoceChatContactCacheFailure(user *models.User, ttl time.Duration, syncErr error) {
	if user == nil || user.ID == 0 || database.DB == nil {
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	_ = repository.MarkVoceChatContactsSyncFailure(user.ID, user.VoceChatUserID, syncErr, now, expiresAt)
}

func EnsureVoceChatContactCacheForAuthor(authorID uint) error {
	if authorID == 0 || database.DB == nil {
		return nil
	}
	config, err := loadVoceChatSiteConfig()
	if err != nil {
		return err
	}
	if !config.Enabled || !config.ContactsEnabled {
		return nil
	}
	ttl := voceChatContactCacheTTL(config)
	now := time.Now().UTC()
	if fresh, err := repository.HasFreshVoceChatContactSyncRecord(authorID, now); err != nil {
		return err
	} else if fresh {
		return nil
	}

	var author models.User
	if err := database.DB.First(&author, authorID).Error; err != nil || author.ID == 0 {
		return nil
	}
	if !config.IsReady() {
		markVoceChatContactCacheFailure(&author, ttl, errors.New("VoceChat 未配置完成"))
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	contactOwner, token, err := loginVoceChatContactOwner(ctx, config, &author)
	if err != nil {
		markVoceChatContactCacheFailure(&contactOwner, ttl, err)
		return nil
	}
	contacts, err := voceChatListContacts(ctx, config, token)
	if err != nil {
		markVoceChatContactCacheFailure(&contactOwner, ttl, err)
		return nil
	}

	return replaceVoceChatContactCacheFromRemote(&contactOwner, contacts, now, now.Add(ttl))
}

func loginVoceChatContactOwner(ctx context.Context, config vocechat.Config, author *models.User) (models.User, string, error) {
	owner := models.User{}
	if author != nil {
		owner = *author
	}
	if author == nil || author.ID == 0 {
		return owner, "", errors.New("作者不存在")
	}

	if author.ID == 1 && author.IsAdmin {
		email := strings.TrimSpace(config.AdminUsername)
		password := strings.TrimSpace(config.AdminPassword)
		if email == "" || password == "" {
			return owner, "", errors.New("VoceChat 管理员邮箱或密码未配置完整")
		}
		login, err := voceChatPasswordLogin(ctx, config, email, password)
		if err != nil {
			return owner, "", err
		}
		if login == nil || strings.TrimSpace(login.Token) == "" {
			return owner, "", errors.New("VoceChat 联系人令牌为空")
		}
		if !login.User.IsAdmin {
			return owner, "", errors.New("VoceChat 管理员邮箱对应账号不是管理员")
		}
		if login.User.UID <= 0 {
			return owner, "", errors.New("VoceChat 管理员账号缺少用户 ID")
		}
		if loginEmail := strings.TrimSpace(login.User.Email); loginEmail != "" {
			owner.VoceChatEmail = loginEmail
		} else {
			owner.VoceChatEmail = email
		}
		owner.VoceChatUserID = strconv.FormatInt(login.User.UID, 10)
		return owner, strings.TrimSpace(login.Token), nil
	}

	if strings.TrimSpace(author.VoceChatEmail) == "" || strings.TrimSpace(author.VoceChatUserID) == "" {
		return owner, "", errors.New("作者未绑定 VoceChat")
	}
	record, ok, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(author.ID)
	if err != nil || !ok || strings.TrimSpace(record.Password) == "" {
		if err == nil {
			err = errors.New("作者 VoceChat 密码不可用")
		}
		return owner, "", err
	}
	login, err := voceChatPasswordLogin(ctx, config, strings.TrimSpace(author.VoceChatEmail), record.Password)
	if err != nil {
		return owner, "", err
	}
	if login == nil || strings.TrimSpace(login.Token) == "" {
		return owner, "", errors.New("VoceChat 联系人令牌为空")
	}
	return owner, strings.TrimSpace(login.Token), nil
}

func replaceVoceChatContactCacheFromRemote(author *models.User, contacts []vocechat.UserContact, syncedAt time.Time, expiresAt time.Time) error {
	if author == nil || author.ID == 0 {
		return nil
	}
	vcIDs := make([]string, 0, len(contacts))
	seen := make(map[string]struct{}, len(contacts))
	for _, contact := range contacts {
		uid := contact.TargetUID
		if uid <= 0 {
			uid = contact.TargetInfo.UID
		}
		if uid <= 0 {
			continue
		}
		vcID := strconv.FormatInt(uid, 10)
		if _, ok := seen[vcID]; ok {
			continue
		}
		seen[vcID] = struct{}{}
		vcIDs = append(vcIDs, vcID)
	}

	usersByVoceID := make(map[string]models.User, len(vcIDs))
	if len(vcIDs) > 0 {
		var users []models.User
		if err := database.DB.Where("voce_chat_user_id IN ?", vcIDs).Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			if id := strings.TrimSpace(user.VoceChatUserID); id != "" {
				usersByVoceID[id] = user
			}
		}
	}

	rows := make([]models.VoceChatContactCache, 0, len(usersByVoceID)+1)
	rows = append(rows, models.VoceChatContactCache{
		UserID:         author.ID,
		ContactUserID:  0,
		VoceChatUserID: strings.TrimSpace(author.VoceChatUserID),
		Source:         "vocechat",
		SyncedAt:       syncedAt,
		ExpiresAt:      expiresAt,
		LastSyncStatus: models.VoceChatContactSyncStatusOK,
	})
	for _, vcID := range vcIDs {
		user, ok := usersByVoceID[vcID]
		if !ok || user.ID == 0 || user.ID == author.ID {
			continue
		}
		rows = append(rows, models.VoceChatContactCache{
			UserID:            author.ID,
			ContactUserID:     user.ID,
			VoceChatUserID:    strings.TrimSpace(author.VoceChatUserID),
			ContactVoceChatID: vcID,
			Source:            "vocechat",
			SyncedAt:          syncedAt,
			ExpiresAt:         expiresAt,
			LastSyncStatus:    models.VoceChatContactSyncStatusOK,
		})
	}
	return repository.ReplaceVoceChatContacts(author.ID, rows)
}

func EnsureVoceChatContactCachesForViewer(userID *uint, isAdmin bool) {
	if isAdmin || userID == nil || *userID == 0 || database.DB == nil {
		return
	}
	config, err := loadVoceChatSiteConfig()
	if err != nil || !config.Enabled || !config.ContactsEnabled {
		return
	}
	var authorIDs []uint
	if err := database.DB.Model(&models.Message{}).
		Where("visibility = ? AND user_id <> ?", MessageVisibilityContacts, *userID).
		Distinct("user_id").
		Pluck("user_id", &authorIDs).Error; err != nil {
		return
	}
	for _, authorID := range authorIDs {
		_ = EnsureVoceChatContactCacheForAuthor(authorID)
	}
}
