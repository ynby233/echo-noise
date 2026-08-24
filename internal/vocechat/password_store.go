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
	Key                            string     `json:"key" gorm:"primaryKey;size:128"`
	Kind                           string     `json:"kind" gorm:"index;size:32;not null"`
	UserID                         uint       `json:"user_id,omitempty" gorm:"index"`
	ApplicationID                  string     `json:"application_id,omitempty" gorm:"index;size:64"`
	Username                       string     `json:"username" gorm:"size:255;not null"`
	Password                       string     `json:"password,omitempty" gorm:"not null"`
	VoceChatPassword               string     `json:"voce_chat_password,omitempty"`
	VoceChatPasswordUpdatedAt      *time.Time `json:"voce_chat_password_updated_at,omitempty"`
	LocalFallbackPassword          string     `json:"local_fallback_password,omitempty"`
	LocalFallbackPasswordUpdatedAt *time.Time `json:"local_fallback_password_updated_at,omitempty"`
	VoceChatEmail                  string     `json:"voce_chat_email,omitempty" gorm:"size:255"`
	VoceChatUserID                 string     `json:"voce_chat_user_id,omitempty" gorm:"size:64"`
	UpdatedAt                      time.Time  `json:"updated_at"`
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

func (r PlainPasswordRecord) VoceChatPasswordValue() string {
	if strings.TrimSpace(r.VoceChatPassword) != "" {
		return r.VoceChatPassword
	}
	return r.Password
}

func (r PlainPasswordRecord) LocalFallbackPasswordValue() string {
	if strings.TrimSpace(r.LocalFallbackPassword) != "" {
		return r.LocalFallbackPassword
	}
	return ""
}

func (r PlainPasswordRecord) HasAnyPassword() bool {
	return strings.TrimSpace(r.VoceChatPasswordValue()) != "" || strings.TrimSpace(r.LocalFallbackPasswordValue()) != ""
}

func (s *PlainPasswordStore) UpsertUserPassword(userID uint, username, password, voceChatEmail, voceChatUserID string) error {
	return s.UpsertUserVoceChatPassword(userID, username, password, voceChatEmail, voceChatUserID)
}

func (s *PlainPasswordStore) UpsertUserVoceChatPassword(userID uint, username, password, voceChatEmail, voceChatUserID string) error {
	if userID == 0 {
		return errors.New("用户ID不能为空")
	}
	return s.upsert(plainPasswordUpdate{
		key:              userRecordKey(userID),
		kind:             PlainPasswordKindUser,
		userID:           userID,
		username:         strings.TrimSpace(username),
		voceChatEmail:    strings.TrimSpace(voceChatEmail),
		voceChatUserID:   strings.TrimSpace(voceChatUserID),
		voceChatPassword: stringPtr(password),
	})
}

func (s *PlainPasswordStore) UpsertUserLocalFallbackPassword(userID uint, username, password, voceChatEmail, voceChatUserID string) error {
	if userID == 0 {
		return errors.New("用户ID不能为空")
	}
	return s.upsert(plainPasswordUpdate{
		key:                   userRecordKey(userID),
		kind:                  PlainPasswordKindUser,
		userID:                userID,
		username:              strings.TrimSpace(username),
		voceChatEmail:         strings.TrimSpace(voceChatEmail),
		voceChatUserID:        strings.TrimSpace(voceChatUserID),
		localFallbackPassword: stringPtr(password),
	})
}

func (s *PlainPasswordStore) UpsertUserPasswordMetadata(userID uint, username, voceChatEmail, voceChatUserID string) error {
	if userID == 0 {
		return errors.New("用户ID不能为空")
	}
	return s.upsert(plainPasswordUpdate{
		key:            userRecordKey(userID),
		kind:           PlainPasswordKindUser,
		userID:         userID,
		username:       strings.TrimSpace(username),
		voceChatEmail:  strings.TrimSpace(voceChatEmail),
		voceChatUserID: strings.TrimSpace(voceChatUserID),
	})
}

func (s *PlainPasswordStore) UpsertApplicationPassword(applicationID, username, password, voceChatEmail, voceChatUserID string) error {
	return s.UpsertApplicationVoceChatPassword(applicationID, username, password, voceChatEmail, voceChatUserID)
}

func (s *PlainPasswordStore) UpsertApplicationVoceChatPassword(applicationID, username, password, voceChatEmail, voceChatUserID string) error {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return errors.New("申请ID不能为空")
	}
	return s.upsert(plainPasswordUpdate{
		key:              applicationRecordKey(applicationID),
		kind:             PlainPasswordKindApplication,
		applicationID:    applicationID,
		username:         strings.TrimSpace(username),
		voceChatEmail:    strings.TrimSpace(voceChatEmail),
		voceChatUserID:   strings.TrimSpace(voceChatUserID),
		voceChatPassword: stringPtr(password),
	})
}

