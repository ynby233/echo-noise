package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func waitAICredentialBarrier(t *testing.T, barrier <-chan struct{}) {
	t.Helper()
	select {
	case <-barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("AI credential concurrency barrier timed out")
	}
}

func TestAICredentialRevocationBeforeRenewalHasNoLoginSideEffects(t *testing.T) {
	db, owner, _ := setupAICredentialServiceTest(t)
	credential, token, err := CreateAICredential(owner.ID, "revoke first")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour).UTC()
	if err := db.Model(&owner).Update("login_issued_at", old).Error; err != nil {
		t.Fatal(err)
	}
	var before models.User
	if err := db.First(&before, owner.ID).Error; err != nil {
		t.Fatal(err)
	}

	resolved, resume := make(chan struct{}), make(chan struct{})
	var resumeOnce sync.Once
	var blocked atomic.Bool
	var updates atomic.Int32
	if err := db.Callback().Query().After("gorm:query").Register("test:ai_initial_resolution", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && blocked.CompareAndSwap(false, true) {
			// Pause after the first credential resolution, before the account lock.
			close(resolved)
			<-resume
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:ai_login_updates", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			updates.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	var revokeHeldLock atomic.Bool
	if err := db.Callback().Delete().Before("gorm:begin_transaction").Register("test:ai_revoke_lock", func(tx *gorm.DB) {
		if tx.Statement.Table != "ai_credentials" {
			return
		}
		mutex := aiCredentialUserMutex(owner.ID)
		if mutex.TryLock() {
			mutex.Unlock()
		} else {
			revokeHeldLock.Store(true)
		}
	}); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, err := LoginWithAICredential("Bearer "+token, dto.LoginDto{Username: owner.Username, Password: "owner-password"})
		result <- err
	}()
	t.Cleanup(func() {
		resumeOnce.Do(func() { close(resume) })
		waitAICredentialBarrier(t, done)
	})
	waitAICredentialBarrier(t, resolved)
	if err := RevokeAICredential(owner.ID, credential.ID); err != nil {
		t.Fatal(err)
	}
	if !revokeHeldLock.Load() {
		t.Fatal("revocation must hold the same account lock before deleting")
	}
	resumeOnce.Do(func() { close(resume) })
	waitAICredentialBarrier(t, done)
	if err := <-result; err == nil {
		t.Fatal("renewal must reject a credential revoked after its first resolution")
	}
	var after models.User
	if err := db.First(&after, owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updates.Load() != 0 || after.LoginIssuedAt == nil || !after.LoginIssuedAt.Equal(*before.LoginIssuedAt) || after.Password != before.Password || after.Token != before.Token {
		t.Fatal("revocation that completes first must prevent every login mutation")
	}
}

func TestAICredentialRenewalBeforeRevocationFinishesBeforeDelete(t *testing.T) {
	db, owner, _ := setupAICredentialServiceTest(t)
	credential, token, err := CreateAICredential(owner.ID, "renew first")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour).UTC()
	if err := db.Model(&owner).Update("login_issued_at", old).Error; err != nil {
		t.Fatal(err)
	}
	updating, resume := make(chan struct{}), make(chan struct{})
	var resumeOnce sync.Once
	var blocked atomic.Bool
	var updateFinished atomic.Bool
	var deletedAfterUpdate atomic.Bool
	if err := db.Callback().Update().Before("gorm:begin_transaction").Register("test:ai_pause_login_update", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && blocked.CompareAndSwap(false, true) {
			// Pause before opening a write transaction, leaving SQLite's connection
			// available so the synchronization guarantee comes from the account lock.
			close(updating)
			<-resume
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().After("gorm:commit_or_rollback_transaction").Register("test:ai_finished_login_update", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && tx.Error == nil {
			updateFinished.Store(true)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Delete().Before("gorm:begin_transaction").Register("test:ai_delete_after_login", func(tx *gorm.DB) {
		if tx.Statement.Table == "ai_credentials" {
			deletedAfterUpdate.Store(updateFinished.Load())
		}
	}); err != nil {
		t.Fatal(err)
	}
	loginResult, revokeResult := make(chan error, 1), make(chan error, 1)
	loginDone, revokeDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(loginDone)
		_, _, err := LoginWithAICredential("Bearer "+token, dto.LoginDto{Username: owner.Username, Password: "owner-password"})
		loginResult <- err
	}()
	t.Cleanup(func() {
		resumeOnce.Do(func() { close(resume) })
		waitAICredentialBarrier(t, loginDone)
	})
	waitAICredentialBarrier(t, updating)
	mutex := aiCredentialUserMutex(owner.ID)
	if mutex.TryLock() {
		mutex.Unlock()
		t.Fatal("renewal must hold the account lock throughout login mutations")
	}
	revokeStarted := make(chan struct{})
	go func() {
		defer close(revokeDone)
		close(revokeStarted)
		revokeResult <- RevokeAICredential(owner.ID, credential.ID)
	}()
	t.Cleanup(func() {
		resumeOnce.Do(func() { close(resume) })
		waitAICredentialBarrier(t, revokeDone)
	})
	waitAICredentialBarrier(t, revokeStarted)
	resumeOnce.Do(func() { close(resume) })
	waitAICredentialBarrier(t, loginDone)
	waitAICredentialBarrier(t, revokeDone)
	if err := <-loginResult; err != nil {
		t.Fatalf("renewal holding the lock first must succeed: %v", err)
	}
	if err := <-revokeResult; err != nil {
		t.Fatal(err)
	}
	if !deletedAfterUpdate.Load() {
		t.Fatal("revocation must wait for the login mutation to finish")
	}
	var renewed models.User
	if err := db.First(&renewed, owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if renewed.LoginIssuedAt == nil || !renewed.LoginIssuedAt.After(old) {
		t.Fatal("the login that obtained the lock first must renew the account")
	}
	for range 2 {
		if _, _, err := LoginWithAICredential("Bearer "+token, dto.LoginDto{Username: owner.Username, Password: "owner-password"}); err == nil {
			t.Fatal("completed revocation must prevent subsequent credential login")
		}
	}
	var after models.User
	if err := db.First(&after, owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.LoginIssuedAt == nil || !after.LoginIssuedAt.Equal(*renewed.LoginIssuedAt) || after.Password != renewed.Password || after.Token != renewed.Token {
		t.Fatal("revoked login attempts must not cause further account mutations")
	}
}
