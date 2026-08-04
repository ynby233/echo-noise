package authorization

import (
	"fmt"
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
)

// AuditPresentation contains stable, user-facing text derived from the
// append-only audit fields. It deliberately uses only safe target metadata;
// it never reads note titles or content.
type AuditPresentation struct {
	OperationDescription string `json:"operation_description"`
	ResultDescription    string `json:"result_description"`
	ReasonDescription    string `json:"reason_description"`
	SafeSummary          string `json:"-"`
}

func PresentAudit(record models.AdminAuditLog) AuditPresentation {
	return AuditPresentation{
		OperationDescription: auditOperationDescription(record),
		ResultDescription:    auditResultDescription(record.Result, record.Reason),
		ReasonDescription:    auditReasonDescription(record.Reason),
		SafeSummary:          auditSafeSummary(record.Summary),
	}
}

func auditOperationDescription(record models.AdminAuditLog) string {
	target := auditTargetDescription(record.TargetType, record.TargetID)
	prefix := "尝试"
	if record.Result == "success" {
		prefix = "已"
	}

	switch {
	case record.Capability == string(CapabilityNotesPinGlobal) && record.Action == "set_global_pin":
		if record.Reason == string(DenialProtectedContent) {
			return fmt.Sprintf("尝试修改受保护%s", target)
		}
		if record.Result == "success" {
			return fmt.Sprintf("已将%s设为全站置顶", target)
		}
		return fmt.Sprintf("尝试全站置顶%s", target)
	case record.Capability == string(CapabilityNotesPinGlobal) && record.Action == "unset_global_pin":
		if record.Result == "success" {
			return fmt.Sprintf("已取消%s的全站置顶", target)
		}
		return fmt.Sprintf("尝试取消%s的全站置顶", target)
	case record.Capability == string(CapabilityAuthorizationManage) && record.Action == "replace_grants":
		return fmt.Sprintf("%s调整受托管理员%s的能力授权", prefix, target)
	case record.Capability == string(CapabilityAuditView) && record.Action == "view_detail":
		return fmt.Sprintf("%s查看管理员%s", prefix, target)
	case isAuditReadMethod(record.Action):
		return fmt.Sprintf("%s查看%s", prefix, auditModuleDescription(record.Module, record.Capability))
	case record.Action != "":
		return fmt.Sprintf("%s执行%s%s管理操作", prefix, auditModuleDescription(record.Module, record.Capability), auditActionDescription(record.Action))
	default:
		return fmt.Sprintf("%s执行管理员操作%s", prefix, targetSuffix(target))
	}
}

func auditResultDescription(result string, reason string) string {
	switch result {
	case "success":
		return "成功"
	case "denied":
		if description := auditReasonDescription(reason); description != "" {
			return "已拒绝：" + description
		}
		return "已拒绝"
	case "failure":
		return "失败"
	default:
		return "结果待确认"
	}
}

func auditReasonDescription(reason string) string {
	switch strings.TrimSpace(reason) {
	case string(DenialMissingGrant):
		return "无此权限"
	case string(DenialProtectedContent):
		return "受保护内容不可管理"
	case string(DenialNotAdministrator):
		return "需要管理员权限"
	case string(DenialUnknownCapability):
		return "未识别的管理能力"
	case "":
		return ""
	default:
		return "拒绝原因未记录"
	}
}

func auditSafeSummary(summary string) string {
	switch strings.TrimSpace(summary) {
	case "capability request denied":
		return "能力请求被拒绝"
	case "administrative write completed":
		return "管理操作完成"
	case "administrative write failed":
		return "管理操作失败"
	case "message mutation completed":
		return "笔记管理操作完成"
	case "global pin mutation completed":
		return "全站置顶操作完成"
	case "global pin mutation failed":
		return "全站置顶操作失败"
	case "viewed administrator audit detail":
		return "查看管理员审计详情"
	case "replaced delegated administrator capabilities":
		return "受托管理员能力授权已调整"
	default:
		return "管理员操作摘要"
	}
}

func isAuditReadMethod(action string) bool {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "GET", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func auditModuleDescription(module string, capability string) string {
	if definition, ok := DefinitionFor(Capability(capability)); ok && definition.Module == module {
		switch module {
		case "security":
			return "安全防护配置"
		case "notifications":
			return "通知设置"
		default:
			return strings.TrimSuffix(definition.Label, "查看")
		}
	}
	labels := map[string]string{
		"account_security": "账户安全",
		"audit":            "管理员审计",
		"users":            "用户信息",
		"registration":     "注册申请",
		"comments":         "评论",
		"attachments":      "附件",
		"storage":          "存储",
		"database":         "数据库",
		"version":          "版本设置",
		"security":         "安全防护配置",
		"access_logs":      "访问日志",
		"site_visits":      "访客记录",
		"login_audits":     "登录审计",
		"site":             "站点设置",
		"announcements":    "公告",
		"feed":             "信息流",
		"rss":              "RSS",
		"notifications":    "通知设置",
		"email":            "邮件设置",
		"notes":            "笔记",
	}
	if label := labels[module]; label != "" {
		return label
	}
	if strings.TrimSpace(module) != "" {
		return "模块 " + strings.TrimSpace(module)
	}
	return "管理模块"
}

func auditActionDescription(action string) string {
	if action == "" {
		return ""
	}
	return "（" + action + "）"
}

func auditTargetDescription(targetType string, targetID string) string {
	label := map[string]string{
		"message":         "笔记",
		"user":            "用户",
		"admin_audit_log": "审计记录",
		"route":           "路由",
	}
	name := label[targetType]
	if name == "" {
		name = strings.TrimSpace(targetType)
	}
	if name == "" {
		name = "目标"
	}
	if id := strings.TrimSpace(targetID); id != "" {
		return fmt.Sprintf("%s #%s", name, id)
	}
	return name
}

func targetSuffix(target string) string {
	if strings.TrimSpace(target) == "" {
		return ""
	}
	return "（" + target + "）"
}
