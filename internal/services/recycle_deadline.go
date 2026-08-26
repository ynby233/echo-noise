package services

import "time"

type RecycleDeadline struct {
	AutoCleanupEnabled  bool       `json:"auto_cleanup_enabled"`
	ScheduledDeletionAt *time.Time `json:"scheduled_deletion_at,omitempty"`
	RemainingSeconds    int64      `json:"remaining_seconds"`
	RemainingDays       int        `json:"remaining_days"`
	RemainingHours      int        `json:"remaining_hours"`
	Expired             bool       `json:"expired"`
}

// CalculateRecycleDeadline is the shared source of truth for personal and
// administrative recycle-bin countdowns. It is recalculated for every read so
// changing the retention policy immediately changes all displayed deadlines.
func CalculateRecycleDeadline(deletedAt *time.Time, retentionDays int, now time.Time) RecycleDeadline {
	result := RecycleDeadline{}
	if deletedAt == nil || retentionDays <= 0 {
		return result
	}
	deadline := deletedAt.UTC().AddDate(0, 0, retentionDays)
	result.AutoCleanupEnabled = true
	result.ScheduledDeletionAt = &deadline
	remaining := deadline.Sub(now.UTC())
	if remaining <= 0 {
		result.Expired = true
		return result
	}
	seconds := int64(remaining / time.Second)
	result.RemainingSeconds = seconds
	totalHours := int(seconds / int64(time.Hour/time.Second))
	result.RemainingDays = totalHours / 24
	result.RemainingHours = totalHours % 24
	return result
}
