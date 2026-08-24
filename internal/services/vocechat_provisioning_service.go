package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/runtimepolicy"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"gorm.io/gorm"
)

const voceChatProvisioningLeaseDuration = 2 * time.Minute

var (
	ErrVoceChatProvisioningPrimaryRequired = errors.New("VoceChat provisioning requires the primary administrator")
	ErrVoceChatProvisioningModeRequired    = errors.New("VoceChat provisioning requires normal VoceChat mode")
	errVoceChatProvisioningConflict        = errors.New("VoceChat provisioning identity conflict")
	errVoceChatProvisioningCredential      = errors.New("VoceChat provisioning credential invalid")
	errVoceChatProvisioningPasswordMissing = errors.New("VoceChat provisioning password material missing")
	errVoceChatProvisioningPersistence     = errors.New("VoceChat provisioning state persistence failed")
	errVoceChatProvisioningPaused          = errors.New("VoceChat provisioning paused")
)

type voceChatProvisioningCreateResult struct {
	User       *vocechat.User
	CreatedNow bool
}

type voceChatProvisioningCreateFunc func(context.Context, vocechat.Config, string, string, string) (voceChatProvisioningCreateResult, error)
type voceChatProvisioningDeleteFunc func(context.Context, vocechat.Config, string) error

var (
	voceChatProvisioningCreate  = defaultVoceChatProvisioningCreate
	voceChatProvisioningDelete  = defaultVoceChatProvisioningDelete
	voceChatProvisioningWake    = make(chan struct{}, 1)
	voceChatProvisioningBatch   sync.Mutex
	voceChatProvisioningCommand sync.Mutex
)

type voceChatProvisioningOutcome struct {
	Status     string
	UserStatus string
	Code       string
	Summary    string
}

func StartVoceChatProvisioning(ctx context.Context, actorID uint) (RuntimePolicyDiagnostics, error) {
	return requestVoceChatProvisioning(ctx, actorID, models.VoceChatProvisioningCommandStart)
}

func RetryVoceChatProvisioningFailures(ctx context.Context, actorID uint) (RuntimePolicyDiagnostics, error) {
	return requestVoceChatProvisioning(ctx, actorID, models.VoceChatProvisioningCommandRetry)
}

func requestVoceChatProvisioning(ctx context.Context, actorID uint, command string) (RuntimePolicyDiagnostics, error) {
	if actorID != models.PrimaryAdminUserID {
		return RuntimePolicyDiagnostics{}, ErrVoceChatProvisioningPrimaryRequired
	}
	voceChatProvisioningCommand.Lock()
	defer voceChatProvisioningCommand.Unlock()
	policy, config, err := loadRuntimePolicyAndVoceChatConfig()
	if err != nil {
		return RuntimePolicyDiagnostics{}, err
	}
	if policy.RuntimeState != runtimepolicy.StateVoceChatNormal || !config.IsReady() {
		return RuntimePolicyDiagnostics{}, ErrVoceChatProvisioningModeRequired
	}
	db, err := database.GetDB()
	if err != nil {
		return RuntimePolicyDiagnostics{}, err
	}

	var active models.VoceChatProvisioningRun
	if err := db.Where("status IN ?", []string{models.VoceChatProvisioningRunStatusRunning, models.VoceChatProvisioningRunStatusPaused}).Order("id DESC").First(&active).Error; err == nil {
		if active.Status == models.VoceChatProvisioningRunStatusPaused {
			if err := db.Model(&models.VoceChatProvisioningRun{}).Where("id = ?", active.ID).Updates(map[string]interface{}{
				"status":      models.VoceChatProvisioningRunStatusRunning,
				"finished_at": nil,
			}).Error; err != nil {
				return RuntimePolicyDiagnostics{}, err
			}
		}
		signalVoceChatProvisioningWorker()
		return GetRuntimePolicyDiagnostics(actorID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return RuntimePolicyDiagnostics{}, err
	}

	tasks, err := prepareVoceChatProvisioningTasks(command, config.EmailDomain)
	if err != nil {
		return RuntimePolicyDiagnostics{}, err
	}
	if len(tasks) == 0 {
		return GetRuntimePolicyDiagnostics(actorID)
	}

	now := time.Now().UTC()
	run := models.VoceChatProvisioningRun{
		RequestedByUserID: actorID,
		Command:           command,
		Status:            models.VoceChatProvisioningRunStatusRunning,
		StartedAt:         now,
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		for index := range tasks {
			task := &tasks[index]
			task.RunID = run.ID
			task.Status = models.VoceChatSyncStatusPending
			task.ErrorCode = ""
			task.ErrorSummary = ""
			task.LeaseUntil = nil
			task.CompletedAt = nil
			if err := tx.Model(&models.VoceChatProvisioningTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
				"run_id":        run.ID,
				"action":        task.Action,
				"status":        task.Status,
				"error_code":    "",
				"error_summary": "",
				"lease_until":   nil,
				"completed_at":  nil,
			}).Error; err != nil {
				return err
			}
		}
		return authorization.New(tx).WriteAudit(models.AdminAuditLog{
			ActorUserID: actorID,
			Module:      "runtime_mode",
			Action:      "vocechat_provisioning_" + command,
			TargetType:  "vocechat_provisioning_run",
			TargetID:    fmt.Sprint(run.ID),
			Result:      "success",
			Summary:     fmt.Sprintf("queued %d VoceChat account provisioning tasks", len(tasks)),
		})
	})
	if err != nil {
		return RuntimePolicyDiagnostics{}, err
	}
	signalVoceChatProvisioningWorker()
	return GetRuntimePolicyDiagnostics(actorID)
}

