package controllers

import (
	"testing"

	backupservice "github.com/rcy1314/echo-noise/internal/backup"
)

func TestStagedRestoreResponseIncludesIncompleteMigrationWarning(t *testing.T) {
	response := stagedRestoreResponse("备份已暂存", backupservice.StageResult{Warning: "该备份不含附件，不能用于完整迁移"})
	if response["code"] != 1 || response["pendingRestart"] != true {
		t.Fatalf("staged response = %#v", response)
	}
	if response["warning"] != "该备份不含附件，不能用于完整迁移" || response["completeMigration"] != false {
		t.Fatalf("migration warning response = %#v", response)
	}
}

func TestStagedRestoreResponseDoesNotWarnForCompleteArchive(t *testing.T) {
	response := stagedRestoreResponse("备份已暂存", backupservice.StageResult{})
	if _, exists := response["warning"]; exists {
		t.Fatalf("complete restore response unexpectedly warns: %#v", response)
	}
	if _, exists := response["completeMigration"]; exists {
		t.Fatalf("complete restore response unexpectedly marks itself incomplete: %#v", response)
	}
}
