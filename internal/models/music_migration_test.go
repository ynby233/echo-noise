package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMusicMigrationSeedsDefaultsAndPreservesExistingConfiguration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := MigrateDB(db); err != nil {
		t.Fatal(err)
	}
	for _, model := range []interface{}{&MusicConfig{}, &MusicTrack{}, &MusicPlaylistItem{}, &MusicScanState{}} {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("missing music table %T", model)
		}
	}
	var config MusicConfig
	var scan MusicScanState
	if err := db.First(&config, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&scan, 1).Error; err != nil {
		t.Fatal(err)
	}
	if config.Source != "netease" || config.Version != 1 || config.ScanIntervalMinutes != 60 || scan.State != "idle" {
		t.Fatalf("defaults: config=%#v scan=%#v", config, scan)
	}
	if err := db.Model(&config).Updates(map[string]interface{}{"source": "local", "version": 7, "scan_interval_minutes": 30}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&scan).Update("state", "failed").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDB(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&config, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&scan, 1).Error; err != nil {
		t.Fatal(err)
	}
	if config.Source != "local" || config.Version != 7 || config.ScanIntervalMinutes != 30 || scan.State != "failed" {
		t.Fatalf("migration reset existing music data: config=%#v scan=%#v", config, scan)
	}
}