func prepareVoceChatProvisioningTasks(command, emailDomain string) ([]models.VoceChatProvisioningTask, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var users []models.User
	if err := db.Where("id <> ?", models.PrimaryAdminUserID).Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	prepared := make([]models.VoceChatProvisioningTask, 0, len(users))
	for index := range users {
		user := &users[index]
		var task *models.VoceChatProvisioningTask
		if command == models.VoceChatProvisioningCommandRetry {
			var existing models.VoceChatProvisioningTask
			if err := db.Where("user_id = ?", user.ID).First(&existing).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return nil, err
			}
			task = &existing
		} else {
			var err error
			task, err = ensureVoceChatProvisioningTask(user, emailDomain)
			if err != nil {
				return nil, err
			}
		}
		task.Action = voceChatProvisioningActionForUser(user)
		if !voceChatProvisioningTaskEligible(*task, *user, command) {
			continue
		}
		prepared = append(prepared, *task)
	}
	return prepared, nil
}

func ensureVoceChatProvisioningTask(user *models.User, emailDomain string) (*models.VoceChatProvisioningTask, error) {
	if user == nil || user.ID == 0 || user.ID == models.PrimaryAdminUserID {
		return nil, fmt.Errorf("invalid provisioning user")
	}
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var task models.VoceChatProvisioningTask
	if err := db.Where("user_id = ?", user.ID).First(&task).Error; err == nil {
		return &task, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var application models.RegistrationApplication
	applicationErr := db.Where("local_user_id = ?", user.ID).Order("id ASC").First(&application).Error
	if applicationErr == nil {
		applicationID := application.ID
		task = models.VoceChatProvisioningTask{
			UserID:                    user.ID,
			RegistrationApplicationID: &applicationID,
			ApplicationID:             application.ApplicationID,
			CandidateEmail:            application.VoceChatCandidateEmail,
			Action:                    voceChatProvisioningActionForUser(user),
			Status:                    models.VoceChatSyncStatusPending,
		}
		if err := db.Create(&task).Error; err != nil {
			if loadErr := db.Where("user_id = ?", user.ID).First(&task).Error; loadErr == nil {
				return &task, nil
			}
			return nil, err
		}
		return &task, nil
	}
	if !errors.Is(applicationErr, gorm.ErrRecordNotFound) {
		return nil, applicationErr
	}

	task = models.VoceChatProvisioningTask{
		UserID: user.ID,
		Action: voceChatProvisioningActionForUser(user),
		Status: models.VoceChatSyncStatusPending,
	}
	if err := repository.CreateVoceChatProvisioningTaskWithPermanentNumber(&task, emailDomain); err != nil {
		if loadErr := db.Where("user_id = ?", user.ID).First(&task).Error; loadErr == nil {
			return &task, nil
		}
		return nil, err
	}
	return &task, nil
}

func voceChatProvisioningActionForUser(user *models.User) string {
	if isVoceChatBoundNonPrimaryUser(user) {
		if user.VoceChatSyncStatus == models.VoceChatSyncStatusPasswordSyncRequired {
			return models.VoceChatProvisioningActionSyncPassword
		}
		return models.VoceChatProvisioningActionVerify
	}
	return models.VoceChatProvisioningActionProvision
}

func voceChatProvisioningTaskEligible(task models.VoceChatProvisioningTask, user models.User, command string) bool {
	if command == models.VoceChatProvisioningCommandRetry {
		return task.Status == models.VoceChatSyncStatusFailed ||
			task.Status == models.VoceChatSyncStatusConflicted ||
			task.Status == models.VoceChatSyncStatusCredentialInvalid ||
			task.Status == models.VoceChatSyncStatusPasswordSyncRequired ||
			user.VoceChatSyncStatus == models.VoceChatSyncStatusPasswordSyncRequired
	}
	if task.Status == models.VoceChatSyncStatusFailed || task.Status == models.VoceChatSyncStatusConflicted || task.Status == models.VoceChatSyncStatusCredentialInvalid {
		return false
	}
	return !(task.Status == models.VoceChatSyncStatusLinked && user.VoceChatSyncStatus == models.VoceChatSyncStatusLinked)
}

func StartVoceChatProvisioningWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		signalVoceChatProvisioningWorker()
		for {
			select {
			case <-ctx.Done():
				return
			case <-voceChatProvisioningWake:
				_ = RunActiveVoceChatProvisioning(ctx)
			case <-ticker.C:
				_ = RunActiveVoceChatProvisioning(ctx)
			}
		}
	}()
}