func (s *PlainPasswordStore) UpsertApplicationLocalFallbackPassword(applicationID, username, password, candidateEmail string) error {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return errors.New("申请ID不能为空")
	}
	return s.upsert(plainPasswordUpdate{
		key:                   applicationRecordKey(applicationID),
		kind:                  PlainPasswordKindApplication,
		applicationID:         applicationID,
		username:              strings.TrimSpace(username),
		voceChatEmail:         strings.TrimSpace(candidateEmail),
		localFallbackPassword: stringPtr(password),
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

// RestoreUserPasswordSnapshot restores a previously read user record exactly, or
// removes a record that did not exist when the snapshot was taken.
func (s *PlainPasswordStore) RestoreUserPasswordSnapshot(record PlainPasswordRecord, existed bool) error {
	if record.UserID == 0 {
		return errors.New("用户ID不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.openLocked()
	if err != nil {
		return err
	}
	defer closeDatabase(db)

	if !existed {
		if err := db.Delete(&PlainPasswordRecord{}, "key = ?", userRecordKey(record.UserID)).Error; err != nil {
			return fmt.Errorf("恢复用户密码记录失败: %w", err)
		}
		return s.ensurePermissionsLocked()
	}

	record.Key = userRecordKey(record.UserID)
	record.Kind = PlainPasswordKindUser
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		UpdateAll: true,
	}).Create(&record).Error; err != nil {
		return fmt.Errorf("恢复用户密码记录失败: %w", err)
	}
	return s.ensurePermissionsLocked()
}

func (s *PlainPasswordStore) DeleteApplicationPassword(applicationID string) error {
	return s.delete(applicationRecordKey(applicationID))
}

func (s *PlainPasswordStore) Path() string {
	return s.path
}

type plainPasswordUpdate struct {
	key                   string
	kind                  string
	userID                uint
	applicationID         string
	username              string
	voceChatEmail         string
	voceChatUserID        string
	voceChatPassword      *string
	localFallbackPassword *string
}

func stringPtr(value string) *string {
	return &value
}

func (s *PlainPasswordStore) upsert(update plainPasswordUpdate) error {
	if update.voceChatPassword != nil && strings.TrimSpace(*update.voceChatPassword) == "" {
		return errors.New("VoceChat 明文密码不能为空")
	}
	if update.localFallbackPassword != nil && strings.TrimSpace(*update.localFallbackPassword) == "" {
		return errors.New("本地备用明文密码不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.openLocked()
	if err != nil {
		return err
	}
	defer closeDatabase(db)

	now := time.Now().UTC()
	record := PlainPasswordRecord{}
	if err := db.First(&record, "key = ?", update.key).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("读取明文密码库失败: %w", err)
		}
		record = PlainPasswordRecord{Key: update.key}
	} else {
		backfillLegacyPasswords(&record)
	}

	record.Key = update.key
	record.Kind = update.kind
	record.UserID = update.userID
	record.ApplicationID = strings.TrimSpace(update.applicationID)
	record.Username = strings.TrimSpace(update.username)
	record.VoceChatEmail = strings.TrimSpace(update.voceChatEmail)
	record.VoceChatUserID = strings.TrimSpace(update.voceChatUserID)
	if update.voceChatPassword != nil {
		record.VoceChatPassword = *update.voceChatPassword
		updatedAt := now
		record.VoceChatPasswordUpdatedAt = &updatedAt
	}
	if update.localFallbackPassword != nil {
		record.LocalFallbackPassword = *update.localFallbackPassword
		updatedAt := now
		record.LocalFallbackPasswordUpdatedAt = &updatedAt
	}
	record.Password = ""
	record.UpdatedAt = now

	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		UpdateAll: true,
	}).Create(&record).Error; err != nil {
		return fmt.Errorf("写入明文密码库失败: %w", err)
	}
	return s.ensurePermissionsLocked()
}

func backfillLegacyPasswords(record *PlainPasswordRecord) {
	if record == nil || strings.TrimSpace(record.Password) == "" {
		return
	}
	legacyUpdatedAt := record.UpdatedAt
	if strings.TrimSpace(record.VoceChatPassword) == "" {
		record.VoceChatPassword = record.Password
		if record.VoceChatPasswordUpdatedAt == nil && !legacyUpdatedAt.IsZero() {
			updatedAt := legacyUpdatedAt
			record.VoceChatPasswordUpdatedAt = &updatedAt
		}
	}
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
	backfillLegacyPasswords(&record)
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
