package updates

import (
	"strings"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestInstallationRequiresFreshPairedSupportedDeployment(t *testing.T) {
	db := openTaskTestDB(t, ":memory:")
	s := NewTaskService(db)
	revision := strings.Repeat("1", 40)
	assertReason := func(want string) {
		t.Helper()
		got, err := s.InstallationStatus(revision)
		if err != nil || got.Reason != want || got.Available != (want == "") {
			t.Fatalf("want=%s got=%+v err=%v", want, got, err)
		}
	}
	assertReason("executor_unconfigured")
	c, token, err := s.CreateCredential(1, "host")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(token); err != nil {
		t.Fatal(err)
	}
	assertReason("deployment_unchecked")
	instance, _ := s.InstanceID()
	check := DeploymentCheck{InstanceID: instance, Version: ExecutorVersion, Platform: "linux/amd64", Revision: revision, OK: true}
	bad := check
	bad.InstanceID = strings.Repeat("f", 32)
	if err := s.RecordDeploymentCheck(c.ID, bad); err == nil {
		t.Fatal("unpaired check accepted")
	}
	for _, field := range []string{"version", "platform"} {
		bad = check
		if field == "version" {
			bad.Version = "private host text"
		} else {
			bad.Platform = "private host text"
		}
		if err := s.RecordDeploymentCheck(c.ID, bad); err == nil {
			t.Fatalf("free text accepted as %s", field)
		}
	}
	if err := s.RecordDeploymentCheck(c.ID, check); err != nil {
		t.Fatal(err)
	}
	assertReason("")
	for _, tc := range []struct {
		column string
		value  any
		reason string
	}{
		{"executor_version", "u4-1", "executor_upgrade_required"},
		{"platform", "linux/arm64", "platform_unsupported"},
		{"check_ok", false, "deployment_check_failed"},
		{"installed_revision", strings.Repeat("2", 40), "deployment_unchecked"},
		{"instance_id", "other", "instance_mismatch"},
		{"checked_at", time.Now().Add(-4 * time.Minute), "executor_offline"},
		{"expires_at", time.Now().Add(-time.Minute), "credential_expired"},
	} {
		if err := db.Model(&c).Update(tc.column, tc.value).Error; err != nil {
			t.Fatal(err)
		}
		// Authentication never extends deployment-check freshness.
		if tc.reason == "executor_offline" {
			if _, err := s.Authenticate(token); err != nil {
				t.Fatal(err)
			}
		}
		assertReason(tc.reason)
		if err := db.Model(&c).Update("expires_at", nil).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.RecordDeploymentCheck(c.ID, check); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.CreateCredential(1, "rotated"); err != nil {
		t.Fatal(err)
	}
	assertReason("deployment_unchecked")
	if err := s.RecordDeploymentCheck(c.ID, check); err == nil {
		t.Fatal("superseded check enabled new credential")
	}
	if err := s.RevokeCredential(1); err != nil {
		t.Fatal(err)
	}
	assertReason("executor_unconfigured")
	var count int64
	db.Model(&models.UpdateTask{}).Count(&count)
	if count != 0 {
		t.Fatal("checking created tasks")
	}
}
