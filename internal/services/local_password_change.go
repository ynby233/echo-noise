package services

import (
	"errors"
	"strings"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

type localPasswordMaterialStore interface {
	GetUserPassword(uint) (vocechat.PlainPasswordRecord, bool, error)
	UpsertUserLocalFallbackPassword(uint, string, string, string, string) error
	RestoreUserPasswordSnapshot(vocechat.PlainPasswordRecord, bool) error
}

type localPasswordChangeDependencies struct {
	store  localPasswordMaterialStore
	verify func(uint, managedPasswordSnapshot, models.User, string, localPasswordMaterialStore) error
}

func defaultLocalPasswordChangeDependencies() localPasswordChangeDependencies {
	return localPasswordChangeDependencies{
		store:  vocechat.DefaultPlainPasswordStore(),
		verify: localPasswordStateMatches,
	}
}

// changeLocalPasswordAtomically is the local-password deep module. Callers
// choose only that the operation is local; snapshotting, status selection,
// verification, and compensation remain hidden behind this interface.
func changeLocalPasswordAtomically(user *models.User, plain string) error {
	return changeLocalPasswordAtomicallyWithDependencies(user, plain, defaultLocalPasswordChangeDependencies())
}

func changeLocalPasswordAtomicallyWithDependencies(user *models.User, plain string, dependencies localPasswordChangeDependencies) error {
	if user == nil || user.ID == 0 || strings.TrimSpace(plain) == "" {
		return &passwordUpdateFailure{cause: errors.New("missing local password state")}
	}
	if dependencies.store == nil || dependencies.verify == nil {
		return &passwordUpdateFailure{cause: errors.New("local password dependencies unavailable")}
	}
	hashed := models.HashPassword(plain)
	if hashed == "" {
		return &passwordUpdateFailure{cause: errors.New("password hashing failed")}
	}
	snapshot, err := readManagedPasswordSnapshotFromStore(user.ID, dependencies.store)
	if err != nil {
		return &passwordUpdateFailure{cause: err}
	}
	expected := snapshot.user
	expected.Password = hashed
	if isVoceChatBoundNonPrimaryUser(&expected) {
		expected.VoceChatSyncStatus = models.VoceChatSyncStatusPasswordSyncRequired
		expected.VoceChatSyncError = ""
	}
	rollback := func(cause error) error {
		if restoreErr := restoreManagedPasswordSnapshotWithStore(user, snapshot, dependencies.store); restoreErr != nil {
			statusErr := markPasswordUpdateIncomplete(user)
			return &passwordUpdateFailure{incomplete: true, cause: errors.Join(cause, restoreErr, statusErr)}
		}
		return &passwordUpdateFailure{rolledBack: true, cause: cause}
	}

	if err := repository.UpdateUserField(user.ID, "password", hashed); err != nil {
		return rollback(err)
	}
	user.Password = hashed
	if err := dependencies.store.UpsertUserLocalFallbackPassword(user.ID, user.Username, plain, user.VoceChatEmail, user.VoceChatUserID); err != nil {
		return rollback(err)
	}
	if isVoceChatBoundNonPrimaryUser(&expected) {
		if err := database.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
			"voce_chat_sync_status": expected.VoceChatSyncStatus,
			"voce_chat_sync_error":  expected.VoceChatSyncError,
		}).Error; err != nil {
			return rollback(err)
		}
		repository.ClearUserCache()
		user.VoceChatSyncStatus = expected.VoceChatSyncStatus
		user.VoceChatSyncError = expected.VoceChatSyncError
	}
	if err := dependencies.verify(user.ID, snapshot, expected, plain, dependencies.store); err != nil {
		return rollback(err)
	}
	return nil
}

func localPasswordStateMatches(userID uint, snapshot managedPasswordSnapshot, expected models.User, plain string, store localPasswordMaterialStore) error {
	var current models.User
	if err := database.DB.First(&current, userID).Error; err != nil {
		return err
	}
	if current.Password != expected.Password ||
		current.Token != expected.Token ||
		!sameOptionalTime(current.LoginIssuedAt, expected.LoginIssuedAt) ||
		current.VoceChatUserID != expected.VoceChatUserID ||
		current.VoceChatEmail != expected.VoceChatEmail ||
		current.VoceChatUsername != expected.VoceChatUsername ||
		current.VoceChatSyncStatus != expected.VoceChatSyncStatus ||
		current.VoceChatSyncError != expected.VoceChatSyncError ||
		!sameOptionalTime(current.VoceChatLastSyncAt, expected.VoceChatLastSyncAt) {
		return errors.New("local password state was not fully persisted")
	}
	record, found, err := store.GetUserPassword(userID)
	if err != nil {
		return err
	}
	if !found || record.LocalFallbackPasswordValue() != plain ||
		record.VoceChatPasswordValue() != snapshot.passwordRecord.VoceChatPasswordValue() ||
		record.Username != expected.Username ||
		record.VoceChatEmail != expected.VoceChatEmail ||
		record.VoceChatUserID != expected.VoceChatUserID {
		return errors.New("local password material was not fully persisted")
	}
	return nil
}
