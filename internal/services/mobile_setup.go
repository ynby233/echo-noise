package services

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

type MobileSetupState string

const (
	MobileSetupRequired MobileSetupState = "required"
	MobileSetupReady    MobileSetupState = "ready"
	MobileSetupInvalid  MobileSetupState = "invalid"
)

var (
	ErrMobileSetupAlreadyCompleted   = errors.New("站点初始化已经完成")
	ErrMobileSetupInvalidDatabase    = errors.New("数据库包含用户数据，但缺少有效的 1 号管理员；为防止误提升权限，初始化已锁定")
	ErrMobileSetupDefaultCredentials = errors.New("不能使用默认的 admin/admin 凭据")
	mobileSetupMu                    sync.Mutex
)

// MobileSetupStateForDB derives setup completion only from the durable owner
// invariant. No separate flag can drift away from the user table.
func MobileSetupStateForDB(db *gorm.DB) (MobileSetupState, error) {
	if db == nil {
		return MobileSetupInvalid, errors.New("数据库未初始化")
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		return MobileSetupInvalid, fmt.Errorf("读取初始化状态失败: %w", err)
	}
	if userCount == 0 {
		return MobileSetupRequired, nil
	}
	if _, err := loadPrimaryAdmin(db); err == nil {
		return MobileSetupReady, nil
	} else if !errors.Is(err, errPrimaryAdminUnavailable) {
		return MobileSetupInvalid, err
	}
	return MobileSetupInvalid, nil
}

// InitializeMobileSiteOwner creates the explicit ID 1 administrator and all
// first-owner dependent content in one transaction. It never repairs a
// nonempty invalid database by promoting an existing account.
func InitializeMobileSiteOwner(db *gorm.DB, username, password string) (*models.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, errors.New(models.UsernameOrPasswordCannotBeEmptyMessage)
	}
	if err := validateRegistrationUsername(username); err != nil {
		return nil, err
	}
	if strings.EqualFold(username, "admin") && password == "admin" {
		return nil, ErrMobileSetupDefaultCredentials
	}
	hashed := models.HashPassword(password)
	if hashed == "" {
		return nil, errors.New("密码加密失败")
	}

	mobileSetupMu.Lock()
	defer mobileSetupMu.Unlock()

	var created models.User
	err := db.Transaction(func(tx *gorm.DB) error {
		state, err := MobileSetupStateForDB(tx)
		if err != nil {
			return err
		}
		switch state {
		case MobileSetupReady:
			return ErrMobileSetupAlreadyCompleted
		case MobileSetupInvalid:
			return ErrMobileSetupInvalidDatabase
		case MobileSetupRequired:
		default:
			return fmt.Errorf("未知初始化状态: %s", state)
		}

		created = models.User{
			ID:            models.PrimaryAdminUserID,
			Username:      username,
			Password:      hashed,
			IsAdmin:       true,
			Token:         models.GenerateToken(32),
			Description:   "欢迎访问",
			AvatarURL:     neutralAvatarURL,
			EmailVerified: true,
		}
		if err := tx.Create(&created).Error; err != nil {
			return fmt.Errorf("创建 1 号管理员失败: %w", err)
		}

		guestbookEnsureMu.Lock()
		defer guestbookEnsureMu.Unlock()
		if _, err := ensureGuestbookInTransaction(tx); err != nil {
			return fmt.Errorf("创建留言板失败: %w", err)
		}
		if err := seedPrimaryAdminExampleMessages(tx, &created, "欢迎来到你的个人站点！"); err != nil {
			return fmt.Errorf("创建示例内容失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}
