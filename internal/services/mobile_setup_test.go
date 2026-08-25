package services

import (
	"errors"
	"sync"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestMobileSetupStateUsesPrimaryAdminInvariant(t *testing.T) {
	db := setupUserServiceTestDB(t)

	state, err := MobileSetupStateForDB(db)
	if err != nil || state != MobileSetupRequired {
		t.Fatalf("empty database state = %q, %v; want %q", state, err, MobileSetupRequired)
	}

	ordinary := models.User{ID: models.PrimaryAdminUserID, Username: "ordinary", Password: models.HashPassword("password")}
	if err := db.Create(&ordinary).Error; err != nil {
		t.Fatalf("create invalid primary row: %v", err)
	}
	state, err = MobileSetupStateForDB(db)
	if err != nil || state != MobileSetupInvalid {
		t.Fatalf("non-admin ID 1 state = %q, %v; want %q", state, err, MobileSetupInvalid)
	}

	if err := db.Model(&ordinary).Update("is_admin", true).Error; err != nil {
		t.Fatalf("promote explicit primary row: %v", err)
	}
	state, err = MobileSetupStateForDB(db)
	if err != nil || state != MobileSetupReady {
		t.Fatalf("valid ID 1 administrator state = %q, %v; want %q", state, err, MobileSetupReady)
	}
}

func TestInitializeMobileSiteOwnerIsSingleWinnerUnderConcurrency(t *testing.T) {
	db := setupUserServiceTestDB(t)
	results := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for _, username := range []string{"first_owner", "second_owner"} {
		waitGroup.Add(1)
		go func(username string) {
			defer waitGroup.Done()
			_, err := InitializeMobileSiteOwner(db, username, "strong-password")
			results <- err
		}(username)
	}
	waitGroup.Wait()
	close(results)

	successes := 0
	alreadyCompleted := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrMobileSetupAlreadyCompleted):
			alreadyCompleted++
		default:
			t.Fatalf("unexpected concurrent initialization error: %v", err)
		}
	}
	if successes != 1 || alreadyCompleted != 1 {
		t.Fatalf("concurrent results successes=%d already_completed=%d", successes, alreadyCompleted)
	}
}

func TestInitializeMobileSiteOwnerRollsBackAndCanRetry(t *testing.T) {
	db := setupUserServiceTestDB(t)
	callbackName := "test:fail-first-mobile-setup-message"
	failed := false
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if !failed && tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Message" {
			failed = true
			tx.AddError(errors.New("injected message failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := InitializeMobileSiteOwner(db, "retry_owner", "strong-password"); err == nil {
		t.Fatal("initialization unexpectedly succeeded despite injected dependent-row failure")
	}
	var users int64
	if err := db.Model(&models.User{}).Count(&users).Error; err != nil || users != 0 {
		t.Fatalf("failed transaction left users=%d err=%v", users, err)
	}
	if err := db.Callback().Create().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if _, err := InitializeMobileSiteOwner(db, "retry_owner", "strong-password"); err != nil {
		t.Fatalf("retry after rollback failed: %v", err)
	}
}

func TestInitializeMobileSiteOwnerCreatesPrimaryDependenciesAtomically(t *testing.T) {
	db := setupUserServiceTestDB(t)

	owner, err := InitializeMobileSiteOwner(db, "site_owner", "strong-password")
	if err != nil {
		t.Fatalf("initialize mobile site owner: %v", err)
	}
	if owner.ID != models.PrimaryAdminUserID || !owner.IsAdmin || owner.Username != "site_owner" {
		t.Fatalf("owner = %#v", owner)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(owner.Password), []byte("strong-password")); err != nil {
		t.Fatalf("stored password is not the submitted password: %v", err)
	}

	state, err := MobileSetupStateForDB(db)
	if err != nil || state != MobileSetupReady {
		t.Fatalf("initialized state = %q, %v; want %q", state, err, MobileSetupReady)
	}

	var guestbooks int64
	if err := db.Model(&models.Message{}).Where("is_guestbook = ? AND user_id = ?", true, models.PrimaryAdminUserID).Count(&guestbooks).Error; err != nil {
		t.Fatalf("count guestbooks: %v", err)
	}
	var examples int64
	if err := db.Model(&models.Message{}).Where("is_guestbook = ? AND user_id = ?", false, models.PrimaryAdminUserID).Count(&examples).Error; err != nil {
		t.Fatalf("count example messages: %v", err)
	}
	if guestbooks != 1 || examples == 0 {
		t.Fatalf("dependencies guestbooks=%d examples=%d", guestbooks, examples)
	}

	if _, err := InitializeMobileSiteOwner(db, "replacement", "another-password"); !errors.Is(err, ErrMobileSetupAlreadyCompleted) {
		t.Fatalf("repeat initialization error = %v, want %v", err, ErrMobileSetupAlreadyCompleted)
	}
}

func TestInitializeMobileSiteOwnerRejectsUnsafeOrInvalidDatabases(t *testing.T) {
	t.Run("default admin credentials", func(t *testing.T) {
		db := setupUserServiceTestDB(t)
		if _, err := InitializeMobileSiteOwner(db, "admin", "admin"); !errors.Is(err, ErrMobileSetupDefaultCredentials) {
			t.Fatalf("error = %v, want %v", err, ErrMobileSetupDefaultCredentials)
		}
	})

	t.Run("nonempty database without valid primary admin", func(t *testing.T) {
		db := setupUserServiceTestDB(t)
		if err := db.Create(&models.User{ID: 2, Username: "existing", Password: models.HashPassword("password")}).Error; err != nil {
			t.Fatalf("seed existing user: %v", err)
		}
		if _, err := InitializeMobileSiteOwner(db, "site_owner", "strong-password"); !errors.Is(err, ErrMobileSetupInvalidDatabase) {
			t.Fatalf("error = %v, want %v", err, ErrMobileSetupInvalidDatabase)
		}
		var primaryCount int64
		if err := db.Model(&models.User{}).Where("id = ?", models.PrimaryAdminUserID).Count(&primaryCount).Error; err != nil {
			t.Fatalf("count primary rows: %v", err)
		}
		if primaryCount != 0 {
			t.Fatalf("invalid database was auto-repaired with %d primary rows", primaryCount)
		}
	})
}
