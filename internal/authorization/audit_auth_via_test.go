package authorization

import (
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestAuditAuthViaNormalizesWithoutChangingActorType(t *testing.T) {
	db, authorizer, primary, _, ordinary := setupAuthorizerTest(t)
	for _, tc := range []struct {
		actorType string
		via       string
		want      string
	}{
		{"user", "session", "session"},
		{"user", "token", "token"},
		{"user", "ai_token", "ai_token"},
		{"user", "", "unknown"},
		{"user", "forged", "unknown"},
		{"user", "system", "unknown"},
		{"system", "system", "unknown"},
		{"system", "ai_token", "unknown"},
	} {
		err := authorizer.WriteAudit(models.AdminAuditLog{
			ActorType: tc.actorType, ActorUserID: primary.ID, AuthVia: tc.via,
			Module: "notes", Action: "update", Result: "success",
		})
		if err != nil {
			t.Fatal(err)
		}
		var record models.AdminAuditLog
		if err := db.Order("id DESC").First(&record).Error; err != nil {
			t.Fatal(err)
		}
		if record.AuthVia != tc.want || record.ActorType != tc.actorType {
			t.Fatalf("actor=%s via=%q: got actor=%s via=%q", tc.actorType, tc.via, record.ActorType, record.AuthVia)
		}
		if tc.actorType == "system" {
			if record.ActorUserID != 0 || record.ActorIsPrimary || record.ActorUsername != "系统自动任务" {
				t.Fatalf("system actor must retain its independent meaning: %#v", record)
			}
		} else if record.ActorUserID != primary.ID || !record.ActorIsPrimary {
			t.Fatal("normalizing source must preserve the user actor")
		}
	}
	var before, after int64
	if err := db.Model(&models.AdminAuditLog{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := authorizer.WriteAudit(models.AdminAuditLog{ActorUserID: ordinary.ID, AuthVia: "ai_token", Module: "notes", Action: "update"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AdminAuditLog{}).Count(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatal("AI source must not turn ordinary-user activity into administrator audit")
	}
}
