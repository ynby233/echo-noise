package database

import (
	"log"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

const poolPressureLogInterval = 30 * time.Second

// ConnectionPoolSnapshot is the small, driver-independent subset of sql.DBStats
// needed to diagnose connection starvation without exposing database details.
type ConnectionPoolSnapshot struct {
	MaxOpen      int
	Open         int
	InUse        int
	Idle         int
	WaitCount    int64
	WaitDuration time.Duration
}

var lastPoolPressureLog atomic.Int64

// SnapshotConnectionPool returns current pool pressure for diagnostics.
func SnapshotConnectionPool(db *gorm.DB) (ConnectionPoolSnapshot, bool) {
	if db == nil {
		return ConnectionPoolSnapshot{}, false
	}
	sqlDB, err := db.DB()
	if err != nil {
		return ConnectionPoolSnapshot{}, false
	}
	stats := sqlDB.Stats()
	return ConnectionPoolSnapshot{
		MaxOpen:      stats.MaxOpenConnections,
		Open:         stats.OpenConnections,
		InUse:        stats.InUse,
		Idle:         stats.Idle,
		WaitCount:    stats.WaitCount,
		WaitDuration: stats.WaitDuration,
	}, true
}

// LogConnectionPoolPressure emits a rate-limited, credential-free diagnostic
// line. It is intended for fail-fast paths where a database operation could
// not acquire a connection before its deadline.
func LogConnectionPoolPressure(operation string, db *gorm.DB) {
	now := time.Now()
	for {
		last := lastPoolPressureLog.Load()
		if last != 0 && now.Sub(time.Unix(0, last)) < poolPressureLogInterval {
			return
		}
		if lastPoolPressureLog.CompareAndSwap(last, now.UnixNano()) {
			break
		}
	}

	snapshot, ok := SnapshotConnectionPool(db)
	if !ok {
		log.Printf("database pool pressure operation=%s stats=unavailable", operation)
		return
	}
	log.Printf(
		"database pool pressure operation=%s max_open=%d open=%d in_use=%d idle=%d wait_count=%d wait_duration=%s",
		operation,
		snapshot.MaxOpen,
		snapshot.Open,
		snapshot.InUse,
		snapshot.Idle,
		snapshot.WaitCount,
		snapshot.WaitDuration,
	)
}
