package updates

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TaskPending        = "pending"
	TaskClaimed        = "claimed"
	TaskDownloading    = "downloading"
	TaskStopping       = "stopping"
	TaskBackingUp      = "backing_up"
	TaskReplacing      = "replacing"
	TaskVerifying      = "verifying"
	TaskSucceeded      = "succeeded"
	TaskFailed         = "failed"
	TaskNeedsAttention = "needs_attention"
)

const officialUpdateImage = "ghcr.io/ynby233/echo-noise"

var (
	ErrExecutorNotConfigured = errors.New("update executor is not configured")
	ErrInvalidTarget         = errors.New("invalid update target")
	ErrNoPendingTask         = errors.New("no pending update task")
	ErrTaskNotFound          = errors.New("update task not found")
	ErrTaskOwnership         = errors.New("update task belongs to another executor")
	ErrInvalidTransition     = errors.New("invalid update task transition")
	ErrCredentialInvalid     = errors.New("invalid executor credential")
)

type Target struct {
	Channel  string
	Image    string
	Digest   string
	Revision string
	Version  string
}

type TaskService struct{ db *gorm.DB }

func NewTaskService(db *gorm.DB) *TaskService { return &TaskService{db: db} }

func (s *TaskService) Create(actorID uint, target Target) (models.UpdateTask, bool, error) {
	target.Channel = strings.ToLower(strings.TrimSpace(target.Channel))
	target.Image = strings.TrimSpace(target.Image)
	target.Digest = strings.ToLower(strings.TrimSpace(target.Digest))
	target.Revision = strings.ToLower(strings.TrimSpace(target.Revision))
	if actorID != models.PrimaryAdminUserID || (target.Channel != "stable" && target.Channel != "edge") || target.Image != officialUpdateImage || !validDigest(target.Digest) || !validRevision(target.Revision) {
		return models.UpdateTask{}, false, ErrInvalidTarget
	}
	var task models.UpdateTask
	created := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("active_slot = ?", 1).First(&task).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var configured int64
		if err := tx.Model(&models.UpdateExecutorCredential{}).Where("revoked_at IS NULL AND superseded_at IS NULL AND last_seen_at IS NOT NULL AND (expires_at IS NULL OR expires_at > ?)", time.Now().UTC()).Count(&configured).Error; err != nil {
			return err
		}
		if configured == 0 {
			return ErrExecutorNotConfigured
		}
		publicID, err := randomHex(16)
		if err != nil {
			return err
		}
		slot := uint(1)
		task = models.UpdateTask{PublicID: publicID, RequestedByUserID: actorID, Channel: target.Channel, TargetImage: target.Image, TargetDigest: target.Digest, TargetRevision: target.Revision, TargetVersion: strings.TrimSpace(target.Version), Status: TaskPending, ActiveSlot: &slot}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&task)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return tx.Where("active_slot = ?", 1).First(&task).Error
		}
		created = true
		return nil
	})
	return task, created, err
}

func (s *TaskService) Get(publicID string) (models.UpdateTask, error) {
	var task models.UpdateTask
	if err := s.db.Where("public_id = ?", strings.TrimSpace(publicID)).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return task, ErrTaskNotFound
		}
		return task, err
	}
	return task, nil
}

// Claim returns the already assigned task first, making a lost HTTP response
// safe to retry without ever assigning the work twice.
func (s *TaskService) Claim(credentialID uint) (*models.UpdateTask, error) {
	var task models.UpdateTask
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("executor_credential_id = ? AND active_slot = ?", credentialID, 1).First(&task).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var credential models.UpdateExecutorCredential
		if err := tx.Where("id = ? AND revoked_at IS NULL AND superseded_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", credentialID, time.Now().UTC()).First(&credential).Error; err != nil {
			return ErrCredentialInvalid
		}
		if err := tx.Where("status = ? AND active_slot = ?", TaskPending, 1).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoPendingTask
			}
			return err
		}
		now := time.Now().UTC()
		result := tx.Model(&models.UpdateTask{}).Where("id = ? AND status = ? AND executor_credential_id IS NULL", task.ID, TaskPending).Updates(map[string]any{"status": TaskClaimed, "executor_credential_id": credentialID, "claimed_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNoPendingTask
		}
		task.Status, task.ExecutorCredentialID, task.ClaimedAt = TaskClaimed, &credentialID, &now
		return tx.Create(&models.UpdateTaskEvent{TaskID: task.ID, Status: TaskClaimed}).Error
	})
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *TaskService) RecordEvent(credentialID uint, publicID, next, _ string) error {
	next = strings.TrimSpace(next)
	return s.db.Transaction(func(tx *gorm.DB) error {
		var task models.UpdateTask
		if err := tx.Where("public_id = ?", strings.TrimSpace(publicID)).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskNotFound
			}
			return err
		}
		if task.ExecutorCredentialID == nil || *task.ExecutorCredentialID != credentialID {
			return ErrTaskOwnership
		}
		if task.Status == next {
			return nil
		}
		if !validTaskTransition(task.Status, next) {
			return ErrInvalidTransition
		}
		// No executor-supplied free text is persisted: it may contain host
		// credentials or private paths. U3 can add finite error codes later.
		summary := ""
		updates := map[string]any{"status": next, "error_summary": ""}
		if next == TaskFailed || next == TaskNeedsAttention {
			updates["error_summary"] = "执行器报告异常，请查看宿主日志"
		}
		if terminalTaskStatus(next) {
			now := time.Now().UTC()
			updates["active_slot"], updates["finished_at"] = nil, now
		}
		result := tx.Model(&models.UpdateTask{}).Where("id = ? AND status = ?", task.ID, task.Status).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var current models.UpdateTask
			if err := tx.First(&current, task.ID).Error; err != nil {
				return err
			}
			if current.Status == next {
				return nil
			}
			return ErrInvalidTransition
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.UpdateTaskEvent{TaskID: task.ID, Status: next, Summary: summary}).Error
	})
}

