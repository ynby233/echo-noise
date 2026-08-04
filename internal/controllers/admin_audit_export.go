package controllers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
)

func ExportAdminAuditLogs(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("审计服务不可用"))
		return
	}
	filters, err := parseAdminAuditFilters(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any](err.Error()))
		return
	}
	rows, err := applyAdminAuditFilters(db.Model(&models.AdminAuditLog{}), filters).
		Order("created_at DESC, id DESC").Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("导出审计失败"))
		return
	}
	defer rows.Close()

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("admin-audit-%s.csv", timestamp)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if _, err := c.Writer.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
		return
	}

	writer := csv.NewWriter(c.Writer)
	if err := writer.Write([]string{"筛选条件", auditFilterSummary(filters)}); err != nil {
		return
	}
	if err := writer.Write([]string{"时间", "操作人", "操作说明", "结果", "模块", "动作", "能力", "摘要", "目标类型", "目标 ID", "拒绝原因"}); err != nil {
		return
	}
	for rows.Next() {
		var log models.AdminAuditLog
		if err := db.ScanRows(rows, &log); err != nil {
			c.Error(err)
			return
		}
		presentation := authorization.PresentAudit(log)
		if err := writer.Write([]string{
			formatAuditExportTime(log.CreatedAt),
			log.ActorUsername,
			presentation.OperationDescription,
			presentation.ResultDescription,
			log.Module,
			log.Action,
			log.Capability,
			presentation.SafeSummary,
			log.TargetType,
			log.TargetID,
			presentation.ReasonDescription,
		}); err != nil {
			return
		}
	}
	if err := rows.Err(); err != nil {
		c.Error(err)
		return
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		c.Error(err)
	}
}

func formatAuditExportTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func auditFilterSummary(filters adminAuditFilters) string {
	parts := make([]string, 0, 9)
	if filters.Keyword != "" {
		parts = append(parts, "关键词已设置")
	}
	for _, filter := range []struct {
		label string
		value string
	}{
		{label: "操作人 ID", value: formatAuditFilterActor(filters.ActorUserID)},
		{label: "模块", value: filters.Module},
		{label: "动作", value: filters.Action},
		{label: "结果", value: filters.Result},
		{label: "目标类型", value: filters.TargetType},
		{label: "目标 ID", value: filters.TargetID},
		{label: "开始时间", value: formatAuditFilterTime(filters.Start)},
		{label: "结束时间", value: formatAuditFilterTime(filters.End)},
	} {
		if filter.value != "" {
			parts = append(parts, filter.label+"="+filter.value)
		}
	}
	if len(parts) == 0 {
		return "未设置（全部记录）"
	}
	return strings.Join(parts, "；")
}

func formatAuditFilterActor(actorID *uint) string {
	if actorID == nil {
		return ""
	}
	return fmt.Sprint(*actorID)
}

func formatAuditFilterTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}
