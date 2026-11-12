package backup

import (
	"errors"
	"fmt"
	"os"

	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func createEmptyPasswordStore(destination string) error {
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	db, err := OpenOffline(destination, true)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&vocechat.PlainPasswordRecord{}); err != nil {
		return err
	}
	return db.Exec("PRAGMA journal_mode=DELETE").Error
}

func requireNoPasswordDependencies(databasePath string) error {
	db, err := OpenOffline(databasePath, false)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	var count int64
	if db.Migrator().HasTable(&models.RegistrationApplication{}) {
		if err := db.Model(&models.RegistrationApplication{}).Where("status = ?", models.RegistrationApplicationStatusPending).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("独立密码库缺失，但存在待审批注册申请；请先修复密码库后再备份")
		}
	}
	if db.Migrator().HasColumn(&models.User{}, "voce_chat_sync_status") {
		if err := db.Model(&models.User{}).Where("coalesce(voce_chat_user_id, '') <> '' OR voce_chat_sync_status IN ?", []string{"pending", "provisioning", "failed"}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("独立密码库缺失，但存在关联或待同步的 VoceChat 账户；请先修复密码库后再备份")
		}
	}
	return nil
}

// The snapshots may be taken at different instants. Detect mismatched local
// fallback credentials and pending applications instead of restoring stale secrets.
func validatePasswordPair(databasePath, passwordPath string) error {
	db, err := OpenOffline(databasePath, false)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	passwordDB, err := OpenOffline(passwordPath, false)
	if err != nil {
		return err
	}
	passwordSQL, err := passwordDB.DB()
	if err != nil {
		return err
	}
	defer passwordSQL.Close()
	var records []vocechat.PlainPasswordRecord
	if err := passwordDB.Find(&records).Error; err != nil {
		return err
	}
	byApplication := map[string]vocechat.PlainPasswordRecord{}
	for _, record := range records {
		switch record.Kind {
		case vocechat.PlainPasswordKindUser:
			if !db.Migrator().HasTable(&models.User{}) {
				continue
			}
			var user models.User
			if err := db.First(&user, record.UserID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return fmt.Errorf("查询密码库对应账户失败: %w", err)
			}
			if user.Username != record.Username {
				return errors.New("密码库账户名称与主数据库不一致")
			}
			if record.LocalFallbackPassword != "" && bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(record.LocalFallbackPassword)) != nil {
				return errors.New("主数据库与独立密码库的本地密码不一致，请停止改密后重新备份")
			}
		case vocechat.PlainPasswordKindApplication:
			byApplication[record.ApplicationID] = record
		default:
			return errors.New("独立密码库包含未知记录类型")
		}
	}
	if db.Migrator().HasTable(&models.RegistrationApplication{}) {
		var pending []models.RegistrationApplication
		if err := db.Where("status = ?", models.RegistrationApplicationStatusPending).Find(&pending).Error; err != nil {
			return err
		}
		for _, application := range pending {
			record, ok := byApplication[application.ApplicationID]
			if !ok || record.Username != application.Username {
				return fmt.Errorf("注册申请 %s 的独立密码记录缺失或不一致", application.ApplicationID)
			}
			matched := false
			for _, candidate := range []string{record.LocalFallbackPasswordValue(), record.VoceChatPasswordValue()} {
				if candidate != "" && bcrypt.CompareHashAndPassword([]byte(application.PasswordHash), []byte(candidate)) == nil {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("注册申请 %s 的独立密码记录缺失或不一致", application.ApplicationID)
			}
		}
	}
	return nil
}