func validTaskTransition(current, next string) bool {
	if next == TaskFailed {
		return current != TaskPending && !terminalTaskStatus(current)
	}
	if next == TaskNeedsAttention {
		return current != TaskPending && current != TaskClaimed && !terminalTaskStatus(current)
	}
	return map[string]string{TaskClaimed: TaskDownloading, TaskDownloading: TaskStopping, TaskStopping: TaskBackingUp, TaskBackingUp: TaskReplacing, TaskReplacing: TaskVerifying, TaskVerifying: TaskSucceeded}[current] == next
}

func terminalTaskStatus(status string) bool {
	return status == TaskSucceeded || status == TaskFailed || status == TaskNeedsAttention
}

func (s *TaskService) CreateCredential(actorID uint, name string) (models.UpdateExecutorCredential, string, error) {
	if actorID != models.PrimaryAdminUserID {
		return models.UpdateExecutorCredential{}, "", ErrCredentialInvalid
	}
	raw, err := randomHex(32)
	if err != nil {
		return models.UpdateExecutorCredential{}, "", err
	}
	raw = "enu_" + raw
	credential := models.UpdateExecutorCredential{Name: strings.TrimSpace(name), TokenHash: HashExecutorToken(raw), TokenPrefix: raw[:12]}
	if credential.Name == "" {
		credential.Name = "executor"
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureInstanceID(tx); err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&models.UpdateExecutorCredential{}).Where("revoked_at IS NULL AND superseded_at IS NULL").Update("superseded_at", now).Error; err != nil {
			return err
		}
		return tx.Create(&credential).Error
	})
	return credential, raw, err
}

func (s *TaskService) RevokeCredential(actorID uint) error {
	if actorID != models.PrimaryAdminUserID {
		return ErrCredentialInvalid
	}
	now := time.Now().UTC()
	return s.db.Model(&models.UpdateExecutorCredential{}).Where("revoked_at IS NULL").Update("revoked_at", now).Error
}

func (s *TaskService) CurrentCredential() (*models.UpdateExecutorCredential, error) {
	var credential models.UpdateExecutorCredential
	if err := s.db.Where("revoked_at IS NULL AND superseded_at IS NULL").Order("id DESC").First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &credential, nil
}

func (s *TaskService) Authenticate(raw string) (models.UpdateExecutorCredential, error) {
	var credential models.UpdateExecutorCredential
	if err := s.db.Where("token_hash = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", HashExecutorToken(strings.TrimSpace(raw)), time.Now().UTC()).First(&credential).Error; err != nil {
		return credential, ErrCredentialInvalid
	}
	if credential.SupersededAt != nil {
		var assigned int64
		if err := s.db.Model(&models.UpdateTask{}).Where("executor_credential_id = ? AND active_slot = ?", credential.ID, 1).Count(&assigned).Error; err != nil || assigned == 0 {
			return credential, ErrCredentialInvalid
		}
	}
	now := time.Now().UTC()
	if err := s.db.Model(&credential).Update("last_seen_at", now).Error; err != nil {
		return credential, err
	}
	credential.LastSeenAt = &now
	return credential, nil
}

func HashExecutorToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate secure random value: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func (s *TaskService) GetChannel() (string, error) {
	var preference models.UpdatePreference
	if err := s.db.First(&preference, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "stable", nil
		}
		return "", err
	}
	return preference.Channel, nil
}

// InstanceID lets an administrator pin the executor to the intended site.
// It is created when the first credential is issued, not by a GET request.
func (s *TaskService) InstanceID() (string, error) {
	var preference models.UpdatePreference
	if err := s.db.First(&preference, 1).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return preference.InstanceID, nil
}

func ensureInstanceID(tx *gorm.DB) error {
	var preference models.UpdatePreference
	if err := tx.First(&preference, 1).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if preference.InstanceID != "" {
		return nil
	}
	id, err := randomHex(16)
	if err != nil {
		return err
	}
	if preference.ID == 0 {
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.UpdatePreference{ID: 1, Channel: "stable", InstanceID: id}).Error
	}
	return tx.Model(&preference).Where("instance_id = ''").Update("instance_id", id).Error
}

func (s *TaskService) SetChannel(actorID uint, channel string) error {
	channel = strings.ToLower(strings.TrimSpace(channel))
	if actorID != models.PrimaryAdminUserID || (channel != "stable" && channel != "edge") {
		return ErrInvalidTarget
	}
	preference := models.UpdatePreference{ID: 1, Channel: channel}
	return s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"channel"})}).Create(&preference).Error
}
