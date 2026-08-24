package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/runtimepolicy"
	"gorm.io/gorm"
)

type RuntimePolicyDiagnostics struct {
	runtimepolicy.Policy
	LastHealthStatus  string                          `json:"last_health_status"`
	LastHealthSummary string                          `json:"last_health_summary"`
	LastHealthCheckAt *time.Time                      `json:"last_health_check_at,omitempty"`
	AccountCounts     map[string]int64                `json:"account_counts"`
	ProvisioningRun   *models.VoceChatProvisioningRun `json:"provisioning_run,omitempty"`
	ProvisioningTasks []VoceChatProvisioningTaskView  `json:"provisioning_tasks"`
}

type VoceChatProvisioningTaskView struct {
	ID             uint       `json:"id"`
	UserID         uint       `json:"user_id"`
	Username       string     `json:"username"`
	CandidateEmail string     `json:"candidate_email"`
	Action         string     `json:"action"`
	Status         string     `json:"status"`
	AttemptCount   uint       `json:"attempt_count"`
	ErrorCode      string     `json:"error_code,omitempty"`
	ErrorSummary   string     `json:"error_summary,omitempty"`
	LastAttemptAt  *time.Time `json:"last_attempt_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

var (
	ErrRuntimeModePrimaryAdminRequired = errors.New("runtime mode operations require the primary administrator")
	ErrRuntimeModeInvalid              = errors.New("invalid configured runtime mode")
	ErrRuntimeModeConfigIncomplete     = errors.New("VoceChat configuration is incomplete")
	ErrRuntimeModeHealthCheckFailed    = errors.New("VoceChat health check failed")
)

var runtimeModeHealthCheck = func(ctx context.Context) error {
	_, err := CheckVoceChatHealth(ctx)
	return err
}

func ResolveRuntimePolicy() (runtimepolicy.Policy, error) {
	db, err := database.GetDB()
	if err != nil {
		return runtimepolicy.Policy{}, err
	}
	var config models.SiteConfig
	if err := db.Order("id ASC").First(&config).Error; err != nil {
		return runtimepolicy.Policy{}, err
	}
	return runtimepolicy.Resolve(config), nil
}

func GetRuntimePolicyDiagnostics(actorID uint) (RuntimePolicyDiagnostics, error) {
	if actorID != models.PrimaryAdminUserID {
		return RuntimePolicyDiagnostics{}, ErrRuntimeModePrimaryAdminRequired
	}
	db, err := database.GetDB()
	if err != nil {
		return RuntimePolicyDiagnostics{}, err
	}
	var config models.SiteConfig
	if err := db.Order("id ASC").First(&config).Error; err != nil {
		return RuntimePolicyDiagnostics{}, err
	}
	counts := map[string]int64{}
	for _, status := range []string{
		models.VoceChatSyncStatusUnbound,
		models.VoceChatSyncStatusPending,
		models.VoceChatSyncStatusProvisioning,
		models.VoceChatSyncStatusLinked,
		models.VoceChatSyncStatusFailed,
		models.VoceChatSyncStatusConflicted,
		models.VoceChatSyncStatusCredentialInvalid,
		models.VoceChatSyncStatusPasswordSyncRequired,
	} {
		var count int64
		if err := db.Model(&models.User{}).
			Where("id <> ? AND voce_chat_sync_status = ?", models.PrimaryAdminUserID, status).
			Count(&count).Error; err != nil {
			return RuntimePolicyDiagnostics{}, err
		}
		counts[status] = count
	}
	policy := runtimepolicy.Resolve(config)
	var latestRun models.VoceChatProvisioningRun
	var latestRunPointer *models.VoceChatProvisioningRun
	if err := db.Order("id DESC").First(&latestRun).Error; err == nil {
		latestRunPointer = &latestRun
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return RuntimePolicyDiagnostics{}, err
	}
	taskViews := []VoceChatProvisioningTaskView{}
	type taskRow struct {
		models.VoceChatProvisioningTask
		Username string
	}
	var taskRows []taskRow
	if err := db.Table("voce_chat_provisioning_tasks AS tasks").
		Select("tasks.*, users.username AS username").
		Joins("JOIN users ON users.id = tasks.user_id").
		Order("tasks.user_id ASC").
		Scan(&taskRows).Error; err != nil {
		return RuntimePolicyDiagnostics{}, err
	}
	for index := range taskRows {
		task := taskRows[index]
		taskViews = append(taskViews, VoceChatProvisioningTaskView{
			ID:             task.ID,
			UserID:         task.UserID,
			Username:       task.Username,
			CandidateEmail: task.CandidateEmail,
			Action:         task.Action,
			Status:         task.Status,
			AttemptCount:   task.AttemptCount,
			ErrorCode:      task.ErrorCode,
			ErrorSummary:   task.ErrorSummary,
			LastAttemptAt:  task.LastAttemptAt,
			CompletedAt:    task.CompletedAt,
		})
	}
	healthStatus := strings.ToLower(strings.TrimSpace(config.VoceChatLastHealthStatus))
	healthSummary := "尚未完成 VoceChat 健康检查"
	if policy.ConfiguredMode == runtimepolicy.ModeLocal {
		healthSummary = "本地模式不调用 VoceChat"
	} else if healthStatus == "ok" {
		healthSummary = "VoceChat 健康检查正常"
	} else if healthStatus == "failed" {
		healthSummary = "VoceChat 暂时不可用，请检查服务与管理员配置"
	}
	return RuntimePolicyDiagnostics{
		Policy:            policy,
		LastHealthStatus:  healthStatus,
		LastHealthSummary: healthSummary,
		LastHealthCheckAt: config.VoceChatLastHealthCheckAt,
		AccountCounts:     counts,
		ProvisioningRun:   latestRunPointer,
		ProvisioningTasks: taskViews,
	}, nil
}

func SwitchConfiguredMode(ctx context.Context, actorID uint, targetMode runtimepolicy.ConfiguredMode) (runtimepolicy.Policy, error) {
	if actorID != models.PrimaryAdminUserID {
		return runtimepolicy.Policy{}, ErrRuntimeModePrimaryAdminRequired
	}
	parsedMode, ok := runtimepolicy.ParseConfiguredMode(string(targetMode))
	if !ok {
		return runtimepolicy.Policy{}, ErrRuntimeModeInvalid
	}
	db, err := database.GetDB()
	if err != nil {
		return runtimepolicy.Policy{}, err
	}
	var config models.SiteConfig
	if err := db.Order("id ASC").First(&config).Error; err != nil {
		return runtimepolicy.Policy{}, err
	}
	currentMode := runtimepolicy.EffectiveConfiguredMode(config)
	if currentMode == parsedMode {
		return runtimepolicy.Resolve(config), nil
	}

	if parsedMode == runtimepolicy.ModeVoceChat {
		if !runtimepolicy.ConnectionConfigured(config) {
			return runtimepolicy.Policy{}, ErrRuntimeModeConfigIncomplete
		}
		if err := runtimeModeHealthCheck(ctx); err != nil {
			authorization.New(db).WriteAuditBestEffort(models.AdminAuditLog{
				ActorUserID: actorID,
				Module:      "runtime_mode",
				Action:      "switch",
				TargetType:  "site_config",
				TargetID:    fmt.Sprint(config.ID),
				Result:      "failure",
				Summary:     "VoceChat mode switch rejected because the health check failed",
			})
			return runtimepolicy.Policy{}, fmt.Errorf("%w: %v", ErrRuntimeModeHealthCheckFailed, err)
		}
		if err := db.Order("id ASC").First(&config).Error; err != nil {
			return runtimepolicy.Policy{}, err
		}
	}

	changes, _ := json.Marshal(map[string]string{"from": string(currentMode), "to": string(parsedMode)})
	if err := db.Transaction(func(tx *gorm.DB) error {
		voceChatEnabled := parsedMode == runtimepolicy.ModeVoceChat
		updates := map[string]interface{}{
			"runtime_mode":                         string(parsedMode),
			"runtime_mode_migration_version":       models.RuntimeModeMigrationVersionCurrent,
			"voce_chat_enabled":                    voceChatEnabled,
			"voce_chat_login_verification_enabled": voceChatEnabled,
			"voce_chat_local_fallback_enabled":     false,
			"voce_chat_contacts_enabled":           voceChatEnabled,
			"voce_chat_notification_enabled":       voceChatEnabled,
		}
		if err := tx.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(updates).Error; err != nil {
			return err
		}
		if parsedMode == runtimepolicy.ModeVoceChat {
			if err := tx.Model(&models.User{}).
				Where("id <> ?", models.PrimaryAdminUserID).
				Where("voce_chat_sync_status IS NULL OR voce_chat_sync_status IN ?", []string{"", models.VoceChatSyncStatusNone, models.VoceChatSyncStatusUnbound}).
				Update("voce_chat_sync_status", models.VoceChatSyncStatusPending).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&models.VoceChatProvisioningRun{}).
				Where("status = ?", models.VoceChatProvisioningRunStatusRunning).
				Update("status", models.VoceChatProvisioningRunStatusPaused).Error; err != nil {
				return err
			}
		}
		return authorization.New(tx).WriteAudit(models.AdminAuditLog{
			ActorUserID: actorID,
			Module:      "runtime_mode",
			Action:      "switch",
			TargetType:  "site_config",
			TargetID:    fmt.Sprint(config.ID),
			Result:      "success",
			Summary:     fmt.Sprintf("switched configured runtime mode from %s to %s", currentMode, parsedMode),
			ChangesJSON: string(changes),
		})
	}); err != nil {
		return runtimepolicy.Policy{}, err
	}
	if err := db.Order("id ASC").First(&config).Error; err != nil {
		return runtimepolicy.Policy{}, err
	}
	return runtimepolicy.Resolve(config), nil
}
