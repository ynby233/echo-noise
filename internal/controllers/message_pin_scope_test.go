package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func setupMessagePinScopeTest(t *testing.T) (*gorm.DB, *gin.Engine, *uint) {
	db, r, currentUserID := setupMessagePublishTimeTest(t)
	r.PUT("/messages/:id/pin/global", UpdateMessageGlobalPin)
	r.PUT("/messages/:id/pin/personal", UpdateMessagePersonalPin)
	r.PUT("/token/messages/:id/pin", UpdateMessagePinned)
	return db, r, currentUserID
}

func TestPinEndpointsKeepGlobalAndPersonalStatesIndependent(t *testing.T) {
	db, r, currentUserID := setupMessagePinScopeTest(t)
	owner := models.User{ID: 1801, Username: "owner"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	message := models.Message{Content: "owned note", UserID: owner.ID, Username: owner.Username, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	*currentUserID = owner.ID

	personal := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(message.ID), 10)+"/pin/personal", map[string]any{"pinned": true})
	assertMessageResponseCode(t, personal, http.StatusOK, 1)
	var afterPersonal models.Message
	if err := db.First(&afterPersonal, message.ID).Error; err != nil {
		t.Fatalf("reload after personal pin: %v", err)
	}
	if !afterPersonal.PersonalPinned || afterPersonal.Pinned {
		t.Fatalf("personal pin must only update personal_pinned: %#v", afterPersonal)
	}

	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatalf("create primary admin: %v", err)
	}
	*currentUserID = primary.ID
	global := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(message.ID), 10)+"/pin/global", map[string]any{"pinned": true})
	assertMessageResponseCode(t, global, http.StatusOK, 1)
	if err := db.First(&afterPersonal, message.ID).Error; err != nil {
		t.Fatalf("reload after global pin: %v", err)
	}
	if !afterPersonal.PersonalPinned || !afterPersonal.Pinned {
		t.Fatalf("global pin must preserve the personal pin: %#v", afterPersonal)
	}
}

func TestGlobalAndPersonalPinPermissionsAndAudits(t *testing.T) {
	db, r, currentUserID := setupMessagePinScopeTest(t)
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	delegated := models.User{ID: 1802, Username: "delegated", IsAdmin: true}
	owner := models.User{ID: 1803, Username: "owner"}
	for _, user := range []*models.User{&primary, &delegated, &owner} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user %s: %v", user.Username, err)
		}
	}
	ownerMessage := models.Message{Content: "owner note", UserID: owner.ID, Username: owner.Username, Visibility: "public"}
	primaryMessage := models.Message{Content: "primary note", UserID: primary.ID, Username: primary.Username, Visibility: "public"}
	delegatedMessage := models.Message{Content: "delegated note", UserID: delegated.ID, Username: delegated.Username, Visibility: "public"}
	for _, message := range []*models.Message{&ownerMessage, &primaryMessage, &delegatedMessage} {
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	*currentUserID = owner.ID
	ordinaryGlobal := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(ownerMessage.ID), 10)+"/pin/global", map[string]any{"pinned": true})
	assertMessageResponseCode(t, ordinaryGlobal, http.StatusForbidden, 0)
	ordinaryPersonal := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(ownerMessage.ID), 10)+"/pin/personal", map[string]any{"pinned": true})
	assertMessageResponseCode(t, ordinaryPersonal, http.StatusOK, 1)

	*currentUserID = delegated.ID
	delegatedWithoutGrant := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(ownerMessage.ID), 10)+"/pin/global", map[string]any{"pinned": true})
	assertMessageResponseCode(t, delegatedWithoutGrant, http.StatusForbidden, 0)
	delegatedPersonal := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(delegatedMessage.ID), 10)+"/pin/personal", map[string]any{"pinned": true})
	assertMessageResponseCode(t, delegatedPersonal, http.StatusOK, 1)

	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityNotesPinGlobal}); err != nil {
		t.Fatalf("grant global pin capability: %v", err)
	}
	delegatedAllowed := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(ownerMessage.ID), 10)+"/pin/global", map[string]any{"pinned": true})
	assertMessageResponseCode(t, delegatedAllowed, http.StatusOK, 1)
	delegatedProtected := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(primaryMessage.ID), 10)+"/pin/global", map[string]any{"pinned": true})
	assertMessageResponseCode(t, delegatedProtected, http.StatusForbidden, 0)

	*currentUserID = primary.ID
	primaryAllowed := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(primaryMessage.ID), 10)+"/pin/global", map[string]any{"pinned": true})
	assertMessageResponseCode(t, primaryAllowed, http.StatusOK, 1)
	primaryPersonal := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(primaryMessage.ID), 10)+"/pin/personal", map[string]any{"pinned": true})
	assertMessageResponseCode(t, primaryPersonal, http.StatusOK, 1)

	var globalAudits []models.AdminAuditLog
	if err := db.Where("capability = ? AND action IN ?", authorization.CapabilityNotesPinGlobal, []string{"set_global_pin", "unset_global_pin"}).Order("id ASC").Find(&globalAudits).Error; err != nil {
		t.Fatalf("load global pin audits: %v", err)
	}
	if len(globalAudits) != 5 {
		t.Fatalf("expected denied and successful global pin audits, got %#v", globalAudits)
	}
	for _, audit := range globalAudits {
		if audit.ChangesJSON == "" || !json.Valid([]byte(audit.ChangesJSON)) {
			t.Fatalf("global pin audit must contain valid changes JSON: %#v", audit)
		}
		if strings.Contains(audit.ChangesJSON, "content") || strings.Contains(audit.ChangesJSON, "owner note") {
			t.Fatalf("global pin audit must not contain message content: %#v", audit)
		}
	}

	var personalAudits int64
	if err := db.Model(&models.AdminAuditLog{}).Where("action = ?", "set_personal_pin").Count(&personalAudits).Error; err != nil {
		t.Fatalf("count personal pin audits: %v", err)
	}
	if personalAudits != 0 {
		t.Fatalf("personal pin must not create administrator audit records, got %d", personalAudits)
	}
}

func TestLegacyPinEndpointCannotBypassGlobalAuthorization(t *testing.T) {
	db, r, currentUserID := setupMessagePinScopeTest(t)
	owner := models.User{ID: 1804, Username: "owner"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	message := models.Message{Content: "legacy route note", UserID: owner.ID, Username: owner.Username, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	*currentUserID = owner.ID

	legacy := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(message.ID), 10)+"/pin", map[string]any{"pinned": true})
	assertMessageResponseCode(t, legacy, http.StatusForbidden, 0)
	legacyToken := performMessageJSONRequest(r, http.MethodPut, "/token/messages/"+strconv.FormatUint(uint64(message.ID), 10)+"/pin", map[string]any{"pinned": true})
	assertMessageResponseCode(t, legacyToken, http.StatusForbidden, 0)

	var unchanged models.Message
	if err := db.First(&unchanged, message.ID).Error; err != nil {
		t.Fatalf("reload legacy message: %v", err)
	}
	if unchanged.Pinned || unchanged.PersonalPinned {
		t.Fatalf("legacy unauthorized endpoint changed pin state: %#v", unchanged)
	}
}
