package updates

import (
	"errors"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const ExecutorPrepareCooldown = 30 * time.Second

var (
	ErrUpdateTaskActive        = errors.New("active update task")
	ErrExecutorPrepareCooldown = errors.New("executor preparation cooldown")
)

type DeploymentPreparation struct {
	CredentialID uint               `json:"credential_id"`
	RequestedAt  *time.Time         `json:"requested_at,omitempty"`
	Installation InstallationStatus `json:"installation"`
}

// PreparationStatus reads local state only; it never creates an instance or
// credential, requests a check, or discovers a registry target.
func (s *TaskService) PreparationStatus(revision string) (DeploymentPreparation, error) {
	var preparation DeploymentPreparation
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		preparation, err = NewTaskService(tx).preparationStatus(revision)
		return err
	})
	return preparation, err
}

func (s *TaskService) preparationStatus(revision string) (DeploymentPreparation, error) {
	credential, err := s.CurrentCredential()
	if err != nil {
		return DeploymentPreparation{}, err
	}
	installation, err := s.InstallationStatus(revision)
	if err != nil {
		return DeploymentPreparation{}, err
	}
	preparation := DeploymentPreparation{Installation: installation}
	if credential != nil {
		preparation.CredentialID = credential.ID
		preparation.RequestedAt = credential.CheckRequestedAt
	}
	return preparation, nil
}

func preparationRecheckAllowed(status InstallationStatus) bool {
	return status.Reason == "deployment_unchecked" || status.Reason == "executor_offline" || status.Reason == "deployment_check_failed"
}

func deploymentCheckPending(credential *models.UpdateExecutorCredential) bool {
	return credential != nil && credential.CheckRequestedAt != nil && (credential.CheckedAt == nil || credential.CheckedAt.Before(*credential.CheckRequestedAt))
}

// RequestDeploymentCheck persists intent before the caller sends a wake. The
// conditional write enforces the cooldown in the database, including independent
// service instances; no process-local lock or timer owns the request.
func (s *TaskService) RequestDeploymentCheck(actorID uint, revision string) (DeploymentPreparation, bool, error) {
	if actorID != models.PrimaryAdminUserID {
		return DeploymentPreparation{}, false, ErrCredentialInvalid
	}
	var preparation DeploymentPreparation
	created := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var active int64
		if err := tx.Model(&models.UpdateTask{}).Where("active_slot = ?", 1).Count(&active).Error; err != nil {
			return err
		}
		if active != 0 {
			return ErrUpdateTaskActive
		}
		service := NewTaskService(tx)
		var err error
		preparation, err = service.preparationStatus(revision)
		if err != nil {
			return err
		}
		if preparation.Installation.Available {
			return nil
		}
		if !preparationRecheckAllowed(preparation.Installation) {
			return ErrExecutorNotConfigured
		}
		now := time.Now().UTC()
		credentialID := preparation.CredentialID
		result := tx.Model(&models.UpdateExecutorCredential{}).
			Where("id = ? AND revoked_at IS NULL AND superseded_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", credentialID, now).
			Where("check_requested_at IS NULL OR check_requested_at <= ?", now.Add(-ExecutorPrepareCooldown)).
			Update("check_requested_at", now)
		if result.Error != nil {
			return result.Error
		}
		preparation, err = service.preparationStatus(revision)
		if err != nil {
			return err
		}
		credential, err := service.CurrentCredential()
		if err != nil {
			return err
		}
		if credential == nil || credential.ID != credentialID || (credential.ExpiresAt != nil && !credential.ExpiresAt.After(now)) {
			return ErrExecutorNotConfigured
		}
		if result.RowsAffected == 1 {
			created = true
			return nil
		}
		if preparation.Installation.Available {
			return nil
		}
		if !preparationRecheckAllowed(preparation.Installation) {
			return ErrExecutorNotConfigured
		}
		if deploymentCheckPending(credential) {
			return nil
		}
		return ErrExecutorPrepareCooldown
	})
	// A failed transaction never authorizes a wake, even if its write succeeded
	// before a later read or commit failed.
	return preparation, created && err == nil, err
}

type ExecutorWork struct {
	InstanceID     string `json:"instance_id"`
	CheckRequested bool   `json:"check_requested"`
	TaskAvailable  bool   `json:"task_available"`
}

// ExecutorWork advertises work without claiming it or refreshing deployment
// checks. A superseded credential can see its own assigned work only, matching
// the existing recovery/claim rules.
func (s *TaskService) ExecutorWork(credentialID uint) (ExecutorWork, error) {
	var work ExecutorWork
	err := s.db.Transaction(func(tx *gorm.DB) error {
		service := NewTaskService(tx)
		var err error
		work.InstanceID, err = service.InstanceID()
		if err != nil {
			return err
		}
		var credential models.UpdateExecutorCredential
		now := time.Now().UTC()
		if err := tx.Where("id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", credentialID, now).First(&credential).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCredentialInvalid
			}
			return err
		}
		current := credential.SupersededAt == nil
		work.CheckRequested = current && deploymentCheckPending(&credential)
		query := tx.Model(&models.UpdateTask{}).Where("active_slot = ?", 1)
		if current {
			query = query.Where("executor_credential_id = ? OR (status = ? AND executor_credential_id IS NULL)", credentialID, TaskPending)
		} else {
			query = query.Where("executor_credential_id = ?", credentialID)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		work.TaskAvailable = count > 0
		return nil
	})
	return work, err
}