func signalVoceChatProvisioningWorker() {
	select {
	case voceChatProvisioningWake <- struct{}{}:
	default:
	}
}

func RunActiveVoceChatProvisioning(ctx context.Context) error {
	voceChatProvisioningBatch.Lock()
	defer voceChatProvisioningBatch.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return err
	}
	var run models.VoceChatProvisioningRun
	if err := db.Where("status IN ?", []string{models.VoceChatProvisioningRunStatusRunning, models.VoceChatProvisioningRunStatusPaused}).Order("id DESC").First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if run.Status == models.VoceChatProvisioningRunStatusPaused {
		return nil
	}
	if ready, err := voceChatProvisioningRuntimeReady(); err != nil {
		return err
	} else if !ready {
		return db.Model(&models.VoceChatProvisioningRun{}).Where("id = ?", run.ID).Update("status", models.VoceChatProvisioningRunStatusPaused).Error
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if ready, err := voceChatProvisioningRuntimeReady(); err != nil {
			return err
		} else if !ready {
			return db.Model(&models.VoceChatProvisioningRun{}).Where("id = ?", run.ID).Update("status", models.VoceChatProvisioningRunStatusPaused).Error
		}
		task, err := claimNextVoceChatProvisioningTask(run.ID)
		if err != nil {
			return err
		}
		if task == nil {
			var leased int64
			if err := db.Model(&models.VoceChatProvisioningTask{}).Where("run_id = ? AND status = ?", run.ID, models.VoceChatSyncStatusProvisioning).Count(&leased).Error; err != nil {
				return err
			}
			if leased > 0 {
				return nil
			}
			var linkedCount int64
			if err := db.Model(&models.VoceChatProvisioningTask{}).Where("run_id = ? AND status = ?", run.ID, models.VoceChatSyncStatusLinked).Count(&linkedCount).Error; err != nil {
				return err
			}
			var upstreamFailureCount int64
			if err := db.Model(&models.VoceChatProvisioningTask{}).Where("run_id = ? AND error_code = ?", run.ID, "upstream_unavailable").Count(&upstreamFailureCount).Error; err != nil {
				return err
			}
			if linkedCount == 0 && upstreamFailureCount > 0 {
				recordVoceChatLoginHealth("failed", errors.New("VoceChat provisioning requests unavailable"))
			}
			now := time.Now().UTC()
			return db.Model(&models.VoceChatProvisioningRun{}).Where("id = ?", run.ID).Updates(map[string]interface{}{
				"status":      models.VoceChatProvisioningRunStatusCompleted,
				"finished_at": now,
			}).Error
		}

		outcome := processVoceChatProvisioningTask(ctx, task)
		if err := finishVoceChatProvisioningTask(task, outcome); err != nil {
			return err
		}
	}
}

