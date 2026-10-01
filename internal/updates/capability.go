package updates

import (
	"errors"
	"regexp"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const ExecutorVersion = "u5-1"
const ExecutorCheckWindow = 3 * time.Minute

var executorVersionPattern = regexp.MustCompile(`^u[0-9]{1,2}-[0-9]{1,3}$`)

type DeploymentCheck struct {
	InstanceID string `json:"instance_id"`
	Version    string `json:"version"`
	Platform   string `json:"platform"`
	Revision   string `json:"revision"`
	OK         bool   `json:"ok"`
}

type InstallationStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

// This is an authenticated report of the administrator-installed executor's
// actual preflight, not a promise based on token authentication alone.
func (s *TaskService) RecordDeploymentCheck(id uint, check DeploymentCheck) error {
	instance, err := s.InstanceID()
	if err != nil {
		return err
	}
	if instance == "" || check.InstanceID != instance || !executorVersionPattern.MatchString(check.Version) || (check.Platform != "linux/amd64" && check.Platform != "linux/arm64") || !validRevision(check.Revision) {
		return ErrInvalidTarget
	}
	result := s.db.Model(&models.UpdateExecutorCredential{}).Where("id = ? AND revoked_at IS NULL AND superseded_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", id, time.Now().UTC()).Updates(map[string]any{
		"checked_at": time.Now().UTC(), "instance_id": instance, "executor_version": check.Version, "platform": check.Platform, "installed_revision": check.Revision, "check_ok": check.OK,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCredentialInvalid
	}
	return nil
}

func (s *TaskService) InstallationStatus(revision string) (InstallationStatus, error) {
	c, err := s.CurrentCredential()
	if err != nil {
		return InstallationStatus{}, err
	}
	instance, err := s.InstanceID()
	if err != nil {
		return InstallationStatus{}, err
	}
	status := InstallationStatus{}
	switch {
	case s.db.Dialector.Name() != "sqlite":
		status.Reason = "database_unsupported"
	case c == nil:
		status.Reason = "executor_unconfigured"
	case c.ExpiresAt != nil && !c.ExpiresAt.After(time.Now()):
		status.Reason = "credential_expired"
	case c.CheckedAt == nil:
		status.Reason = "deployment_unchecked"
	case c.CheckedAt.Before(time.Now().Add(-ExecutorCheckWindow)):
		status.Reason = "executor_offline"
	case c.InstanceID != instance:
		status.Reason = "instance_mismatch"
	case c.ExecutorVersion != ExecutorVersion:
		status.Reason = "executor_upgrade_required"
	case c.Platform != "linux/amd64":
		status.Reason = "platform_unsupported"
	case !c.CheckOK:
		status.Reason = "deployment_check_failed"
	case revision != "" && c.InstalledRevision != revision:
		status.Reason = "deployment_unchecked"
	default:
		status.Available = true
	}
	return status, nil
}

// Prefer the occupied task even if another result was inserted later; otherwise
// restore the most recent result, including a lost final browser response.
func (s *TaskService) LatestTask() (*models.UpdateTask, error) {
	var task models.UpdateTask
	err := s.db.Order("CASE WHEN active_slot = 1 THEN 0 ELSE 1 END").Order("id DESC").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &task, err
}
