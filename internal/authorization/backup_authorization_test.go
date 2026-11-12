package authorization

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestBackupCatalogContract(t *testing.T) {
	for _, kind := range []struct {
		key    string
		module string
		label  string
	}{
		{"data", "backup-data", "数据备份"},
		{"attachments", "backup-attachments", "附件备份"},
		{"server_config", "backup-server-config", "服务端配置备份"},
	} {
		for _, action := range []struct {
			key   string
			label string
		}{
			{"view", "查看" + kind.label},
			{"history", "查看" + kind.label + "记录"},
			{"create", "生成" + kind.label},
			{"plan", "管理" + kind.label + "计划"},
			{"download", "下载" + kind.label},
			{"restore", "恢复" + kind.label},
		} {
			capability := Capability("backup." + kind.key + "." + action.key)
			definition, ok := DefinitionFor(capability)
			grantable := kind.key != "server_config" && action.key != "download" && action.key != "restore"
			label := action.label
			if !grantable {
				label += "（仅站长）"
			}
			if !ok || definition.Module != kind.module || definition.Label != label || definition.Grantable != grantable {
				t.Fatalf("catalog %s = %#v, known=%v; want module=%s label=%s grantable=%v", capability, definition, ok, kind.module, label, grantable)
			}
			parent, hasParent := ParentCapabilityFor(capability)
			if action.key == "view" {
				if hasParent {
					t.Errorf("view %s unexpectedly has parent %s", capability, parent)
				}
			} else if !hasParent || parent != Capability("backup."+kind.key+".view") {
				t.Errorf("parent for %s = %s, %v", capability, parent, hasParent)
			}
			payload, err := json.Marshal(definition)
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Parent Capability `json:"parent_capability"`
			}
			if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Parent != parent {
				t.Fatalf("serialized parent for %s: %s, error=%v", capability, payload, err)
			}
		}
	}
	for _, retired := range []Capability{"database.view", "database.backup", "database.restore"} {
		if _, ok := DefinitionFor(retired); ok {
			t.Errorf("retired capability remains in catalog: %s", retired)
		}
	}
	for capability, label := range map[Capability]string{
		CapabilityStorageView:   "查看附件存储与处理",
		CapabilityStorageManage: "管理附件处理与安全压缩",
	} {
		definition, ok := DefinitionFor(capability)
		if !ok || !definition.Grantable || definition.Label != label {
			t.Errorf("safe attachment processing must remain grantable: %#v", definition)
		}
	}
}

func TestBackupDelegationRequiresViewAndCannotExport(t *testing.T) {
	for _, kind := range []string{"data", "attachments"} {
		for _, action := range []string{"history", "create", "plan"} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				db, authorizer, primary, delegated, _ := setupAuthorizerTest(t)
				view := Capability("backup." + kind + ".view")
				child := Capability("backup." + kind + "." + action)
				if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []Capability{child}); err == nil {
					t.Fatal("child grant without view accepted")
				}
				if err := db.Create(&models.AdminCapabilityGrant{UserID: delegated.ID, Capability: string(child), GrantedByUserID: primary.ID}).Error; err != nil {
					t.Fatal(err)
				}
				if decision := authorizer.Authorize(delegated.ID, child, nil); decision.Reason != DenialMissingPrerequisite {
					t.Fatalf("orphan child authorized: %#v", decision)
				}
				if snapshot, err := authorizer.CapabilitiesFor(delegated.ID); err != nil || len(snapshot) != 0 {
					t.Fatalf("orphan child snapshot = %v, error=%v", snapshot, err)
				}
				if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []Capability{view, child}); err != nil {
					t.Fatal(err)
				}
				if decision := authorizer.Authorize(delegated.ID, child, nil); !decision.Allowed {
					t.Fatalf("valid grant denied: %#v", decision)
				}
				for _, export := range []string{"download", "restore"} {
					if decision := authorizer.Authorize(delegated.ID, Capability("backup."+kind+"."+export), nil); decision.Reason != DenialNotGrantable {
						t.Fatalf("creation or history grant allowed export: %#v", decision)
					}
				}
			})
		}
	}
}

func TestBackupOwnerOnlyDirtyGrantsAndRetiredGrantsCannotAuthorize(t *testing.T) {
	db, authorizer, primary, delegated, _ := setupAuthorizerTest(t)
	var forbidden []Capability
	for _, definition := range Catalog() {
		if len(definition.Capability) >= len("backup.") && string(definition.Capability[:len("backup.")]) == "backup." && !definition.Grantable {
			forbidden = append(forbidden, definition.Capability)
		}
	}
	forbidden = append(forbidden, "database.view", "database.backup", "database.restore")
	for _, capability := range forbidden {
		if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []Capability{capability}); err == nil {
			t.Fatalf("forbidden capability accepted: %s", capability)
		}
		if err := db.Create(&models.AdminCapabilityGrant{UserID: delegated.ID, Capability: string(capability), GrantedByUserID: primary.ID}).Error; err != nil {
			t.Fatal(err)
		}
		want := DenialNotGrantable
		if _, known := DefinitionFor(capability); !known {
			want = DenialUnknownCapability
		}
		if decision := authorizer.Authorize(delegated.ID, capability, nil); decision.Allowed || decision.Reason != want {
			t.Fatalf("dirty grant %s: %#v, want %s", capability, decision, want)
		}
	}
	if snapshot, err := authorizer.CapabilitiesFor(delegated.ID); err != nil || len(snapshot) != 0 {
		t.Fatalf("dirty grant snapshot = %v, error=%v", snapshot, err)
	}
	for _, kind := range []string{"data", "attachments", "server_config"} {
		for _, action := range []string{"view", "history", "create", "plan", "download", "restore"} {
			capability := Capability(fmt.Sprintf("backup.%s.%s", kind, action))
			if decision := authorizer.Authorize(primary.ID, capability, nil); !decision.Allowed {
				t.Errorf("primary denied %s: %#v", capability, decision)
			}
		}
	}
}
