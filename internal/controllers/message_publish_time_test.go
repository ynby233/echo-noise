package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupMessagePublishTimeTest(t *testing.T) (*gorm.DB, *gin.Engine, *uint) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.SiteConfig{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
	})

	var currentUserID uint
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if currentUserID != 0 {
			c.Set("user_id", currentUserID)
			c.Set("auth_via", "session")
		}
		c.Next()
	})
	r.POST("/messages", PostMessage)
	r.PUT("/messages/:id", UpdateMessage)
	r.PUT("/messages/:id/pin", UpdateMessagePinned)
	r.DELETE("/messages/:id", DeleteMessage)
	return db, r, &currentUserID
}

func performMessageJSONRequest(r http.Handler, method string, path string, body map[string]any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assertMessageResponseCode(t *testing.T, w *httptest.ResponseRecorder, expectedStatus int, expectedCode float64) {
	t.Helper()
	if w.Code != expectedStatus {
		t.Fatalf("expected http %d, got %d: %s", expectedStatus, w.Code, w.Body.String())
	}
	var resp struct {
		Code float64 `json:"code"`
		Msg  string  `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	if resp.Code != expectedCode {
		t.Fatalf("expected code %.0f, got %.0f (%s): %s", expectedCode, resp.Code, resp.Msg, w.Body.String())
	}
}

func TestAdminCanSetCreatedAtWhenPostingMessage(t *testing.T) {
	db, r, currentUserID := setupMessagePublishTimeTest(t)
	admin := models.User{ID: models.PrimaryAdminUserID, Username: "admin", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	*currentUserID = admin.ID

	publishedAt := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	w := performMessageJSONRequest(r, http.MethodPost, "/messages", map[string]any{
		"content":    "scheduled admin note",
		"private":    false,
		"created_at": publishedAt.Format(time.RFC3339),
	})
	assertMessageResponseCode(t, w, http.StatusOK, 1)

	var msg models.Message
	if err := db.Where("content = ?", "scheduled admin note").First(&msg).Error; err != nil {
		t.Fatalf("find created message: %v", err)
	}
	if !msg.CreatedAt.Equal(publishedAt) {
		t.Fatalf("expected created_at %s, got %s", publishedAt.Format(time.RFC3339), msg.CreatedAt.Format(time.RFC3339))
	}
}

func TestRegularUserCannotSetCreatedAtWhenPostingMessage(t *testing.T) {
	db, r, currentUserID := setupMessagePublishTimeTest(t)
	user := models.User{ID: 1201, Username: "bob"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	*currentUserID = user.ID

	w := performMessageJSONRequest(r, http.MethodPost, "/messages", map[string]any{
		"content":    "forbidden timestamp",
		"private":    false,
		"created_at": "2024-05-06T07:08:09Z",
	})
	assertMessageResponseCode(t, w, http.StatusOK, 0)

	var count int64
	db.Model(&models.Message{}).Where("content = ?", "forbidden timestamp").Count(&count)
	if count != 0 {
		t.Fatalf("regular user should not be able to create message with custom created_at")
	}
}

func TestAdminCanUpdateOwnMessageCreatedAt(t *testing.T) {
	db, r, currentUserID := setupMessagePublishTimeTest(t)
	admin := models.User{ID: models.PrimaryAdminUserID, Username: "admin", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	originalAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	msg := models.Message{Content: "owned note", UserID: admin.ID, CreatedAt: originalAt}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	*currentUserID = admin.ID

	newAt := time.Date(2024, 6, 7, 8, 9, 10, 0, time.UTC)
	w := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(msg.ID), 10), map[string]any{
		"created_at": newAt.Format(time.RFC3339),
	})
	assertMessageResponseCode(t, w, http.StatusOK, 1)

	var updated models.Message
	if err := db.First(&updated, msg.ID).Error; err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if !updated.CreatedAt.Equal(newAt) {
		t.Fatalf("expected created_at %s, got %s", newAt.Format(time.RFC3339), updated.CreatedAt.Format(time.RFC3339))
	}
}

func TestAdminCannotUpdateOthersMessageCreatedAt(t *testing.T) {
	db, r, currentUserID := setupMessagePublishTimeTest(t)
	owner := models.User{ID: 1401, Username: "owner", IsAdmin: true}
	otherAdmin := models.User{ID: 1402, Username: "other-admin", IsAdmin: true}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := db.Create(&otherAdmin).Error; err != nil {
		t.Fatalf("create other admin: %v", err)
	}
	originalAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	msg := models.Message{Content: "others note", UserID: owner.ID, CreatedAt: originalAt}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	*currentUserID = otherAdmin.ID

	w := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(msg.ID), 10), map[string]any{
		"created_at": "2024-06-07T08:09:10Z",
	})
	assertMessageResponseCode(t, w, http.StatusForbidden, 0)

	var unchanged models.Message
	if err := db.First(&unchanged, msg.ID).Error; err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if !unchanged.CreatedAt.Equal(originalAt) {
		t.Fatalf("created_at changed unexpectedly from %s to %s", originalAt.Format(time.RFC3339), unchanged.CreatedAt.Format(time.RFC3339))
	}
}

func TestRegularUserCannotUpdateOwnMessageCreatedAt(t *testing.T) {
	db, r, currentUserID := setupMessagePublishTimeTest(t)
	user := models.User{ID: 1501, Username: "bob"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	originalAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	msg := models.Message{Content: "own note", UserID: user.ID, CreatedAt: originalAt}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	*currentUserID = user.ID

	w := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(msg.ID), 10), map[string]any{
		"created_at": "2024-06-07T08:09:10Z",
	})
	assertMessageResponseCode(t, w, http.StatusForbidden, 0)

	var unchanged models.Message
	if err := db.First(&unchanged, msg.ID).Error; err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if !unchanged.CreatedAt.Equal(originalAt) {
		t.Fatalf("created_at changed unexpectedly from %s to %s", originalAt.Format(time.RFC3339), unchanged.CreatedAt.Format(time.RFC3339))
	}
}

func TestDelegatedMessageMutationAuditsSuccessAndPrimaryContentDenial(t *testing.T) {
	db, r, currentUserID := setupMessagePublishTimeTest(t)
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	delegated := models.User{ID: 1601, Username: "delegated", IsAdmin: true}
	owner := models.User{ID: 1602, Username: "owner"}
	for _, user := range []*models.User{&primary, &delegated, &owner} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user %s: %v", user.Username, err)
		}
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityNotesEdit}); err != nil {
		t.Fatalf("grant delegated editor: %v", err)
	}
	ordinaryMessage := models.Message{Content: "ordinary", UserID: owner.ID}
	primaryMessage := models.Message{Content: "primary", UserID: primary.ID}
	if err := db.Create(&ordinaryMessage).Error; err != nil {
		t.Fatalf("create ordinary message: %v", err)
	}
	if err := db.Create(&primaryMessage).Error; err != nil {
		t.Fatalf("create primary message: %v", err)
	}
	*currentUserID = delegated.ID

	allowed := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(ordinaryMessage.ID), 10), map[string]any{"content": "delegated update"})
	assertMessageResponseCode(t, allowed, http.StatusOK, 1)
	denied := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(primaryMessage.ID), 10), map[string]any{"content": "forbidden"})
	assertMessageResponseCode(t, denied, http.StatusForbidden, 0)

	var records []models.AdminAuditLog
	if err := db.Where("actor_user_id = ? AND capability = ?", delegated.ID, authorization.CapabilityNotesEdit).Order("id ASC").Find(&records).Error; err != nil {
		t.Fatalf("load message mutation audits: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected success and denied message mutation audit records, got %#v", records)
	}
	if records[0].Result != "success" || records[0].TargetID != strconv.FormatUint(uint64(ordinaryMessage.ID), 10) {
		t.Fatalf("unexpected success audit: %#v", records[0])
	}
	if records[1].Result != "denied" || records[1].TargetID != "" || records[1].TargetOwnerUserID != nil || records[1].Reason != string(authorization.DenialProtectedContent) {
		t.Fatalf("unexpected denial audit: %#v", records[1])
	}
}

func TestDelegatedAdministratorCannotRequestPublishNotificationWithoutCapability(t *testing.T) {
	db, r, currentUserID := setupMessagePublishTimeTest(t)
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	delegated := models.User{ID: 1701, Username: "delegated", IsAdmin: true}
	for _, user := range []*models.User{&primary, &delegated} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user %s: %v", user.Username, err)
		}
	}
	if err := db.Create(&models.SiteConfig{NotifyEnabled: true}).Error; err != nil {
		t.Fatalf("enable publish notifications: %v", err)
	}
	*currentUserID = delegated.ID

	w := performMessageJSONRequest(r, http.MethodPost, "/messages", map[string]any{"content": "notification bypass", "notify": true})
	assertMessageResponseCode(t, w, http.StatusForbidden, 0)
	var count int64
	if err := db.Model(&models.Message{}).Where("content = ?", "notification bypass").Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("unauthorized notification request must not publish a message")
	}
}