func voceChatProvisioningRuntimeReady() (bool, error) {
	policy, config, err := loadRuntimePolicyAndVoceChatConfig()
	if err != nil {
		return false, err
	}
	return policy.RuntimeState == runtimepolicy.StateVoceChatNormal && config.IsReady(), nil
}

func claimNextVoceChatProvisioningTask(runID uint) (*models.VoceChatProvisioningTask, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		now := time.Now().UTC()
		var task models.VoceChatProvisioningTask
		err := db.Where("run_id = ? AND (status = ? OR (status = ? AND lease_until <= ?))", runID, models.VoceChatSyncStatusPending, models.VoceChatSyncStatusProvisioning, now).
			Order("user_id ASC").First(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		leaseUntil := now.Add(voceChatProvisioningLeaseDuration)
		result := db.Model(&models.VoceChatProvisioningTask{}).
			Where("id = ? AND run_id = ? AND (status = ? OR (status = ? AND lease_until <= ?))", task.ID, runID, models.VoceChatSyncStatusPending, models.VoceChatSyncStatusProvisioning, now).
			Updates(map[string]interface{}{
				"status":          models.VoceChatSyncStatusProvisioning,
				"attempt_count":   gorm.Expr("attempt_count + ?", 1),
				"last_attempt_at": now,
				"lease_until":     leaseUntil,
				"error_code":      "",
				"error_summary":   "",
			})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		if err := db.First(&task, task.ID).Error; err != nil {
			return nil, err
		}
		if err := db.Model(&models.User{}).Where("id = ?", task.UserID).Updates(map[string]interface{}{
			"voce_chat_sync_status": models.VoceChatSyncStatusProvisioning,
			"voce_chat_sync_error":  "",
		}).Error; err != nil {
			return nil, err
		}
		repository.ClearUserCache()
		return &task, nil
	}
	return nil, nil
}

