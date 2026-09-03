package routers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStatusVoceChatFieldVisibilityHTTPMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "status-visibility-test-secret-32")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
	})

	createUser := func(user models.User) *models.User {
		t.Helper()
		if err := repository.CreateUser(&user); err != nil {
			t.Fatalf("create user %s: %v", user.Username, err)
		}
		return &user
	}
	primary := createUser(models.User{Username: "primary", IsAdmin: true, Token: "primary-status-token", VoceChatEmail: "primary@vc.example", VoceChatNotificationEnabled: true})
	delegatedWithUsersView := createUser(models.User{Username: "delegated-view", IsAdmin: true, Token: "delegated-view-status-token", VoceChatEmail: "delegated-view@vc.example", VoceChatNotificationEnabled: false})
	delegatedWithoutUsersView := createUser(models.User{Username: "delegated-no-view", IsAdmin: true, Token: "delegated-no-view-status-token", VoceChatEmail: "delegated-no-view@vc.example", VoceChatNotificationEnabled: true})
	ordinaryA := createUser(models.User{Username: "ordinary-a", Token: "ordinary-a-status-token", VoceChatEmail: "ordinary-a@vc.example", VoceChatNotificationEnabled: false})
	ordinaryB := createUser(models.User{Username: "ordinary-b", Token: "ordinary-b-status-token", VoceChatEmail: "ordinary-b@vc.example", VoceChatNotificationEnabled: true})
	if primary.ID != models.PrimaryAdminUserID {
		t.Fatalf("primary user id = %d, want %d", primary.ID, models.PrimaryAdminUserID)
	}
	notificationValues := map[uint]bool{
		primary.ID:                   primary.VoceChatNotificationEnabled,
		delegatedWithUsersView.ID:    delegatedWithUsersView.VoceChatNotificationEnabled,
		delegatedWithoutUsersView.ID: delegatedWithoutUsersView.VoceChatNotificationEnabled,
		ordinaryA.ID:                 ordinaryA.VoceChatNotificationEnabled,
		ordinaryB.ID:                 ordinaryB.VoceChatNotificationEnabled,
	}

	r := SetupRouter()
	r.GET("/__test/status-session/:id", func(c *gin.Context) {
		id := c.Param("id")
		session := sessions.Default(c)
		session.Set("user_id", id)
		session.Set("login_expire_at", time.Now().Add(time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	seedSession := func(user *models.User) []*http.Cookie {
		t.Helper()
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/__test/status-session/"+strconv.FormatUint(uint64(user.ID), 10), nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("seed session for %d: status=%d body=%s", user.ID, response.Code, response.Body.String())
		}
		return response.Result().Cookies()
	}
	statusBodyAt := func(t *testing.T, path string, cookies []*http.Cookie, token string) map[string]any {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status http=%d body=%s", response.Code, response.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode status JSON: %v", err)
		}
		if body["code"] != float64(1) {
			t.Fatalf("status code=%v body=%s", body["code"], response.Body.String())
		}
		return body
	}
	statusBody := func(t *testing.T, cookies []*http.Cookie, token string) map[string]any {
		return statusBodyAt(t, "/api/status", cookies, token)
	}
	allUsers := []*models.User{primary, delegatedWithUsersView, delegatedWithoutUsersView, ordinaryA, ordinaryB}
	assertFields := func(t *testing.T, body map[string]any, visibleUsers map[uint]bool, visibleEmails map[uint]bool, visibleNotifications map[uint]bool) {
		t.Helper()
		data, ok := body["data"].(map[string]any)
		if !ok {
			t.Fatalf("status data missing: %#v", body)
		}
		for _, key := range []string{"total_messages", "total_users", "total_comments", "total_replies", "total_guestbook"} {
			if _, exists := data[key]; exists {
				t.Errorf("status leaked legacy operational metric %q", key)
			}
		}
		users, ok := data["users"].([]any)
		if data["users"] == nil {
			users, ok = []any{}, true
		}
		if !ok {
			t.Fatalf("status users missing: %#v", data)
		}
		seenUsers := map[uint]bool{}
		for _, raw := range users {
			user, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("status user has unexpected JSON shape: %#v", raw)
			}
			id, ok := user["id"].(float64)
			if !ok {
				t.Fatalf("status user id missing: %#v", user)
			}
			userID := uint(id)
			seenUsers[userID] = true
			if got, want := hasJSONKey(user, "voce_chat_email"), visibleEmails[userID]; got != want {
				t.Errorf("user %d email key present=%v, want %v", userID, got, want)
			}
			if got, want := hasJSONKey(user, "voce_chat_notification_enabled"), visibleNotifications[userID]; got != want {
				t.Errorf("user %d notification key present=%v, want %v", userID, got, want)
			} else if got {
				value, ok := user["voce_chat_notification_enabled"].(bool)
				if !ok || value != notificationValues[userID] {
					t.Errorf("user %d notification value=%v, want %v", userID, user["voce_chat_notification_enabled"], notificationValues[userID])
				}
			}
		}
		for _, user := range allUsers {
			if got, want := seenUsers[user.ID], visibleUsers[user.ID]; got != want {
				t.Errorf("user %d present=%v, want %v", user.ID, got, want)
			}
		}
	}
	allFields := map[uint]bool{}
	allNonPrimaryEmails := map[uint]bool{}
	for _, user := range allUsers {
		allFields[user.ID] = true
		if user.ID != models.PrimaryAdminUserID {
			allNonPrimaryEmails[user.ID] = true
		}
	}
	selfFields := func(user *models.User) map[uint]bool { return map[uint]bool{user.ID: true} }

	assertFields(t, statusBody(t, nil, ""), map[uint]bool{}, map[uint]bool{}, map[uint]bool{})
	assertFields(t, statusBody(t, seedSession(ordinaryA), ""), selfFields(ordinaryA), selfFields(ordinaryA), selfFields(ordinaryA))
	assertFields(t, statusBody(t, nil, ordinaryA.Token), selfFields(ordinaryA), selfFields(ordinaryA), selfFields(ordinaryA))
	assertFields(t, statusBody(t, seedSession(delegatedWithoutUsersView), ""), selfFields(delegatedWithoutUsersView), selfFields(delegatedWithoutUsersView), selfFields(delegatedWithoutUsersView))
	assertFields(t, statusBody(t, nil, delegatedWithoutUsersView.Token), selfFields(delegatedWithoutUsersView), selfFields(delegatedWithoutUsersView), selfFields(delegatedWithoutUsersView))
	assertFields(t, statusBodyAt(t, "/api?include_voce_chat_email=true&user_id=1&export=users", nil, delegatedWithoutUsersView.Token), selfFields(delegatedWithoutUsersView), selfFields(delegatedWithoutUsersView), selfFields(delegatedWithoutUsersView))
	assertFields(t, statusBody(t, nil, "expired-or-invalid-token"), map[uint]bool{}, map[uint]bool{}, map[uint]bool{})
	assertFields(t, statusBody(t, seedSession(ordinaryA), primary.Token), selfFields(ordinaryA), selfFields(ordinaryA), selfFields(ordinaryA))

	if err := db.Create(&models.AdminCapabilityGrant{UserID: delegatedWithUsersView.ID, Capability: string(authorization.CapabilityUsersView), GrantedByUserID: primary.ID}).Error; err != nil {
		t.Fatalf("grant users.view: %v", err)
	}
	assertFields(t, statusBody(t, seedSession(delegatedWithUsersView), ""), allFields, allNonPrimaryEmails, selfFields(delegatedWithUsersView))
	assertFields(t, statusBody(t, nil, delegatedWithUsersView.Token), allFields, allNonPrimaryEmails, selfFields(delegatedWithUsersView))

	if err := db.Where("user_id = ? AND capability = ?", delegatedWithUsersView.ID, authorization.CapabilityUsersView).Delete(&models.AdminCapabilityGrant{}).Error; err != nil {
		t.Fatalf("revoke users.view: %v", err)
	}
	assertFields(t, statusBody(t, seedSession(delegatedWithUsersView), ""), selfFields(delegatedWithUsersView), selfFields(delegatedWithUsersView), selfFields(delegatedWithUsersView))
	assertFields(t, statusBody(t, nil, delegatedWithUsersView.Token), selfFields(delegatedWithUsersView), selfFields(delegatedWithUsersView), selfFields(delegatedWithUsersView))
	assertFields(t, statusBody(t, seedSession(primary), ""), allFields, allNonPrimaryEmails, allFields)
	assertFields(t, statusBody(t, nil, primary.Token), allFields, allNonPrimaryEmails, allFields)
}

