package controllers

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestInvisibleLifecycleDenialsRedactSingleAndBatchAuditTargets(t *testing.T) {
	db, r, currentUserID := setupMessagePublishTimeTest(t)
	r.POST("/admin/notes/:id/trash", TrashAdminNote)
	r.POST("/admin/recycle-bin/:id/restore", RestoreAdminRecycleBinNote)
	r.DELETE("/admin/recycle-bin/:id", PermanentlyDeleteAdminRecycleBinNote)
	r.POST("/admin/notes/batch-trash", BatchTrashAdminNotes)
	r.POST("/admin/recycle-bin/batch-restore", BatchRestoreAdminRecycleBin)
	r.POST("/admin/recycle-bin/batch-permanent-delete", BatchPermanentDeleteAdminRecycleBin)
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	delegated := models.User{ID: 1901, Username: "delegated", IsAdmin: true}
	owner := models.User{ID: 1902, Username: "owner"}
	for _, user := range []*models.User{&primary, &delegated, &owner} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{
		authorization.CapabilityNotesView,
		authorization.CapabilityNotesTrash,
		authorization.CapabilityNotesRecycleBinView,
		authorization.CapabilityNotesRestore,
		authorization.CapabilityNotesDelete,
	}); err != nil {
		t.Fatal(err)
	}
	newMessage := func(content string) models.Message {
		message := models.Message{Content: content, UserID: owner.ID, Username: owner.Username, Visibility: "private", Private: true}
		if err := db.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
		return message
	}
	trashTarget := newMessage("hidden trash target")
	*currentUserID = delegated.ID
	deniedTrash := performMessageJSONRequest(r, http.MethodPost, "/admin/notes/"+strconv.FormatUint(uint64(trashTarget.ID), 10)+"/trash", map[string]any{"reason": "test"})
	assertMessageResponseCode(t, deniedTrash, http.StatusNotFound, 0)

	restoreTarget := newMessage("hidden restore target")
	permanentTarget := newMessage("hidden permanent target")
	*currentUserID = primary.ID
	for _, message := range []models.Message{restoreTarget, permanentTarget} {
		trashed := performMessageJSONRequest(r, http.MethodPost, "/admin/notes/"+strconv.FormatUint(uint64(message.ID), 10)+"/trash", map[string]any{"reason": "test"})
		assertMessageResponseCode(t, trashed, http.StatusOK, 1)
	}
	*currentUserID = delegated.ID
	deniedRestore := performMessageJSONRequest(r, http.MethodPost, "/admin/recycle-bin/"+strconv.FormatUint(uint64(restoreTarget.ID), 10)+"/restore", map[string]any{})
	assertMessageResponseCode(t, deniedRestore, http.StatusNotFound, 0)
	deniedPermanent := performMessageJSONRequest(r, http.MethodDelete, "/admin/recycle-bin/"+strconv.FormatUint(uint64(permanentTarget.ID), 10), nil)
	assertMessageResponseCode(t, deniedPermanent, http.StatusNotFound, 0)

	batchTrashTarget := newMessage("hidden batch trash target")
	batchRestoreTarget := newMessage("hidden batch restore target")
	batchPermanentTarget := newMessage("hidden batch permanent target")
	*currentUserID = primary.ID
	for _, message := range []models.Message{batchRestoreTarget, batchPermanentTarget} {
		trashed := performMessageJSONRequest(r, http.MethodPost, "/admin/notes/"+strconv.FormatUint(uint64(message.ID), 10)+"/trash", map[string]any{"reason": "test"})
		assertMessageResponseCode(t, trashed, http.StatusOK, 1)
	}
	*currentUserID = delegated.ID
	batchDenied := performMessageJSONRequest(r, http.MethodPost, "/admin/notes/batch-trash", map[string]any{"ids": []uint{batchTrashTarget.ID}, "reason": "test"})
	if batchDenied.Code != http.StatusOK {
		t.Fatalf("batch trash status=%d body=%s", batchDenied.Code, batchDenied.Body.String())
	}
	batchRestoreDenied := performMessageJSONRequest(r, http.MethodPost, "/admin/recycle-bin/batch-restore", map[string]any{"ids": []uint{batchRestoreTarget.ID}})
	if batchRestoreDenied.Code != http.StatusOK {
		t.Fatalf("batch restore status=%d body=%s", batchRestoreDenied.Code, batchRestoreDenied.Body.String())
	}
	batchPermanentDenied := performMessageJSONRequest(r, http.MethodPost, "/admin/recycle-bin/batch-permanent-delete", map[string]any{"ids": []uint{batchPermanentTarget.ID}, "reason": "test"})
	if batchPermanentDenied.Code != http.StatusOK {
		t.Fatalf("batch permanent status=%d body=%s", batchPermanentDenied.Code, batchPermanentDenied.Body.String())
	}

	var audits []models.AdminAuditLog
	if err := db.Where("actor_user_id = ? AND result = ?", delegated.ID, "denied").Find(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if len(audits) < 6 {
		t.Fatalf("expected single and batch lifecycle denials, got %d", len(audits))
	}
	for _, audit := range audits {
		if audit.TargetID != "" || audit.TargetOwnerUserID != nil || audit.Summary != "capability request denied" {
			t.Fatalf("lifecycle denial leaked hidden target data: %#v", audit)
		}
	}
}
