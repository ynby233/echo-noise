package controllers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

type adminAuditFilters struct {
	Keyword     string
	ActorUserID *uint
	Module      string
	Action      string
	Result      string
	TargetType  string
	TargetID    string
	Start       *time.Time
	End         *time.Time
}

func parseAdminAuditFilters(c *gin.Context) (adminAuditFilters, error) {
	filters := adminAuditFilters{
		Keyword:    strings.TrimSpace(c.Query("q")),
		Module:     strings.TrimSpace(c.Query("module")),
		Action:     strings.TrimSpace(c.Query("action")),
		Result:     strings.TrimSpace(c.Query("result")),
		TargetType: strings.TrimSpace(c.Query("target_type")),
		TargetID:   strings.TrimSpace(c.Query("target_id")),
	}
	if raw := strings.TrimSpace(c.Query("actor_user_id")); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			return adminAuditFilters{}, fmt.Errorf("操作人参数错误")
		}
		value := uint(id)
		filters.ActorUserID = &value
	}
	for _, field := range []struct {
		name   string
		target **time.Time
	}{
		{name: "start", target: &filters.Start},
		{name: "end", target: &filters.End},
	} {
		raw := strings.TrimSpace(c.Query(field.name))
		if raw == "" {
			continue
		}
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			if field.name == "start" {
				return adminAuditFilters{}, fmt.Errorf("开始时间参数错误")
			}
			return adminAuditFilters{}, fmt.Errorf("结束时间参数错误")
		}
		*field.target = &value
	}
	return filters, nil
}

func applyAdminAuditFilters(query *gorm.DB, filters adminAuditFilters) *gorm.DB {
	for _, filter := range []struct {
		column string
		value  string
	}{
		{column: "module", value: filters.Module},
		{column: "action", value: filters.Action},
		{column: "result", value: filters.Result},
		{column: "target_type", value: filters.TargetType},
	} {
		if filter.value != "" {
			query = query.Where(filter.column+" = ?", filter.value)
		}
	}
	if filters.TargetID != "" {
		query = query.Where("target_id = ? AND (module != 'notes' OR target_type != 'message' OR result NOT IN ('denied', 'failure') OR reason = ?)", filters.TargetID, string(authorization.DenialMissingGrant))
	}
	if filters.ActorUserID != nil {
		query = query.Where("actor_user_id = ?", *filters.ActorUserID)
	}
	if filters.Start != nil {
		query = query.Where("created_at >= ?", *filters.Start)
	}
	if filters.End != nil {
		query = query.Where("created_at <= ?", *filters.End)
	}
	if filters.Keyword != "" {
		keyword := "%" + filters.Keyword + "%"
		query = query.Where("(module != 'notes' OR target_type != 'message' OR result NOT IN ('denied', 'failure') OR reason = ?) AND (summary LIKE ? OR reason LIKE ? OR target_id LIKE ?)", string(authorization.DenialMissingGrant), keyword, keyword, keyword)
	}
	return query
}

type adminAuditLogView struct {
	models.AdminAuditLog
	authorization.AuditPresentation
}

func sanitizeAdminAuditLog(log models.AdminAuditLog) models.AdminAuditLog {
	if log.Module != "notes" || log.TargetType != "message" || (log.Result != "denied" && log.Result != "failure") {
		return log
	}
	// Denied message mutations are not a safe way to identify content. Keep
	// target identity only for an explicit, visible missing-grant denial;
	// legacy rows without a reason are redacted defensively as well.
	if log.Reason == string(authorization.DenialMissingGrant) {
		return log
	}
	log.TargetID = ""
	log.TargetOwnerUserID = nil
	log.Summary = "capability request denied"
	log.ChangesJSON = ""
	return log
}

func presentAdminAuditLog(log models.AdminAuditLog) adminAuditLogView {
	log = sanitizeAdminAuditLog(log)
	return adminAuditLogView{AdminAuditLog: log, AuditPresentation: authorization.PresentAudit(log)}
}

func presentAdminAuditLogs(logs []models.AdminAuditLog) []adminAuditLogView {
	views := make([]adminAuditLogView, 0, len(logs))
	for _, log := range logs {
		views = append(views, presentAdminAuditLog(log))
	}
	return views
}