func processVoceChatProvisioningTask(ctx context.Context, task *models.VoceChatProvisioningTask) voceChatProvisioningOutcome {
	if task == nil || task.UserID == 0 || task.UserID == models.PrimaryAdminUserID {
		return voceChatProvisioningFailure(errVoceChatProvisioningPersistence)
	}
	taskCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := withUserPasswordMutation(task.UserID, func(user *models.User) error {
		if user.ID == models.PrimaryAdminUserID {
			return errVoceChatProvisioningPersistence
		}
		record, found, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(user.ID)
		if err != nil {
			return fmt.Errorf("%w: password store", errVoceChatProvisioningPersistence)
		}
		config, err := loadVoceChatSiteConfig()
		if err != nil {
			return fmt.Errorf("%w: configuration", errVoceChatProvisioningPersistence)
		}
		if !config.IsReady() {
			return errVoceChatProvisioningPaused
		}
		switch task.Action {
		case models.VoceChatProvisioningActionVerify:
			if !found || strings.TrimSpace(record.VoceChatPasswordValue()) == "" {
				return errVoceChatProvisioningCredential
			}
			response, err := voceChatPasswordLogin(taskCtx, config, user.VoceChatEmail, record.VoceChatPasswordValue())
			if err != nil {
				if isVoceChatCredentialRejected(err) {
					recordVoceChatLoginHealth("ok", nil)
					return errVoceChatProvisioningCredential
				}
				return err
			}
			recordVoceChatLoginHealth("ok", nil)
			if response == nil {
				return errors.New("empty VoceChat verification response")
			}
			return markVoceChatUserSync(user, models.VoceChatSyncStatusLinked, nil, voceChatUserFromInfo(response.User), user.Username)

		case models.VoceChatProvisioningActionSyncPassword:
			plain := currentLocalPasswordMaterial(user, record, found)
			if plain == "" {
				return errVoceChatProvisioningPasswordMissing
			}
			previousRemote := record.VoceChatPasswordValue()
			if previousRemote == "" {
				previousRemote = plain
			}
			if err := changeManagedVoceChatPassword(user, plain, previousRemote); err != nil {
				return err
			}
			recordVoceChatLoginHealth("ok", nil)
			return nil

		default:
			plain := currentLocalPasswordMaterial(user, record, found)
			if plain == "" {
				return errVoceChatProvisioningPasswordMissing
			}
			snapshot, err := readManagedPasswordSnapshot(user.ID)
			if err != nil {
				return fmt.Errorf("%w: snapshot", errVoceChatProvisioningPersistence)
			}
			var applicationSnapshot models.RegistrationApplication
			applicationSnapshotSeen := false
			if task.RegistrationApplicationID != nil {
				if err := database.DB.First(&applicationSnapshot, *task.RegistrationApplicationID).Error; err != nil {
					return fmt.Errorf("%w: registration application snapshot", errVoceChatProvisioningPersistence)
				}
				applicationSnapshotSeen = true
			}
			created, err := voceChatProvisioningCreate(taskCtx, config, task.CandidateEmail, user.Username, plain)
			if err != nil {
				if errors.Is(err, errVoceChatProvisioningConflict) || errors.Is(err, errVoceChatProvisioningCredential) {
					recordVoceChatLoginHealth("ok", nil)
				}
				return err
			}
			if created.User == nil || created.User.UID <= 0 || strings.TrimSpace(created.User.Email) == "" {
				if created.CreatedNow && created.User != nil && created.User.UID > 0 {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
					_ = voceChatProvisioningDelete(cleanupCtx, config, strconv.FormatInt(created.User.UID, 10))
					cleanupCancel()
				}
				return errors.New("incomplete VoceChat account result")
			}
			recordVoceChatLoginHealth("ok", nil)
			persistErr := markVoceChatUserSync(user, models.VoceChatSyncStatusLinked, nil, created.User, user.Username)
			if persistErr == nil {
				persistErr = vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, plain, user.VoceChatEmail, user.VoceChatUserID)
			}
			if persistErr == nil {
				persistErr = managedPasswordStateMatches(user, plain)
			}
			if persistErr == nil && task.RegistrationApplicationID != nil {
				result := database.DB.Model(&models.RegistrationApplication{}).Where("id = ?", *task.RegistrationApplicationID).Updates(map[string]interface{}{
					"voce_chat_user_id":     user.VoceChatUserID,
					"voce_chat_email":       user.VoceChatEmail,
					"voce_chat_sync_status": models.VoceChatSyncStatusLinked,
					"voce_chat_sync_error":  "",
				})
				if result.Error != nil {
					persistErr = fmt.Errorf("registration application binding was not persisted")
				} else {
					var storedApplication models.RegistrationApplication
					if err := database.DB.First(&storedApplication, *task.RegistrationApplicationID).Error; err != nil ||
						storedApplication.VoceChatUserID != user.VoceChatUserID ||
						storedApplication.VoceChatEmail != user.VoceChatEmail ||
						storedApplication.VoceChatSyncStatus != models.VoceChatSyncStatusLinked ||
						storedApplication.VoceChatSyncError != "" {
						persistErr = fmt.Errorf("registration application binding was not verified")
					}
				}
			}
			if persistErr != nil {
				if created.CreatedNow {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
					_ = voceChatProvisioningDelete(cleanupCtx, config, strconv.FormatInt(created.User.UID, 10))
					cleanupCancel()
				}
				_ = restoreManagedPasswordSnapshot(user, snapshot)
				if applicationSnapshotSeen {
					_ = database.DB.Model(&models.RegistrationApplication{}).Where("id = ?", applicationSnapshot.ID).Updates(map[string]interface{}{
						"voce_chat_user_id":     applicationSnapshot.VoceChatUserID,
						"voce_chat_email":       applicationSnapshot.VoceChatEmail,
						"voce_chat_sync_status": applicationSnapshot.VoceChatSyncStatus,
						"voce_chat_sync_error":  applicationSnapshot.VoceChatSyncError,
					}).Error
				}
				return fmt.Errorf("%w: finalize", errVoceChatProvisioningPersistence)
			}
			return nil
		}
	})
	if err != nil {
		if errors.Is(err, errVoceChatProvisioningPaused) || errors.Is(err, context.Canceled) {
			return pausedVoceChatProvisioningOutcome(task.Action)
		}
		return voceChatProvisioningFailure(err)
	}
	return voceChatProvisioningOutcome{Status: models.VoceChatSyncStatusLinked, UserStatus: models.VoceChatSyncStatusLinked}
}