func TestDelegatedAdminCannotReplacePrimaryAdminThroughRoleEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "primary-admin-invariant-test-secret-32")
	t.Chdir(t.TempDir())

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
	})

	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true, Token: "primary-role-token"}
	delegated := models.User{ID: 2, Username: "delegated", IsAdmin: true, Token: "delegated-role-token"}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatalf("create primary administrator: %v", err)
	}
	if err := db.Create(&delegated).Error; err != nil {
		t.Fatalf("create delegated administrator: %v", err)
	}
	if err := db.Create(&models.AdminCapabilityGrant{
		UserID: delegated.ID, Capability: string(authorization.CapabilityAdminRolesManage), GrantedByUserID: primary.ID,
	}).Error; err != nil {
		t.Fatalf("grant admin role capability: %v", err)
	}

	request := httptest.NewRequest(http.MethodPut, "/api/user/admin?id=1", nil)
	request.Header.Set("Authorization", "Bearer "+delegated.Token)
	response := httptest.NewRecorder()
	SetupRouter().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("role endpoint http=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode role endpoint response: %v", err)
	}
	if body["code"] != float64(0) {
		t.Fatalf("delegated role mutation code=%v body=%s", body["code"], response.Body.String())
	}

	var primaryAfter models.User
	if err := db.First(&primaryAfter, models.PrimaryAdminUserID).Error; err != nil {
		t.Fatalf("reload primary administrator: %v", err)
	}
	if !primaryAfter.IsAdmin {
		t.Fatal("delegated administrator changed the ID 1 site owner role")
	}
}

