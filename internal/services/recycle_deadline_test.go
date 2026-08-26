package services

import (
	"testing"
	"time"
)

func TestCalculateRecycleDeadlineUsesCurrentPolicyAndRealTime(t *testing.T) {
	deletedAt := time.Date(2026, 8, 1, 10, 30, 0, 0, time.UTC)
	now := time.Date(2026, 8, 3, 12, 30, 0, 0, time.UTC)
	got := CalculateRecycleDeadline(&deletedAt, 7, now)
	if !got.AutoCleanupEnabled || got.ScheduledDeletionAt == nil {
		t.Fatalf("deadline = %#v", got)
	}
	if got.RemainingDays != 4 || got.RemainingHours != 22 {
		t.Fatalf("remaining = %d days %d hours, want 4 days 22 hours", got.RemainingDays, got.RemainingHours)
	}
	longer := CalculateRecycleDeadline(&deletedAt, 30, now)
	if longer.RemainingDays <= got.RemainingDays {
		t.Fatalf("current policy did not change countdown: seven=%#v thirty=%#v", got, longer)
	}
	disabled := CalculateRecycleDeadline(&deletedAt, 0, now)
	if disabled.AutoCleanupEnabled || disabled.ScheduledDeletionAt != nil {
		t.Fatalf("disabled retention = %#v", disabled)
	}
}
