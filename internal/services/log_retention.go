package services

import (
	"context"
	"log"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const logRetentionBatchSize = 1000

type LogRetentionResult struct {
	AttackLogs  int64
	AccessLogs  int64
	SiteVisits  int64
	LoginAudits int64
	AdminAudits int64
}

type logRetentionTarget struct {
	model         any
	retentionDays int
	maxRows       int64
	softDelete    bool
	deleted       *int64
}

// RunLogRetentionCleanup applies a time policy first and a fixed maximum-row
// safety valve second. Zero days means no time-based deletion; the safety valve
// still prevents an accidentally unbounded SQLite database.
func RunLogRetentionCleanup(db *gorm.DB, now time.Time) (LogRetentionResult, error) {
	result := LogRetentionResult{}
	if db == nil {
		return result, nil
	}
	security := models.SecurityConfig{AttackLogRetentionDays: 90, AccessLogRetentionDays: 30, SiteVisitRetentionDays: 90, LoginAuditRetentionDays: 365}
	if err := db.Order("id asc").First(&security).Error; err != nil && err != gorm.ErrRecordNotFound {
		return result, err
	}
	admin := models.AdminAuditConfig{ID: 1, Enabled: true, RetentionDays: 730}
	if err := db.First(&admin, 1).Error; err != nil && err != gorm.ErrRecordNotFound {
		return result, err
	}
	targets := []logRetentionTarget{
		{model: &models.SecurityAttackLog{}, retentionDays: security.AttackLogRetentionDays, maxRows: 100000, softDelete: true, deleted: &result.AttackLogs},
		{model: &models.SecurityAccessLog{}, retentionDays: security.AccessLogRetentionDays, maxRows: 200000, softDelete: true, deleted: &result.AccessLogs},
		{model: &models.SecuritySiteVisitLog{}, retentionDays: security.SiteVisitRetentionDays, maxRows: 100000, softDelete: true, deleted: &result.SiteVisits},
		{model: &models.SecurityLoginAudit{}, retentionDays: security.LoginAuditRetentionDays, maxRows: 100000, softDelete: true, deleted: &result.LoginAudits},
		{model: &models.AdminAuditLog{}, retentionDays: admin.RetentionDays, maxRows: 200000, deleted: &result.AdminAudits},
	}
	for _, target := range targets {
		deleted, err := cleanupLogTarget(db, target.model, target.retentionDays, target.maxRows, target.softDelete, now.UTC())
		if err != nil {
			return result, err
		}
		*target.deleted = deleted
	}
	return result, nil
}

func cleanupLogTarget(db *gorm.DB, model any, retentionDays int, maxRows int64, softDelete bool, now time.Time) (int64, error) {
	var deleted int64
	if softDelete {
		result := db.Unscoped().Where("deleted_at IS NOT NULL").Delete(model)
		if result.Error != nil {
			return deleted, result.Error
		}
		deleted += result.RowsAffected
	}
	if retentionDays > 0 {
		cutoff := now.AddDate(0, 0, -retentionDays)
		for {
			ids, err := oldestLogIDs(db, model, "created_at < ?", []any{cutoff}, logRetentionBatchSize)
			if err != nil || len(ids) == 0 {
				return deleted, err
			}
			result := db.Unscoped().Where("id IN ?", ids).Delete(model)
			if result.Error != nil {
				return deleted, result.Error
			}
			deleted += result.RowsAffected
			if result.RowsAffected == 0 {
				break
			}
			if len(ids) < logRetentionBatchSize {
				break
			}
		}
	}
	if maxRows <= 0 {
		return deleted, nil
	}
	var count int64
	if err := db.Model(model).Count(&count).Error; err != nil {
		return deleted, err
	}
	excess := count - maxRows
	for excess > 0 {
		limit := logRetentionBatchSize
		if excess < int64(limit) {
			limit = int(excess)
		}
		ids, err := oldestLogIDs(db, model, "", nil, limit)
		if err != nil || len(ids) == 0 {
			return deleted, err
		}
		result := db.Unscoped().Where("id IN ?", ids).Delete(model)
		if result.Error != nil {
			return deleted, result.Error
		}
		deleted += result.RowsAffected
		if result.RowsAffected == 0 {
			break
		}
		excess -= result.RowsAffected
	}
	return deleted, nil
}

func oldestLogIDs(db *gorm.DB, model any, predicate string, args []any, limit int) ([]uint, error) {
	query := db.Model(model)
	if predicate != "" {
		query = query.Where(predicate, args...)
	}
	var ids []uint
	err := query.Order("created_at ASC, id ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func StartLogRetentionWorker(ctx context.Context, db *gorm.DB) {
	go func() {
		run := func() {
			result, err := RunLogRetentionCleanup(db, time.Now().UTC())
			if err != nil {
				log.Printf("日志保留策略清理失败: %v", err)
				return
			}
			if result.AttackLogs+result.AccessLogs+result.SiteVisits+result.LoginAudits+result.AdminAudits > 0 {
				log.Printf("日志保留策略清理完成: 攻击=%d 访问=%d 站点=%d 登录=%d 管理审计=%d", result.AttackLogs, result.AccessLogs, result.SiteVisits, result.LoginAudits, result.AdminAudits)
			}
		}
		run()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				run()
			case <-ctx.Done():
				return
			}
		}
	}()
}