func hasJSONKey(value map[string]any, key string) bool {
	_, ok := value[key]
	return ok
}

func TestUserPasswordResetRouteOnlyResetsOrdinaryUsersAndRechecksGrants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "ordinary-user-password-reset-test-secret-32")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", t.TempDir()+"/plain-passwords.db")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
	})

	createUser := func(user models.User) *models.User {
		t.Helper()
		if err := repository.CreateUser(&user); err != nil {
			t.Fatalf("create %s: %v", user.Username, err)
		}
		return &user
	}
	primary := createUser(models.User{Username: "primary", IsAdmin: true, Token: "primary-password-reset-token"})
	delegated := createUser(models.User{Username: "delegated", IsAdmin: true, Token: "delegated-password-reset-token", VoceChatEmail: "delegated@vc.test", VoceChatUserID: "12"})
	ordinary := createUser(models.User{Username: "ordinary", Token: "ordinary-password-reset-token", VoceChatEmail: "ordinary@vc.test", VoceChatUserID: "13"})
	unboundDelegated := createUser(models.User{Username: "unbound-delegated", Password: models.HashPassword("old-unbound-delegated"), IsAdmin: true, Token: "unbound-delegated-token", VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	unboundOrdinary := createUser(models.User{Username: "unbound-ordinary", Password: models.HashPassword("old-unbound-ordinary"), Token: "unbound-ordinary-token", VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	passwordStore := vocechat.DefaultPlainPasswordStore()
	for _, user := range []*models.User{delegated, ordinary} {
		if err := passwordStore.UpsertUserVoceChatPassword(user.ID, user.Username, "existing-test-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
			t.Fatalf("seed recoverable password state for %s: %v", user.Username, err)
		}
	}
	if primary.ID != models.PrimaryAdminUserID {
		t.Fatalf("primary user id = %d, want %d", primary.ID, models.PrimaryAdminUserID)
	}
	vcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || (r.URL.Path != "/api/admin/user/12" && r.URL.Path != "/api/admin/user/13") || r.Header.Get("X-API-Key") != "password-reset-vc-token" {
			t.Fatalf("unexpected VoceChat password update: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Password *string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == nil || *body.Password == "" {
			t.Fatalf("invalid VoceChat password update body: %#v err=%v", body, err)
		}
		_, _ = w.Write([]byte(`{"uid":12,"email":"delegated@vc.test","name":"managed"}`))
	}))
	defer vcServer.Close()
	if err := db.Create(&models.SiteConfig{VoceChatEnabled: true, VoceChatBaseURL: vcServer.URL, VoceChatAdminToken: "password-reset-vc-token"}).Error; err != nil {
		t.Fatalf("create VoceChat config: %v", err)
	}

	r := SetupRouter()
	r.GET("/__test/password-reset-session/:id", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", c.Param("id"))
		session.Set("login_expire_at", time.Now().Add(time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	seedSession := func(user *models.User) []*http.Cookie {
		t.Helper()
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/__test/password-reset-session/"+strconv.FormatUint(uint64(user.ID), 10), nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("seed session for %d: status=%d body=%s", user.ID, response.Code, response.Body.String())
		}
		return response.Result().Cookies()
	}
	reset := func(t *testing.T, cookies []*http.Cookie, token string, targetID uint, password string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/user/reset_password", bytes.NewBufferString(`{"id":`+strconv.FormatUint(uint64(targetID), 10)+`,"password":"`+password+`"}`))
		request.Header.Set("Content-Type", "application/json")
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		return response
	}
	assertStatus := func(t *testing.T, name string, response *httptest.ResponseRecorder, want int) {
		t.Helper()
		if response.Code != want {
			t.Fatalf("%s status=%d, want %d body=%s", name, response.Code, want, response.Body.String())
		}
		if want == http.StatusOK {
			var body struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != 1 {
				t.Fatalf("%s must succeed, code=%d err=%v body=%s", name, body.Code, err, response.Body.String())
			}
		}
	}

	delegatedSession := seedSession(delegated)
	primarySession := seedSession(primary)
	assertStatus(t, "delegated session without grant", reset(t, delegatedSession, "", ordinary.ID, "delegated-no-grant"), http.StatusForbidden)
	assertStatus(t, "delegated bearer without grant", reset(t, nil, delegated.Token, ordinary.ID, "delegated-token-no-grant"), http.StatusForbidden)

	if err := db.Create(&[]models.AdminCapabilityGrant{
		{UserID: delegated.ID, Capability: string(authorization.CapabilityUsersView), GrantedByUserID: primary.ID},
		{UserID: delegated.ID, Capability: string(authorization.CapabilityUsersResetPassword), GrantedByUserID: primary.ID},
	}).Error; err != nil {
		t.Fatalf("grant users.reset_password: %v", err)
	}
	assertStatus(t, "delegated session resets ordinary user after grant", reset(t, delegatedSession, "", ordinary.ID, "delegated-ordinary-reset"), http.StatusOK)
	assertStatus(t, "delegated session resets unbound ordinary user after grant", reset(t, delegatedSession, "", unboundOrdinary.ID, "delegated-unbound-reset"), http.StatusOK)
	assertStatus(t, "delegated bearer cannot reset delegated administrator", reset(t, nil, delegated.Token, delegated.ID, "delegated-admin-reset"), http.StatusForbidden)
	assertStatus(t, "delegated bearer cannot reset unbound delegated administrator", reset(t, nil, delegated.Token, unboundDelegated.ID, "delegated-unbound-admin-reset"), http.StatusForbidden)
	assertStatus(t, "primary bearer resets delegated administrator", reset(t, nil, primary.Token, delegated.ID, "primary-bearer-admin-reset"), http.StatusOK)
	assertStatus(t, "primary session resets delegated administrator", reset(t, primarySession, "", delegated.ID, "primary-session-admin-reset"), http.StatusOK)
	assertStatus(t, "primary bearer resets unbound delegated administrator", reset(t, nil, primary.Token, unboundDelegated.ID, "primary-unbound-admin-reset"), http.StatusOK)
	assertStatus(t, "primary session resets unbound ordinary user", reset(t, primarySession, "", unboundOrdinary.ID, "primary-unbound-ordinary-reset"), http.StatusOK)
	assertStatus(t, "primary session cannot reset ID 1", reset(t, primarySession, "", primary.ID, "primary-self-reset"), http.StatusForbidden)
	for _, expected := range []struct {
		userID   uint
		password string
	}{
		{userID: unboundDelegated.ID, password: "primary-unbound-admin-reset"},
		{userID: unboundOrdinary.ID, password: "primary-unbound-ordinary-reset"},
	} {
		var updated models.User
		if err := db.First(&updated, expected.userID).Error; err != nil {
			t.Fatalf("reload reset target %d: %v", expected.userID, err)
		}
		if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte(expected.password)) != nil {
			t.Fatalf("target %d did not receive the new local password", expected.userID)
		}
	}

	if err := db.Where("user_id = ? AND capability = ?", delegated.ID, authorization.CapabilityUsersResetPassword).Delete(&models.AdminCapabilityGrant{}).Error; err != nil {
		t.Fatalf("revoke users.reset_password: %v", err)
	}
	assertStatus(t, "delegated session after revoke", reset(t, delegatedSession, "", ordinary.ID, "delegated-revoked-session"), http.StatusForbidden)
	assertStatus(t, "delegated bearer after revoke", reset(t, nil, delegated.Token, ordinary.ID, "delegated-revoked-bearer"), http.StatusForbidden)
}

func TestProtectedAdminRouteMatrixRejectsDelegatedAdministratorWithoutRequiredGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "capability-route-matrix-test-secret-32")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.Setting{}, &models.SiteConfig{}, &models.SecurityConfig{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{},
	); err != nil {
		t.Fatalf("migrate route matrix database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	middleware.InvalidateAccessLogConfigCache()
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
		middleware.InvalidateAccessLogConfigCache()
	})

	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true, Token: "primary-route-token"}
	delegated := models.User{Username: "delegated", IsAdmin: true, Token: "delegated-route-token"}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatalf("create primary administrator: %v", err)
	}
	if err := db.Create(&delegated).Error; err != nil {
		t.Fatalf("create delegated administrator: %v", err)
	}

	r := SetupRouter()
	r.GET("/__test/route-matrix-session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", delegated.ID)
		session.Set("username", delegated.Username)
		session.Set("is_admin", delegated.IsAdmin)
		session.Set("login_expire_at", time.Now().Add(time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	seed := httptest.NewRecorder()
	r.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/__test/route-matrix-session", nil))
	if seed.Code != http.StatusNoContent {
		t.Fatalf("seed delegated session status=%d body=%s", seed.Code, seed.Body.String())
	}
	cookies := seed.Result().Cookies()

	type routeCase struct {
		name       string
		method     string
		path       string
		capability authorization.Capability
		token      bool
	}
	cases := []routeCase{
		{name: "authorization catalog", method: http.MethodGet, path: "/api/admin/authorization/catalog", capability: authorization.CapabilityAuthorizationManage},
		{name: "audit logs", method: http.MethodGet, path: "/api/admin/audit-logs", capability: authorization.CapabilityAuditView},
		{name: "attachment list", method: http.MethodGet, path: "/api/attachments/images", capability: authorization.CapabilityAttachmentsView},
		{name: "attachment zip bearer", method: http.MethodPost, path: "/api/attachments/download-zip", capability: authorization.CapabilityAttachmentsDownload, token: true},
		{name: "attachment reference delete", method: http.MethodDelete, path: "/api/attachments/references/not-real", capability: authorization.CapabilityAttachmentsDeleteReference},
		{name: "attachment blob purge bearer", method: http.MethodPost, path: "/api/attachments/references/batch-purge", capability: authorization.CapabilityAttachmentsPurgeBlob, token: true},
		{name: "version update", method: http.MethodPost, path: "/api/version/update", capability: authorization.CapabilityVersionUpdate},
		{name: "session settings", method: http.MethodPut, path: "/api/settings", capability: authorization.CapabilitySiteSettingsManage},
		{name: "token settings", method: http.MethodPut, path: "/api/token/settings", capability: authorization.CapabilitySiteSettingsManage, token: true},
		{name: "notification config", method: http.MethodGet, path: "/api/notify/config", capability: authorization.CapabilityNotificationsView},
		{name: "email test", method: http.MethodPost, path: "/api/email/test", capability: authorization.CapabilityEmailManage},
		{name: "announcement list", method: http.MethodGet, path: "/api/admin/announcements", capability: authorization.CapabilityAnnouncementsView},
		{name: "security attacks", method: http.MethodGet, path: "/api/security/attacks", capability: authorization.CapabilitySecurityView},
		{name: "backup download", method: http.MethodGet, path: "/api/backup/download", capability: authorization.CapabilityDatabaseBackup},
		{name: "user password reset", method: http.MethodPost, path: "/api/user/reset_password", capability: authorization.CapabilityUsersResetPassword},
		{name: "registration applications", method: http.MethodGet, path: "/api/registration/applications", capability: authorization.CapabilityRegistrationView},
		{name: "comment batch trash", method: http.MethodPost, path: "/api/admin/comments/batch-trash", capability: authorization.CapabilityCommentsTrash},
		{name: "comment batch restore", method: http.MethodPost, path: "/api/admin/comment-recycle-bin/batch-restore", capability: authorization.CapabilityCommentsRestore},
		{name: "comment batch permanent delete", method: http.MethodPost, path: "/api/admin/comment-recycle-bin/batch-permanent-delete", capability: authorization.CapabilityCommentsDeletePermanently},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, ok := authorization.DefinitionFor(testCase.capability); !ok {
				t.Fatalf("route matrix references unknown capability %q", testCase.capability)
			}
			request := httptest.NewRequest(testCase.method, testCase.path, bytes.NewBufferString(`{}`))
			request.Header.Set("Content-Type", "application/json")
			if testCase.token {
				request.Header.Set("Authorization", "Bearer "+delegated.Token)
			} else {
				for _, cookie := range cookies {
					request.AddCookie(cookie)
				}
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("%s without %s status=%d body=%s", testCase.path, testCase.capability, response.Code, response.Body.String())
			}
		})
	}

	if err := db.Create(&models.Setting{AllowRegistration: true}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	if err := db.Create(&models.SiteConfig{RSSEnabled: false, RSSTitle: "unchanged"}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}
	if err := db.Create(&[]models.AdminCapabilityGrant{
		{UserID: delegated.ID, Capability: string(authorization.CapabilitySiteSettingsView), GrantedByUserID: primary.ID},
		{UserID: delegated.ID, Capability: string(authorization.CapabilitySiteSettingsManage), GrantedByUserID: primary.ID},
	}).Error; err != nil {
		t.Fatalf("grant delegated site settings management: %v", err)
	}

	for _, testCase := range []struct {
		name  string
		path  string
		token bool
	}{
		{name: "session", path: "/api/settings"},
		{name: "token", path: "/api/token/settings", token: true},
	} {
		t.Run("delegated administrator cannot save RSS fields via "+testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, testCase.path, bytes.NewBufferString(`{"frontendSettings":{"rssEnabled":true,"rssTitle":"must not save"}}`))
			request.Header.Set("Content-Type", "application/json")
			if testCase.token {
				request.Header.Set("Authorization", "Bearer "+delegated.Token)
			} else {
				for _, cookie := range cookies {
					request.AddCookie(cookie)
				}
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("仅站长可管理 RSS")) {
				t.Fatalf("delegated RSS write status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	var config models.SiteConfig
	if err := db.First(&config).Error; err != nil {
		t.Fatalf("load site config: %v", err)
	}
	if config.RSSEnabled || config.RSSTitle != "unchanged" {
		t.Fatalf("delegated RSS write changed config: %#v", config)
	}

	primaryRequest := httptest.NewRequest(http.MethodPut, "/api/token/settings", bytes.NewBufferString(`{"frontendSettings":{"rssEnabled":true,"rssTitle":"primary saved"}}`))
	primaryRequest.Header.Set("Content-Type", "application/json")
	primaryRequest.Header.Set("Authorization", "Bearer "+primary.Token)
	primaryResponse := httptest.NewRecorder()
	r.ServeHTTP(primaryResponse, primaryRequest)
	if primaryResponse.Code != http.StatusOK || !bytes.Contains(primaryResponse.Body.Bytes(), []byte(`"code":1`)) {
		t.Fatalf("primary RSS write status=%d body=%s", primaryResponse.Code, primaryResponse.Body.String())
	}
	if err := db.First(&config).Error; err != nil {
		t.Fatalf("reload site config after primary save: %v", err)
	}
	if !config.RSSEnabled || config.RSSTitle != "primary saved" {
		t.Fatalf("primary RSS write did not persist: %#v", config)
	}

	retiredRefreshRequest := httptest.NewRequest(http.MethodPost, "/api/rss/refresh", nil)
	retiredRefreshRequest.Header.Set("Authorization", "Bearer "+primary.Token)
	retiredRefreshResponse := httptest.NewRecorder()
	r.ServeHTTP(retiredRefreshResponse, retiredRefreshRequest)
	if retiredRefreshResponse.Code != http.StatusNotFound {
		t.Fatalf("retired RSS refresh route status=%d body=%s", retiredRefreshResponse.Code, retiredRefreshResponse.Body.String())
	}
}
