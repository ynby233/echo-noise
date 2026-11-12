package routers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/rcy1314/echo-noise/internal/authorization"
	backupservice "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type backupRouteFixture struct {
	db                      *gorm.DB
	router                  *gin.Engine
	owner, delegate, member models.User
	cookies                 map[uint][]*http.Cookie
}

func newBackupRouteFixture(t *testing.T) *backupRouteFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "managed-backup-route-test-secret-32")
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("DB_PATH", filepath.Join(root, "test.db"))
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(root, "passwords.db"))
	db, err := gorm.Open(sqlite.Open(filepath.Join(root, "test.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatal(err)
	}
	previousDB, previousManager := database.DB, syncmanager.DefaultManager
	database.DB = db
	models.SetDB(db)
	syncmanager.DefaultManager = syncmanager.NewManager()
	repository.ClearUserCache()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = previousDB
		models.SetDB(previousDB)
		syncmanager.DefaultManager = previousManager
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	f := &backupRouteFixture{db: db, cookies: make(map[uint][]*http.Cookie)}
	f.owner = models.User{ID: models.PrimaryAdminUserID, Username: "owner", IsAdmin: true, Token: "backup-owner-token"}
	f.delegate = models.User{Username: "delegate", IsAdmin: true, Token: "backup-delegate-token"}
	f.member = models.User{Username: "member", Token: "backup-member-token"}
	for _, user := range []*models.User{&f.owner, &f.delegate, &f.member} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.router = SetupRouter(nil)
	f.router.GET("/__test/backup-session/:id", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", c.Param("id"))
		session.Set("login_expire_at", time.Now().Add(time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	for _, user := range []models.User{f.owner, f.delegate, f.member} {
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/__test/backup-session/"+strconv.FormatUint(uint64(user.ID), 10), nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("seed session: %d %s", response.Code, response.Body.String())
		}
		f.cookies[user.ID] = response.Result().Cookies()
	}
	return f
}

func (f *backupRouteFixture) request(t *testing.T, user models.User, token bool, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if user.ID != 0 {
		if token {
			request.Header.Set("Authorization", "Bearer "+user.Token)
		} else {
			for _, cookie := range f.cookies[user.ID] {
				request.AddCookie(cookie)
			}
		}
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	return response
}

func requireBackupRouteStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body.String())
	}
}

func TestManagedBackupRoutesSeparateGenerationFromArchiveAccess(t *testing.T) {
	f := newBackupRouteFixture(t)
	grants := []authorization.Capability{
		authorization.CapabilityBackupDataView, authorization.CapabilityBackupDataCreate, authorization.CapabilityBackupDataPlan,
		authorization.CapabilityBackupAttachmentsView, authorization.CapabilityBackupAttachmentsCreate, authorization.CapabilityBackupAttachmentsPlan,
	}
	if err := authorization.New(f.db).ReplaceGrants(f.owner.ID, f.delegate.ID, grants); err != nil {
		t.Fatal(err)
	}
	const base = "/api/admin/backups"
	for _, token := range []bool{false, true} {
		requireBackupRouteStatus(t, f.request(t, f.delegate, token, http.MethodGet, base+"/status", ""), http.StatusOK)
		for _, kind := range []string{"data", "attachments"} {
			// The create grant reaches validation; arbitrary destinations are rejected before generation.
			response := f.request(t, f.delegate, token, http.MethodPost, base+"/"+kind+"/create", `{"target":"local","uploadURL":"https://unexpected.invalid"}`)
			requireBackupRouteStatus(t, response, http.StatusBadRequest)
			if !bytes.Contains(response.Body.Bytes(), []byte("uploadURL")) {
				t.Fatalf("wrong create rejection: %s", response.Body.String())
			}
			requireBackupRouteStatus(t, f.request(t, f.delegate, token, http.MethodGet, base+"/"+kind+"/history", ""), http.StatusForbidden)
			for _, action := range []string{"download", "restore"} {
				method := http.MethodGet
				if action == "restore" {
					method = http.MethodPost
				}
				requireBackupRouteStatus(t, f.request(t, f.delegate, token, method, base+"/"+kind+"/history/missing/"+action, `{"confirm":true,"source":"local"}`), http.StatusForbidden)
			}
			scope := "default"
			if kind == "attachments" {
				scope = "local"
			}
			plan := `{"enabled":false,"time":"04:30","timezone":"UTC","keepLocal":2,"keepCloud":3,"target":"local","scope":"` + scope + `"}`
			requireBackupRouteStatus(t, f.request(t, f.delegate, token, http.MethodPut, base+"/plans/"+kind, plan), http.StatusOK)
		}
		for _, route := range []struct{ method, path string }{
			{http.MethodPost, "/server-config/create"}, {http.MethodGet, "/server-config/history"},
			{http.MethodPut, "/plans/server-config"}, {http.MethodPost, "/inspect"}, {http.MethodPost, "/restore"},
		} {
			requireBackupRouteStatus(t, f.request(t, f.delegate, token, route.method, base+route.path, `{}`), http.StatusForbidden)
		}
	}
	var count int64
	if err := f.db.Model(&models.ManagedBackupRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected requests generated %d records: %v", count, err)
	}
	var plans []models.ManagedBackupPlan
	if err := f.db.Find(&plans).Error; err != nil {
		t.Fatal(err)
	}
	for _, plan := range plans {
		if plan.Kind != "server-config" && (plan.Time != "04:30" || plan.KeepLocal != 2 || plan.KeepCloud != 3) {
			t.Fatalf("plan was not saved: %+v", plan)
		}
	}
	if err := f.db.Where("user_id = ? AND capability = ?", f.delegate.ID, authorization.CapabilityBackupDataCreate).Delete(&models.AdminCapabilityGrant{}).Error; err != nil {
		t.Fatal(err)
	}
	for _, token := range []bool{false, true} {
		requireBackupRouteStatus(t, f.request(t, f.delegate, token, http.MethodPost, base+"/data/create", `{"target":"local"}`), http.StatusForbidden)
	}
}

func TestManagedBackupRoutesOwnerDownloadAndForgedGrants(t *testing.T) {
	f := newBackupRouteFixture(t)
	for _, definition := range authorization.Catalog() {
		if err := f.db.Create(&models.AdminCapabilityGrant{UserID: f.delegate.ID, Capability: string(definition.Capability), GrantedByUserID: f.owner.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"data", "attachments", "server-config"} {
		id := uuid.NewString()
		directory := syncmanager.DefaultManager.Directory(kind)
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		archive := []byte("private archive for " + kind)
		if err := os.WriteFile(filepath.Join(directory, id+".zip"), archive, 0600); err != nil {
			t.Fatal(err)
		}
		if err := f.db.Create(&models.ManagedBackupRecord{ID: id, Kind: kind, Status: "success", Target: "local", Local: true, Size: int64(len(archive))}).Error; err != nil {
			t.Fatal(err)
		}
		path := "/api/admin/backups/" + kind + "/history/" + id
		for _, token := range []bool{false, true} {
			requireBackupRouteStatus(t, f.request(t, f.delegate, token, http.MethodGet, path+"/download?source=local", ""), http.StatusForbidden)
			requireBackupRouteStatus(t, f.request(t, f.delegate, token, http.MethodPost, path+"/restore", `{"confirm":true,"source":"local"}`), http.StatusForbidden)
			response := f.request(t, f.owner, token, http.MethodGet, path+"/download?source=local", "")
			requireBackupRouteStatus(t, response, http.StatusOK)
			if !bytes.Equal(response.Body.Bytes(), archive) || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("wrong private download: headers=%v body=%s", response.Header(), response.Body.String())
			}
			requireBackupRouteStatus(t, f.request(t, f.owner, token, http.MethodPost, path+"/restore", `{"confirm":false,"source":"local"}`), http.StatusBadRequest)
		}
	}
	response := f.request(t, f.delegate, false, http.MethodGet, "/api/admin/backups/status", "")
	requireBackupRouteStatus(t, response, http.StatusOK)
	var status struct {
		Data struct {
			Plans       []models.ManagedBackupPlan `json:"plans"`
			Directories map[string]string          `json:"directories"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Data.Plans) != 2 || len(status.Data.Directories) != 2 {
		t.Fatalf("delegate status exposed owner configuration: %s", response.Body.String())
	}
	for _, plan := range status.Data.Plans {
		if plan.Kind == "server-config" {
			t.Fatal("delegate saw server configuration plan")
		}
	}
}

func TestRetiredBackupRoutesNeverContactArbitraryURLs(t *testing.T) {
	f := newBackupRouteFixture(t)
	var calls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer remote.Close()
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/backup/download"},
		{http.MethodPost, "/api/backup/storage/upload"}, {http.MethodPost, "/api/backup/storage/restore"},
		{http.MethodPost, "/api/backup/storage/presign/upload"}, {http.MethodPost, "/api/backup/storage/presign/download"},
		{http.MethodPost, "/api/backup/storage/sync-now"}, {http.MethodPost, "/api/backup/storage/sync-confirm"},
	} {
		body := `{"url":"` + remote.URL + `","uploadURL":"` + remote.URL + `","downloadURL":"` + remote.URL + `"}`
		for _, token := range []bool{false, true} {
			requireBackupRouteStatus(t, f.request(t, f.owner, token, route.method, route.path, body), http.StatusGone)
			requireBackupRouteStatus(t, f.request(t, f.delegate, token, route.method, route.path, body), http.StatusForbidden)
		}
	}
	for _, token := range []bool{false, true} {
		requireBackupRouteStatus(t, f.request(t, f.owner, token, http.MethodPost, "/api/backup/restore", `{}`), http.StatusBadRequest)
		requireBackupRouteStatus(t, f.request(t, f.delegate, token, http.MethodPost, "/api/backup/restore", `{}`), http.StatusForbidden)
	}
	if calls.Load() != 0 {
		t.Fatalf("disabled routes contacted arbitrary destination %d times", calls.Load())
	}
}

func TestManagedBackupRoutesRequireIdentityAndHaveNoTokenAlias(t *testing.T) {
	f := newBackupRouteFixture(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/backups/status"}, {http.MethodPost, "/api/admin/backups/data/create"},
		{http.MethodPut, "/api/admin/backups/plans/data"}, {http.MethodPost, "/api/admin/backups/inspect"}, {http.MethodPost, "/api/admin/backups/restore"},
	} {
		requireBackupRouteStatus(t, f.request(t, models.User{}, false, route.method, route.path, `{}`), http.StatusUnauthorized)
		requireBackupRouteStatus(t, f.request(t, f.member, false, route.method, route.path, `{}`), http.StatusForbidden)
		requireBackupRouteStatus(t, f.request(t, f.member, true, route.method, route.path, `{}`), http.StatusUnauthorized)
	}
	for _, route := range f.router.Routes() {
		if route.Path == "/api/token/admin/backups/status" || route.Path == "/api/backup/status" || route.Path == "/api/admin/backups/restore-upload" {
			t.Fatalf("unexpected alias registered: %+v", route)
		}
	}
}

func TestDelegatedDataCreateGeneratesManagedArchiveWithoutDownloadPermission(t *testing.T) {
	f := newBackupRouteFixture(t)
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserPassword(f.owner.ID, f.owner.Username, "synthetic-test-password", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := authorization.New(f.db).ReplaceGrants(f.owner.ID, f.delegate.ID, []authorization.Capability{authorization.CapabilityBackupDataView, authorization.CapabilityBackupDataCreate}); err != nil {
		t.Fatal(err)
	}
	for _, token := range []bool{false, true} {
		response := f.request(t, f.delegate, token, http.MethodPost, "/api/admin/backups/data/create", `{"target":"local"}`)
		requireBackupRouteStatus(t, response, http.StatusOK)
		var body struct {
			Code int `json:"code"`
			Data struct {
				Record models.ManagedBackupRecord `json:"record"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		record := body.Data.Record
		if body.Code != 1 || record.Kind != "data" || record.Status != "success" || !record.Local || record.Cloud {
			t.Fatalf("generation failed: %s", response.Body.String())
		}
		inspection, err := backupservice.InspectTypedArchive(filepath.Join(syncmanager.DefaultManager.Directory("data"), record.ID+".zip"))
		if err != nil || inspection.Kind != backupservice.KindData || inspection.Legacy {
			t.Fatalf("generated archive inspection=%+v err=%v", inspection, err)
		}
		path := "/api/admin/backups/data/history/" + record.ID + "/download?source=local"
		requireBackupRouteStatus(t, f.request(t, f.delegate, token, http.MethodGet, path, ""), http.StatusForbidden)
		requireBackupRouteStatus(t, f.request(t, f.owner, token, http.MethodGet, path, ""), http.StatusOK)
	}
}
