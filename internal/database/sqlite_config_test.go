package database

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSQLiteConnectionPolicySerializesWritesAndWaitsForLocks(t *testing.T) {
	dsn := sqliteConnectionString(t.TempDir() + "/policy.db")
	for _, required := range []string{"_pragma=busy_timeout%2810000%29", "_pragma=journal_mode%28WAL%29", "_pragma=synchronous%28NORMAL%29"} {
		if !strings.Contains(dsn, required) {
			t.Fatalf("sqlite DSN %q missing %q", dsn, required)
		}
	}

	db := &sql.DB{}
	configureConnectionPool(db, "sqlite")
	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("sqlite max open connections = %d, want 1 to serialize writes", got)
	}
}

func TestSQLiteConnectionPolicySustainsRapidConcurrentWrites(t *testing.T) {
	type row struct {
		ID    uint `gorm:"primaryKey"`
		Value string
	}
	db, err := gorm.Open(sqlite.Open(sqliteConnectionString(t.TempDir()+"/rapid-writes.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	configureConnectionPool(sqlDB, "sqlite")
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&row{}); err != nil {
		t.Fatal(err)
	}

	var journalMode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil || strings.ToLower(journalMode) != "wal" {
		t.Fatalf("journal mode=%q err=%v, want wal", journalMode, err)
	}
	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil || busyTimeout != 10000 {
		t.Fatalf("busy timeout=%d err=%v, want 10000", busyTimeout, err)
	}

	const writes = 40
	errorsByWrite := make(chan error, writes)
	var wait sync.WaitGroup
	for index := 0; index < writes; index++ {
		wait.Add(1)
		go func(value int) {
			defer wait.Done()
			errorsByWrite <- db.Transaction(func(tx *gorm.DB) error {
				return tx.Create(&row{Value: fmt.Sprintf("write-%d", value)}).Error
			})
		}(index)
	}
	wait.Wait()
	close(errorsByWrite)
	for writeErr := range errorsByWrite {
		if writeErr != nil {
			t.Fatalf("rapid write failed: %v", writeErr)
		}
	}
	var count int64
	if err := db.Model(&row{}).Count(&count).Error; err != nil || count != writes {
		t.Fatalf("rapid write count=%d err=%v, want %d", count, err, writes)
	}
}
