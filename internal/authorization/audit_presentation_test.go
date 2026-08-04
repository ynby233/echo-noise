package authorization

import (
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestPresentAuditDoesNotDuplicateReadVerbForAuditCapability(t *testing.T) {
	presentation := PresentAudit(models.AdminAuditLog{
		Capability: string(CapabilityAuditView),
		Module:     "audit",
		Action:     "GET",
		Result:     "denied",
		Reason:     string(DenialMissingGrant),
	})

	if got, want := presentation.OperationDescription, "尝试查看管理员审计"; got != want {
		t.Fatalf("operation=%q, want %q", got, want)
	}
}

func TestPresentAuditExplainsGlobalPinDenialInChinese(t *testing.T) {
	presentation := PresentAudit(models.AdminAuditLog{
		ActorUsername: "ceshi",
		Capability:    string(CapabilityNotesPinGlobal),
		Module:        "notes",
		Action:        "set_global_pin",
		TargetType:    "message",
		TargetID:      "59",
		Result:        "denied",
		Reason:        string(DenialMissingGrant),
	})

	if presentation.OperationDescription != "尝试全站置顶笔记 #59" {
		t.Fatalf("unexpected operation description: %q", presentation.OperationDescription)
	}
	if presentation.ResultDescription != "已拒绝：无此权限" {
		t.Fatalf("unexpected result description: %q", presentation.ResultDescription)
	}
}

func TestPresentAuditCoversKnownOperationsAndSafeFallbacks(t *testing.T) {
	tests := []struct {
		name       string
		record     models.AdminAuditLog
		operation  string
		result     string
		safeSummary string
	}{
		{
			name: "global pin success",
			record: models.AdminAuditLog{Capability: string(CapabilityNotesPinGlobal), Action: "set_global_pin", TargetType: "message", TargetID: "59", Result: "success", Summary: "global pin mutation completed"},
			operation: "已将笔记 #59设为全站置顶", result: "成功", safeSummary: "全站置顶操作完成",
		},
		{
			name: "protected content denial",
			record: models.AdminAuditLog{Capability: string(CapabilityNotesPinGlobal), Action: "set_global_pin", TargetType: "message", TargetID: "290", Result: "denied", Reason: string(DenialProtectedContent)},
			operation: "尝试修改受保护笔记 #290", result: "已拒绝：受保护内容不可管理",
		},
		{
			name: "global pin cancellation",
			record: models.AdminAuditLog{Capability: string(CapabilityNotesPinGlobal), Action: "unset_global_pin", TargetType: "message", TargetID: "59", Result: "success"},
			operation: "已取消笔记 #59的全站置顶", result: "成功",
		},
		{
			name: "security read denial",
			record: models.AdminAuditLog{Capability: string(CapabilitySecurityView), Module: "security", Action: "GET", TargetType: "route", TargetID: "/security", Result: "denied", Reason: string(DenialMissingGrant)},
			operation: "尝试查看安全防护配置", result: "已拒绝：无此权限",
		},
		{
			name: "audit detail view",
			record: models.AdminAuditLog{Capability: string(CapabilityAuditView), Action: "view_detail", TargetType: "admin_audit_log", TargetID: "12", Result: "success"},
			operation: "已查看管理员审计记录 #12", result: "成功",
		},
		{
			name: "authorization change",
			record: models.AdminAuditLog{Capability: string(CapabilityAuthorizationManage), Action: "replace_grants", TargetType: "user", TargetID: "7", Result: "success", Summary: "replaced delegated administrator capabilities"},
			operation: "已调整受托管理员用户 #7的能力授权", result: "成功", safeSummary: "受托管理员能力授权已调整",
		},
		{
			name: "unknown summary",
			record: models.AdminAuditLog{Module: "unknown", Action: "POST", TargetType: "route", TargetID: "/internal", Result: "failure", Summary: "token=secret-value"},
			operation: "尝试执行模块 unknown（POST）管理操作", result: "失败", safeSummary: "管理员操作摘要",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			presentation := PresentAudit(tt.record)
			if presentation.OperationDescription != tt.operation {
				t.Fatalf("operation=%q, want %q", presentation.OperationDescription, tt.operation)
			}
			if presentation.ResultDescription != tt.result {
				t.Fatalf("result=%q, want %q", presentation.ResultDescription, tt.result)
			}
			if tt.safeSummary != "" && presentation.SafeSummary != tt.safeSummary {
				t.Fatalf("safe summary=%q, want %q", presentation.SafeSummary, tt.safeSummary)
			}
			if presentation.SafeSummary == "token=secret-value" {
				t.Fatal("safe summary must not expose an arbitrary sensitive-looking summary")
			}
		})
	}
}