func currentLocalPasswordMaterial(user *models.User, record vocechat.PlainPasswordRecord, found bool) string {
	if user == nil || !found {
		return ""
	}
	for _, candidate := range []string{record.LocalFallbackPasswordValue(), record.VoceChatPasswordValue()} {
		if strings.TrimSpace(candidate) != "" && passwordMatchesStored(user.Password, candidate) {
			return candidate
		}
	}
	return ""
}

func voceChatProvisioningFailure(err error) voceChatProvisioningOutcome {
	switch {
	case errors.Is(err, errVoceChatProvisioningPasswordMissing):
		return voceChatProvisioningOutcome{Status: models.VoceChatSyncStatusPasswordSyncRequired, UserStatus: models.VoceChatSyncStatusPasswordSyncRequired, Code: "password_material_missing", Summary: "缺少可用的当前密码，请由用户或1号管理员重置密码后重试"}
	case errors.Is(err, errVoceChatProvisioningConflict):
		return voceChatProvisioningOutcome{Status: models.VoceChatSyncStatusConflicted, UserStatus: models.VoceChatSyncStatusConflicted, Code: "identity_conflict", Summary: "待创建邮箱已被占用，请核对远端账户后重试"}
	case errors.Is(err, errVoceChatProvisioningCredential):
		return voceChatProvisioningOutcome{Status: models.VoceChatSyncStatusCredentialInvalid, UserStatus: models.VoceChatSyncStatusCredentialInvalid, Code: "credential_invalid", Summary: "已绑定账户凭据无法验证，请重置密码后重试"}
	case errors.Is(err, errVoceChatProvisioningPersistence):
		return voceChatProvisioningOutcome{Status: models.VoceChatSyncStatusFailed, UserStatus: models.VoceChatSyncStatusFailed, Code: "state_persistence_failed", Summary: "账户状态保存失败，可安全重试"}
	default:
		return voceChatProvisioningOutcome{Status: models.VoceChatSyncStatusFailed, UserStatus: models.VoceChatSyncStatusFailed, Code: "upstream_unavailable", Summary: "VoceChat 暂不可用，恢复后可重试"}
	}
}

func pausedVoceChatProvisioningOutcome(action string) voceChatProvisioningOutcome {
	userStatus := models.VoceChatSyncStatusPending
	if action == models.VoceChatProvisioningActionVerify {
		userStatus = models.VoceChatSyncStatusLinked
	} else if action == models.VoceChatProvisioningActionSyncPassword {
		userStatus = models.VoceChatSyncStatusPasswordSyncRequired
	}
	return voceChatProvisioningOutcome{Status: models.VoceChatSyncStatusPending, UserStatus: userStatus}
}

func finishVoceChatProvisioningTask(task *models.VoceChatProvisioningTask, outcome voceChatProvisioningOutcome) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var completedAt interface{} = now
	if outcome.Status == models.VoceChatSyncStatusPending {
		completedAt = nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.VoceChatProvisioningTask{}).Where("id = ? AND status = ?", task.ID, models.VoceChatSyncStatusProvisioning).Updates(map[string]interface{}{
			"status":        outcome.Status,
			"error_code":    outcome.Code,
			"error_summary": outcome.Summary,
			"lease_until":   nil,
			"completed_at":  completedAt,
		}).Error; err != nil {
			return err
		}
		userStatus := outcome.UserStatus
		if userStatus == "" {
			userStatus = outcome.Status
		}
		updates := map[string]interface{}{
			"voce_chat_sync_status":  userStatus,
			"voce_chat_sync_error":   outcome.Summary,
			"voce_chat_last_sync_at": now,
		}
		if outcome.Status == models.VoceChatSyncStatusLinked {
			updates["voce_chat_sync_error"] = ""
		}
		if err := tx.Model(&models.User{}).Where("id = ?", task.UserID).Updates(updates).Error; err != nil {
			return err
		}
		repository.ClearUserCache()
		return nil
	})
}

