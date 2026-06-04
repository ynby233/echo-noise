package vocechat

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	PlainPasswordKindUser        = "user"
	PlainPasswordKindApplication = "application"

	plainPasswordStoreEnv  = "NOISE_PLAIN_PASSWORD_STORE"
	plainPasswordStorePath = "/app/data/plain-passwords.db"
)

type PlainPasswordRecord struct {
	Key            string    `json:"key" gorm:"primaryKey;size:128"`
	Kind           string    `json:"kind" gorm:"index;size:32;not null"`
	UserID         uint      `json:"user_id,omitempty" gorm:"index"`
	ApplicationID  string    `json:"application_id,omitempty" gorm:"index;size:64"`
	Username       string    `json:"username" gorm:"size:255;not null"`
	Password       string    `json:"password" gorm:"not null"`
	VoceChatEmail  string    `json:"voce_chat_email,omitempty" gorm:"size:255"`
	VoceChatUserID string    `json:"voce_chat_user_id,omitempty" gorm:"size:64"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (PlainPasswordRecord) TableName() string {
	return "plain_password_records"
}

type PlainPasswordStore struct {
	path string
	mu   sync.Mutex
}

func DefaultPlainPasswordStorePath() string {
	if path := strings.TrimSpace(os.Getenv(plainPasswordStoreEnv)); path != "" {
		return path
	}
	return plainPasswordStorePath
}

func NewPlainPasswordStore(path string) *PlainPasswordStore {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultPlainPasswordStorePath()
	}
	return &PlainPasswordStore{path: path}
}

func DefaultPlainPasswordStore() *PlainPasswordStore {
	return NewPlainPasswordStore("")
}

func (s *PlainPasswordStore) UpsertUserPassword(userID uint, username, password, voceChatEmail, voceChatUserID string) error {
	if userID == 0 {
		return errors.New("用户ID不能为空")
	}
	return s.upsert(PlainPasswordRecord{
		Key:            userRecordKey(userID),
		Kind:           PlainPasswordKindUser,
		UserID:         userID,
		Username:       strings.TrimSpace(username),
		Password:       password,
		VoceChatEmail:  strings.TrimSpace(voceChatEmail),
		VoceChatUserID: strings.TrimSpace(voceChatUserID),
	})
}

func (s *PlainPasswordStore) UpsertApplicationPassword(applicationID, username, password, voceChatEmail, voceChatUserID string) error {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return errors.New("申请ID不能为空")
	}
	return s.upsert(PlainPasswordRecord{
		Key:            applicationRecordKey(applicationID),
		Kind:           PlainPasswordKindApplication,
		ApplicationID:  applicationID,
		Username:       strings.TrimSpace(username),
		Password:       password,
		VoceChatEmail:  strings.TrimSpace(voceChatEmail),
		VoceChatUserID: strings.TrimSpace(voceChatUserID),
	})
}

func (s *PlainPasswordStore) GetUserPassword(userID uint) (PlainPasswordRecord, bool, error) {
	return s.get(userRecordKey(userID))
}

func (s *PlainPasswordStore) GetApplicationPassword(applicationID string) (PlainPasswordRecord, bool, error) {
	return s.get(applicationRecordKey(applicationID))
}

func (s *PlainPasswordStore) DeleteUserPassword(userID uint) error {
	return s.delete(userRecordKey(userID))
}

func (s *PlainPasswordStore) DeleteApplicationPassword(applicationID string) error {
	return s.delete(applicationRecordKey(applicationID))
}

func (s *PlainPasswordStore) Path() string {
	return s.path
}

func (s *PlainPasswordStore) upsert(record PlainPasswordRecord) error {
	if strings.TrimSpace(record.Password) == "" {
		return errors.New("明文密码不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.openLocked()
	if err != nil {
		return err
	}
	defer closeDatabase(db)

	record.UpdatedAt = time.Now().UTC()
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		UpdateAll: true,
	}).Create(&record).Error; err != nil {
		return fmt.Errorf("写入明文密码库失败: %w", err)
	}
	return s.ensurePermissionsLocked()
}

func (s *PlainPasswordStore) get(key string) (PlainPasswordRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.openLocked()
	if err != nil {
		return PlainPasswordRecord{}, false, err
	}
	defer closeDatabase(db)

	var record PlainPasswordRecord
	if err := db.First(&record, "key = ?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return PlainPasswordRecord{}, false, nil
		}
		return PlainPasswordRecord{}, false, fmt.Errorf("读取明文密码库失败: %w", err)
	}
	return record, true, nil
}

func (s *PlainPasswordStore) delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.openLocked()
	if err != nil {
		return err
	}
	defer closeDatabase(db)
	if err := db.Delete(&PlainPasswordRecord{}, "key = ?", key).Error; err != nil {
		return fmt.Errorf("删除明文密码记录失败: %w", err)
	}
	return s.ensurePermissionsLocked()
}

func (s *PlainPasswordStore) openLocked() (*gorm.DB, error) {
	if strings.TrimSpace(s.path) == "" {
		return nil, errors.New("明文密码库路径不能为空")
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("创建明文密码库目录失败: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("创建明文密码库文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("关闭明文密码库文件失败: %w", err)
	}
	if err := s.ensurePermissionsLocked(); err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(s.path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("打开明文密码库失败: %w", err)
	}
	if err := db.AutoMigrate(&PlainPasswordRecord{}); err != nil {
		return nil, fmt.Errorf("迁移明文密码库失败: %w", err)
	}
	return db, nil
}

func (s *PlainPasswordStore) ensurePermissionsLocked() error {
	if err := os.Chmod(s.path, 0600); err != nil {
		return fmt.Errorf("设置明文密码库权限失败: %w", err)
	}
	return nil
}

func closeDatabase(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func userRecordKey(userID uint) string {
	return fmt.Sprintf("user:%d", userID)
}

func applicationRecordKey(applicationID string) string {
	return "application:" + strings.TrimSpace(applicationID)
}
