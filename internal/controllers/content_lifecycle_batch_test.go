package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/services"
)

func performBatchRequest(r http.Handler, path string, ids []uint) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"ids": ids})
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	return response
}

func TestAdminCommentBatchTrashRestoreAndPermanentDelete(t *testing.T) {
	db, router, primary, message := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.User{}).Where("id = ?", primary.ID).Update("is_admin", true).Error; err != nil {
		t.Fatal(err)
	}
	repository.ClearUserCache()
	other := models.User{Username: "batch-admin-target"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &other.ID, Content: "admin batch target", Visibility: "public"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	router.Use(func(c *gin.Context) {
		c.Set("user_id", primary.ID)
		c.Set("is_admin", true)
		c.Next()
	})
	router.POST("/admin/comments/batch-trash", BatchTrashAdminComments)
	router.POST("/admin/comment-recycle-bin/batch-restore", BatchRestoreAdminComments)
	router.POST("/admin/comment-recycle-bin/batch-permanent-delete", BatchPermanentlyDeleteAdminComments)

	for _, step := range []struct {
		path string
	}{
		{path: "/admin/comments/batch-trash"},
		{path: "/admin/comment-recycle-bin/batch-restore"},
		{path: "/admin/comments/batch-trash"},
		{path: "/admin/comment-recycle-bin/batch-permanent-delete"},
	} {
		response := performBatchRequest(router, step.path, []uint{comment.ID})
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"succeeded":1`)) {
			t.Fatalf("%s status=%d body=%s", step.path, response.Code, response.Body.String())
		}
	}
	var count int64
	if err := db.Model(&models.Comment{}).Where("id = ?", comment.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("permanently deleted comment count=%d err=%v", count, err)
	}
}

func TestPersonalCommentBatchHandlesCascadeAndRestoresParentsFirst(t *testing.T) {
	db, router, user, message := setupCommentAccountTest(t)
	router.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID)
		c.Next()
	})
	router.POST("/user/interactions/batch-trash", BatchTrashPersonalInteractions)
	router.POST("/user/recycle-bin/comments/batch-restore", BatchRestorePersonalComments)

	root := models.Comment{MessageID: message.ID, UserID: &user.ID, Content: "root", Visibility: "public"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	child := models.Comment{MessageID: message.ID, ParentID: &root.ID, UserID: &user.ID, Content: "child", Visibility: "public"}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}

	// Deliberately submit the child first. Trash batches must still process the
	// selected parent first so the descendant records the cascade reason.
	trash := performBatchRequest(router, "/user/interactions/batch-trash", []uint{child.ID, root.ID})
	if trash.Code != http.StatusOK || !bytes.Contains(trash.Body.Bytes(), []byte(`"succeeded":2`)) {
		t.Fatalf("batch trash status=%d body=%s", trash.Code, trash.Body.String())
	}
	var trashed []models.Comment
	if err := db.Where("id IN ?", []uint{root.ID, child.ID}).Order("id").Find(&trashed).Error; err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 2 || trashed[0].DeletedAt == nil || trashed[1].DeletedAt == nil || trashed[1].ParentID == nil || *trashed[1].ParentID != root.ID || trashed[1].DeletionReasonCode != "ancestor" {
		t.Fatalf("batch trash broke the parent chain: %#v", trashed)
	}

	// Deliberately send the child first. The batch handler must defer it until
	// its selected parent has been restored instead of producing a partial fail.
	restore := performBatchRequest(router, "/user/recycle-bin/comments/batch-restore", []uint{child.ID, root.ID})
	if restore.Code != http.StatusOK || !bytes.Contains(restore.Body.Bytes(), []byte(`"succeeded":2`)) || !bytes.Contains(restore.Body.Bytes(), []byte(`"failed":0`)) {
		t.Fatalf("batch restore status=%d body=%s", restore.Code, restore.Body.String())
	}
	var activeCount int64
	if err := db.Model(&models.Comment{}).Where("id IN ? AND deleted_at IS NULL", []uint{root.ID, child.ID}).Count(&activeCount).Error; err != nil || activeCount != 2 {
		t.Fatalf("restored active comments=%d err=%v", activeCount, err)
	}
}

func TestPersonalNoteBatchTrashRestoreAndPermanentDeleteUsesAuthorScope(t *testing.T) {
	db, router, user, first := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatal(err)
	}
	second := models.Message{Content: "second", UserID: user.ID, Username: user.Username}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	third := models.Message{Content: "third", UserID: user.ID, Username: user.Username}
	if err := db.Create(&third).Error; err != nil {
		t.Fatal(err)
	}
	interaction := models.Comment{MessageID: first.ID, UserID: &user.ID, Content: "must follow note into recycle bin", Visibility: "public"}
	if err := db.Create(&interaction).Error; err != nil {
		t.Fatal(err)
	}
	router.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID)
		c.Next()
	})
	router.POST("/user/notes/batch-trash", BatchTrashPersonalNotes)
	router.POST("/user/recycle-bin/notes/batch-restore", BatchRestorePersonalNotes)
	router.POST("/user/recycle-bin/notes/batch-permanent-delete", BatchPermanentlyDeletePersonalNotes)
	router.DELETE("/user/recycle-bin/notes/:id", PermanentlyDeletePersonalNote)

	trash := performBatchRequest(router, "/user/notes/batch-trash", []uint{first.ID, second.ID})
	if trash.Code != http.StatusOK || !bytes.Contains(trash.Body.Bytes(), []byte(`"succeeded":2`)) {
		t.Fatalf("note batch trash status=%d body=%s", trash.Code, trash.Body.String())
	}
	var storedInteraction models.Comment
	if err := db.First(&storedInteraction, interaction.ID).Error; err != nil || storedInteraction.DeletedAt == nil || storedInteraction.DeletedAncestorMessageID == nil || *storedInteraction.DeletedAncestorMessageID != first.ID {
		t.Fatalf("new note lifecycle left an active or detached interaction: %#v err=%v", storedInteraction, err)
	}
	restore := performBatchRequest(router, "/user/recycle-bin/notes/batch-restore", []uint{first.ID, second.ID})
	if restore.Code != http.StatusOK || !bytes.Contains(restore.Body.Bytes(), []byte(`"succeeded":2`)) {
		t.Fatalf("note batch restore status=%d body=%s", restore.Code, restore.Body.String())
	}
	if response := performBatchRequest(router, "/user/notes/batch-trash", []uint{second.ID}); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"succeeded":1`)) {
		t.Fatalf("note retrash status=%d body=%s", response.Code, response.Body.String())
	}
	var retrashState models.Message
	if err := db.First(&retrashState, second.ID).Error; err != nil {
		t.Fatalf("load re-trashed note: %v", err)
	}
	if retrashState.DeletedAt == nil || retrashState.UserID != user.ID {
		t.Fatalf("unexpected re-trashed note state: %#v", retrashState)
	}
	deleted := performBatchRequest(router, "/user/recycle-bin/notes/batch-permanent-delete", []uint{second.ID})
	if deleted.Code != http.StatusOK || !bytes.Contains(deleted.Body.Bytes(), []byte(`"succeeded":1`)) {
		t.Fatalf("note batch permanent delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	var count int64
	if err := db.Model(&models.Message{}).Where("id = ?", second.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("personal batch permanent delete count=%d err=%v", count, err)
	}
	if err := services.TrashMessage(db, user.ID, third.ID, "author request"); err != nil {
		t.Fatalf("trash single-delete target: %v", err)
	}
	singleRequest := httptest.NewRequest(http.MethodDelete, "/user/recycle-bin/notes/"+strconvFormatUint(third.ID), nil)
	singleResponse := httptest.NewRecorder()
	router.ServeHTTP(singleResponse, singleRequest)
	if singleResponse.Code != http.StatusOK || !bytes.Contains(singleResponse.Body.Bytes(), []byte(`"code":1`)) {
		t.Fatalf("single personal permanent delete status=%d body=%s", singleResponse.Code, singleResponse.Body.String())
	}
	if err := db.Model(&models.Message{}).Where("id = ?", third.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("personal single permanent delete count=%d err=%v", count, err)
	}
}
