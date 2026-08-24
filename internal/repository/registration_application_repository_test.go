package repository

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func TestPermanentRegistrationNumberSurvivesDatabaseRestartAndDeletion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "registration-restart.db")
	openDatabase := func() (*gorm.DB, error) {
		db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
		if err != nil {
			return nil, err
		}
		if err := models.MigrateDB(db); err != nil {
			return nil, err
		}
		return db, nil
	}

	db, err := openDatabase()
	if err != nil {
		t.Fatalf("open first database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() {
		if database.DB != nil {
			if sqlDB, err := database.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		database.DB = nil
		models.SetDB(nil)
	})

	first := models.RegistrationApplication{Username: "first", PasswordHash: "hash", Status: models.RegistrationApplicationStatusPending}
	if err := CreateRegistrationApplicationWithPermanentNumber(&first, "vc.example"); err != nil {
		t.Fatalf("allocate first application: %v", err)
	}
	if first.ApplicationID != "1" || first.VoceChatCandidateEmail != "1@vc.example" {
		t.Fatalf("first allocation = %#v", first)
	}
	if sqlDB, err := db.DB(); err != nil {
		t.Fatalf("get first sql database: %v", err)
	} else if err := sqlDB.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	db, err = openDatabase()
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	second := models.RegistrationApplication{Username: "second", PasswordHash: "hash", Status: models.RegistrationApplicationStatusPending}
	if err := CreateRegistrationApplicationWithPermanentNumber(&second, "vc.example"); err != nil {
		t.Fatalf("allocate after restart: %v", err)
	}
	if second.ApplicationID != "2" {
		t.Fatalf("allocation after restart = %q, want 2", second.ApplicationID)
	}
	if err := db.Delete(&second).Error; err != nil {
		t.Fatalf("delete highest application: %v", err)
	}
	third := models.RegistrationApplication{Username: "third", PasswordHash: "hash", Status: models.RegistrationApplicationStatusPending}
	if err := CreateRegistrationApplicationWithPermanentNumber(&third, "vc.example"); err != nil {
		t.Fatalf("allocate after deletion: %v", err)
	}
	if third.ApplicationID != "3" {
		t.Fatalf("allocation after deletion = %q, want 3", third.ApplicationID)
	}
}
