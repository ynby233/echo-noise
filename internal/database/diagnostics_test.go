package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSnapshotConnectionPoolReportsExhaustionAndWaits(t *testing.T) {
	dsn := fmt.Sprintf("file:pool-diagnostics-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)

	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("occupy only database connection: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err == nil {
		t.Fatal("ping unexpectedly acquired an exhausted connection")
	}

	snapshot, ok := SnapshotConnectionPool(db)
	if !ok {
		t.Fatal("connection pool snapshot unavailable")
	}
	if snapshot.MaxOpen != 1 || snapshot.Open != 1 || snapshot.InUse != 1 || snapshot.Idle != 0 {
		t.Fatalf("unexpected pool snapshot: %+v", snapshot)
	}
	if snapshot.WaitCount < 1 || snapshot.WaitDuration <= 0 {
		t.Fatalf("pool wait was not recorded: %+v", snapshot)
	}
}