func defaultVoceChatProvisioningCreate(ctx context.Context, config vocechat.Config, candidateEmail, username, password string) (voceChatProvisioningCreateResult, error) {
	if !config.IsReady() {
		return voceChatProvisioningCreateResult{}, errors.New("VoceChat configuration unavailable")
	}
	client, err := vocechat.NewClient(config)
	if err != nil {
		return voceChatProvisioningCreateResult{}, err
	}
	tokenManager := vocechat.NewAdminTokenManager(client, config)
	apiKey, err := tokenManager.GetToken(ctx)
	if err != nil {
		return voceChatProvisioningCreateResult{}, err
	}
	findExisting := func() (*vocechat.User, error) {
		users, err := client.ListUsers(ctx, apiKey)
		if err != nil {
			return nil, err
		}
		for index := range users {
			if strings.EqualFold(strings.TrimSpace(users[index].Email), strings.TrimSpace(candidateEmail)) {
				return &users[index], nil
			}
		}
		return nil, nil
	}
	verifyExisting := func(existing *vocechat.User) (voceChatProvisioningCreateResult, error) {
		response, err := voceChatPasswordLogin(ctx, config, candidateEmail, password)
		if err != nil {
			if isVoceChatCredentialRejected(err) {
				return voceChatProvisioningCreateResult{}, errVoceChatProvisioningConflict
			}
			return voceChatProvisioningCreateResult{}, err
		}
		if response != nil && response.User.UID > 0 {
			existing = voceChatUserFromInfo(response.User)
		}
		return voceChatProvisioningCreateResult{User: existing}, nil
	}
	existing, err := findExisting()
	if err != nil {
		return voceChatProvisioningCreateResult{}, err
	}
	if existing != nil {
		return verifyExisting(existing)
	}
	created, err := client.CreateUser(ctx, apiKey, vocechat.CreateUserRequest{
		Email: candidateEmail, Password: password, Name: strings.TrimSpace(username), Gender: 0, IsAdmin: false, Language: "zh-CN",
	})
	if err != nil {
		if isVoceChatUserConflict(err) {
			existing, lookupErr := findExisting()
			if lookupErr != nil {
				return voceChatProvisioningCreateResult{}, lookupErr
			}
			if existing != nil {
				return verifyExisting(existing)
			}
			return voceChatProvisioningCreateResult{}, errVoceChatProvisioningConflict
		}
		return voceChatProvisioningCreateResult{}, err
	}
	if created == nil || created.UID <= 0 {
		return voceChatProvisioningCreateResult{}, errors.New("VoceChat returned an incomplete created user")
	}
	response, verifyErr := voceChatPasswordLogin(ctx, config, candidateEmail, password)
	if verifyErr != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = client.DeleteUser(cleanupCtx, apiKey, created.UID)
		cleanupCancel()
		if isVoceChatCredentialRejected(verifyErr) {
			return voceChatProvisioningCreateResult{}, errVoceChatProvisioningCredential
		}
		return voceChatProvisioningCreateResult{}, verifyErr
	}
	if response != nil && response.User.UID > 0 {
		created = voceChatUserFromInfo(response.User)
	}
	return voceChatProvisioningCreateResult{User: created, CreatedNow: true}, nil
}

func defaultVoceChatProvisioningDelete(ctx context.Context, config vocechat.Config, userID string) error {
	uid, err := strconv.ParseInt(strings.TrimSpace(userID), 10, 64)
	if err != nil || uid <= 0 {
		return errors.New("invalid VoceChat user id")
	}
	client, err := vocechat.NewClient(config)
	if err != nil {
		return err
	}
	tokenManager := vocechat.NewAdminTokenManager(client, config)
	apiKey, err := tokenManager.GetToken(ctx)
	if err != nil {
		return err
	}
	return client.DeleteUser(ctx, apiKey, uid)
}

func voceChatUserFromInfo(info vocechat.UserInfo) *vocechat.User {
	return &vocechat.User{
		UID:      info.UID,
		Email:    info.Email,
		Name:     info.Name,
		Gender:   info.Gender,
		IsAdmin:  info.IsAdmin,
		Language: info.Language,
		CreateBy: info.CreateBy,
		IsBot:    info.IsBot,
	}
}
