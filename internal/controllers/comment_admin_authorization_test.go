package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestAdminCommentListUsesHiddenContentReadScope(t *testing.T) {
	db, r, primary, message := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate authorization models: %v", err)
	}
	primary.IsAdmin = true
	if err := db.Save(&primary).Error; err != nil {
		t.Fatalf("promote primary fixture: %v", err)
	}
	delegated := models.User{Username: "list-delegated", IsAdmin: true}
	ordinary := models.User{Username: "list-ordinary"}
	for _, user := range []*models.User{&delegated, &ordinary} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create %s: %v", user.Username, err)
		}
	}
	createTestComment(t, db, message.ID, &ordinary, "ordinary hidden", "private", nil)
	createTestComment(t, db, message.ID, &primary, "primary hidden", "private", nil)

	r.Use(func(c *gin.Context) {
		c.Set("user_id", delegated.ID)
		c.Set("is_admin", true)
		c.Next()
	})
	r.GET("/comments", ListComments)
	requestTotal := func() int64 {
		request := httptest.NewRequest(http.MethodGet, "/comments", nil)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("list comments status=%d body=%s", response.Code, response.Body.String())
		}
		var body struct {
			Code int `json:"code"`
			Data struct {
				Total int64 `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode list comments: %v", err)
		}
		return body.Data.Total
	}

	if got := requestTotal(); got != 0 {
		t.Fatalf("delegated admin without hidden read must not count hidden comments, got %d", got)
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityCommentsView, authorization.CapabilityCommentsViewHidden}); err != nil {
		t.Fatalf("grant comment list and hidden read: %v", err)
	}
	if got := requestTotal(); got != 1 {
		t.Fatalf("hidden read must reveal ordinary hidden comment but not primary hidden comment, got %d", got)
	}
}

func TestDelegatedCommentMutationsUseCapabilityProtectionAndSilentAdminAudit(t *testing.T) {
	db, r, primary, message := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate authorization models: %v", err)
	}
	primary.IsAdmin = true
	if err := db.Save(&primary).Error; err != nil {
		t.Fatalf("promote primary fixture: %v", err)
	}
	delegated := models.User{Username: "delegated", IsAdmin: true}
	owner := models.User{Username: "owner"}
	for _, user := range []*models.User{&delegated, &owner} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user %s: %v", user.Username, err)
		}
	}
	ownerComment := createTestComment(t, db, message.ID, &owner, "owner comment", "public", nil)
	primaryComment := createTestComment(t, db, message.ID, &primary, "primary comment", "public", nil)

	var actorID uint = delegated.ID
	r.Use(func(c *gin.Context) {
		c.Set("user_id", actorID)
		c.Set("auth_via", "session")
		c.Next()
	})
	r.PUT("/messages/:id/comments/:cid", UpdateComment)
	r.DELETE("/messages/:id/comments/:cid", DeleteComment)

	withoutGrant := performCommentJSONRequest(r, http.MethodPut, message.ID, ownerComment.ID, map[string]any{"content": "should stay"})
	if withoutGrant.Code != http.StatusForbidden {
		t.Fatalf("comment edit without grant status=%d body=%s", withoutGrant.Code, withoutGrant.Body.String())
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityCommentsView, authorization.CapabilityCommentsEdit, authorization.CapabilityCommentsTrash}); err != nil {
		t.Fatalf("grant comment capabilities: %v", err)
	}

	allowedEdit := performCommentJSONRequest(r, http.MethodPut, message.ID, ownerComment.ID, map[string]any{"content": "delegated edit"})
	if allowedEdit.Code != http.StatusOK {
		t.Fatalf("comment edit with grant status=%d body=%s", allowedEdit.Code, allowedEdit.Body.String())
	}
	visibilityWithoutGrant := performCommentJSONRequest(r, http.MethodPut, message.ID, ownerComment.ID, map[string]any{"content": "delegated edit", "visibility": "users"})
	if visibilityWithoutGrant.Code != http.StatusForbidden {
		t.Fatalf("comment visibility change without independent grant status=%d body=%s", visibilityWithoutGrant.Code, visibilityWithoutGrant.Body.String())
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{
		authorization.CapabilityCommentsView, authorization.CapabilityCommentsEdit,
		authorization.CapabilityCommentsChangeVisibility, authorization.CapabilityCommentsTrash,
	}); err != nil {
		t.Fatalf("grant independent comment visibility capability: %v", err)
	}
	visibilityWithGrant := performCommentJSONRequest(r, http.MethodPut, message.ID, ownerComment.ID, map[string]any{"content": "delegated edit", "visibility": "users"})
	if visibilityWithGrant.Code != http.StatusOK {
		t.Fatalf("comment visibility change with grant status=%d body=%s", visibilityWithGrant.Code, visibilityWithGrant.Body.String())
	}
	protectedEdit := performCommentJSONRequest(r, http.MethodPut, message.ID, primaryComment.ID, map[string]any{"content": "must stay primary"})
	if protectedEdit.Code != http.StatusForbidden {
		t.Fatalf("primary-owned comment edit status=%d body=%s", protectedEdit.Code, protectedEdit.Body.String())
	}
	protectedDelete := performCommentJSONRequest(r, http.MethodDelete, message.ID, primaryComment.ID, nil)
	if protectedDelete.Code != http.StatusForbidden {
		t.Fatalf("primary-owned comment delete status=%d body=%s", protectedDelete.Code, protectedDelete.Body.String())
	}
	allowedDelete := performCommentJSONRequest(r, http.MethodDelete, message.ID, ownerComment.ID, nil)
	if allowedDelete.Code != http.StatusOK {
		t.Fatalf("comment delete with grant status=%d body=%s", allowedDelete.Code, allowedDelete.Body.String())
	}

	var remaining models.Comment
	if err := db.First(&remaining, primaryComment.ID).Error; err != nil {
		t.Fatalf("primary-owned comment must remain: %v", err)
	}
	if remaining.UserID == nil || *remaining.UserID != primary.ID || remaining.Content != "primary comment" {
		t.Fatalf("primary-owned comment changed unexpectedly: %#v", remaining)
	}
	var notifications int64
	if err := db.Model(&models.UserNotification{}).Count(&notifications).Error; err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if notifications != 0 {
		t.Fatalf("administrator comment mutations must not notify the original author, got %d notifications", notifications)
	}

	var editAudits []models.AdminAuditLog
	if err := db.Where("actor_user_id = ? AND capability = ?", delegated.ID, authorization.CapabilityCommentsEdit).Order("id ASC").Find(&editAudits).Error; err != nil {
		t.Fatalf("load comment edit audits: %v", err)
	}
	if len(editAudits) != 3 || editAudits[0].Result != "denied" || editAudits[1].Result != "success" || editAudits[2].Result != "denied" {
		t.Fatalf("comment edit audits=%#v, want denied/success/denied", editAudits)
	}
	var deleteAudits []models.AdminAuditLog
	if err := db.Where("actor_user_id = ? AND capability = ?", delegated.ID, authorization.CapabilityCommentsTrash).Order("id ASC").Find(&deleteAudits).Error; err != nil {
		t.Fatalf("load comment delete audits: %v", err)
	}
	if len(deleteAudits) != 2 || deleteAudits[0].Result != "denied" || deleteAudits[1].Result != "success" {
		t.Fatalf("comment delete audits=%#v, want denied/success", deleteAudits)
	}
}
