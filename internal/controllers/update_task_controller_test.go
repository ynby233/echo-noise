package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/buildinfo"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/updates"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUpdateWakeRunsAfterCommitAndFailureKeepsOriginalTask(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	file := filepath.Join(t.TempDir(), "wake-token")
	if err := os.WriteFile(file, []byte("dedicated-token"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPDATE_EXECUTOR_WAKE_TOKEN_FILE", file)
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		task, err := updates.NewTaskService(db).LatestTask()
		if err != nil || task == nil || task.Status != updates.TaskPending {
			t.Error("not committed before wake")
		}
		time.Sleep(3200 * time.Millisecond) // Exceeds the finite wake timeout.
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv("UPDATE_EXECUTOR_WAKE_URL", server.URL)
	post := func(role string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/updates/tasks", strings.NewReader(`{"channel":"edge"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Test-Role", role)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if post("delegated").Code != 403 || calls != 0 {
		t.Fatal("rejected creation notified executor")
	}
	first := post("primary")
	second := post("primary")
	if first.Code != 201 || second.Code != 200 || calls != 1 {
		t.Fatalf("codes=%d,%d calls=%d", first.Code, second.Code, calls)
	}
	var a, b struct {
		Data models.UpdateTask `json:"data"`
	}
	if json.Unmarshal(first.Body.Bytes(), &a) != nil || json.Unmarshal(second.Body.Bytes(), &b) != nil || a.Data.PublicID == "" || a.Data.PublicID != b.Data.PublicID {
		t.Fatal("wake failure lost or duplicated task")
	}
}

func TestUpdateCreationRejectsContactWithoutDeploymentCheck(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	if err := db.Model(&models.UpdateExecutorCredential{}).Where("id = ?", 1).Update("checked_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/updates/tasks", strings.NewReader(`{"channel":"edge"}`))
	req.Header.Set("X-Test-Role", "primary")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusPreconditionFailed {
		t.Fatalf("contact alone enabled installation: %d %s", w.Code, w.Body.String())
	}
}

func TestUpdateOverviewRestoresAttentionAndLatestResultWithoutBrowserStorage(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	slot := uint(1)
	task := models.UpdateTask{PublicID: "restore-task", Status: updates.TaskNeedsAttention, ActiveSlot: &slot, TargetRevision: targetRevisionForController, TargetDigest: "sha256:private", Channel: "edge", ErrorSummary: "private host error"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{updates.TaskNeedsAttention, updates.TaskSucceeded} {
		for _, role := range []string{"primary", "delegated"} {
			req := httptest.NewRequest(http.MethodGet, "/updates", nil)
			req.Header.Set("X-Test-Role", role)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			var body struct {
				Data struct {
					Task *models.UpdateTask `json:"task"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data.Task == nil || body.Data.Task.PublicID != task.PublicID || body.Data.Task.Status != status {
				t.Fatalf("server did not restore task: %s (%v)", w.Body.String(), err)
			}
			if role != "primary" && (body.Data.Task.TargetRevision != "" || body.Data.Task.TargetDigest != "" || body.Data.Task.ErrorSummary != "") {
				t.Fatal("task leaked private fields")
			}
		}
		if err := db.Model(&task).Updates(map[string]any{"status": updates.TaskSucceeded, "active_slot": nil, "finished_at": time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestExecutorDeploymentCheckHTTPRejectsWrongInstanceRevisionAndCredential(t *testing.T) {
	_, router, token := setupUpdateControllerTest(t)
	router.POST("/api/updates/executor/check", middleware.UpdateExecutorAuthMiddleware(), RecordExecutorDeploymentCheck)
	s, _ := updateTaskService()
	instance, _ := s.InstanceID()
	call := func(raw, instanceID, revision string) int {
		body, _ := json.Marshal(updates.DeploymentCheck{InstanceID: instanceID, Version: updates.ExecutorVersion, Platform: "linux/amd64", Revision: revision, OK: true})
		req := httptest.NewRequest(http.MethodPost, "/api/updates/executor/check", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+raw)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}
	if got := call("administrator-token", instance, buildinfo.Revision); got != 401 {
		t.Fatalf("wrong token: %d", got)
	}
	if got := call(token, strings.Repeat("f", 32), buildinfo.Revision); got != 409 {
		t.Fatalf("wrong instance: %d", got)
	}
	if got := call(token, instance, strings.Repeat("3", 40)); got != 409 {
		t.Fatalf("wrong installed revision: %d", got)
	}
	if got := call(token, instance, buildinfo.Revision); got != 200 {
		t.Fatalf("valid check: %d", got)
	}
}

func TestUpdateTaskHTTPRejectsChangedConfirmedTargetAndPublicMaintenanceIsMinimal(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	router.GET("/maintenance", GetUpdateMaintenance)
	post := httptest.NewRequest(http.MethodPost, "/updates/tasks", strings.NewReader(`{"channel":"edge","revision":"old","digest":"sha256:old"}`))
	post.Header.Set("X-Test-Role", "primary")
	post.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, post)
	if w.Code != 409 {
		t.Fatalf("changed target accepted: %d", w.Code)
	}
	var count int64
	db.Model(&models.UpdateTask{}).Count(&count)
	if count != 0 {
		t.Fatal("changed confirmation created task")
	}
	slot := uint(1)
	if err := db.Create(&models.UpdateTask{PublicID: "private", Status: updates.TaskNeedsAttention, ActiveSlot: &slot, TargetRevision: targetRevisionForController}).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/maintenance", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"maintenance":true`) || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), targetRevisionForController) {
		t.Fatalf("public maintenance leak: %s", w.Body.String())
	}
}

func setupUpdateControllerTest(t *testing.T) (*gorm.DB, *gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}, &models.UpdatePreference{}, &models.UpdateExecutorCredential{}, &models.UpdateTask{}, &models.UpdateTaskEvent{}); err != nil {
		t.Fatal(err)
	}
	users := []models.User{{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}, {Username: "delegated", IsAdmin: true}, {Username: "ordinary"}}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() { database.DB = nil; models.SetDB(nil) })
	_, rawToken, err := updates.NewTaskService(db).CreateCredential(models.PrimaryAdminUserID, "test executor")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updates.NewTaskService(db).Authenticate(rawToken); err != nil {
		t.Fatal(err)
	}
	oldRevision := buildinfo.Revision
	buildinfo.Revision = strings.Repeat("1", 40)
	t.Cleanup(func() { buildinfo.Revision = oldRevision })
	s := updates.NewTaskService(db)
	credential, err := s.CurrentCredential()
	if err != nil {
		t.Fatal(err)
	}
	instance, _ := s.InstanceID()
	if err := s.RecordDeploymentCheck(credential.ID, updates.DeploymentCheck{InstanceID: instance, Version: updates.ExecutorVersion, Platform: "linux/amd64", Revision: buildinfo.Revision, OK: true}); err != nil {
		t.Fatal(err)
	}
	originalDiscovery := discoverUpdates
	discoverUpdates = func(*gin.Context) updates.Report {
		return updates.Report{Channels: []updates.Channel{{
			Name: "edge", Image: "ghcr.io/ynby233/echo-noise", Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Revision: targetRevisionForController, Version: targetRevisionForController[:12], Status: updates.StatusUpdateAvailable, Installable: true, HasUpdate: true,
		}}}
	}
	t.Cleanup(func() { discoverUpdates = originalDiscovery })

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_via", "session")
		switch c.GetHeader("X-Test-Role") {
		case "primary":
			c.Set("user_id", users[0].ID)
		case "primary-token":
			c.Set("user_id", users[0].ID)
			c.Set("auth_via", "token")
		case "delegated":
			c.Set("user_id", users[1].ID)
		case "ordinary":
			c.Set("user_id", users[2].ID)
		}
		c.Next()
	})
	router.POST("/updates/tasks", CreateUpdateTask)
	router.POST("/updates/prepare", PrepareUpdateInstallation)
	executor := router.Group("/updates/executor", middleware.UpdateExecutorAuthMiddleware())
	executor.GET("/work", GetExecutorWork)
	router.GET("/updates", GetUpdates)
	router.GET("/updates/state", GetUpdateState)
	router.GET("/updates/tasks/:id", GetUpdateTask)
	router.GET("/version/update/stream", UpdateVersionStream)
	return db, router, rawToken
}

func TestExecutorShutdownPreparationAuthenticatesOwnerAndBlocksRestore(t *testing.T) {
	db, _, token := setupUpdateControllerTest(t)
	s := updates.NewTaskService(db)
	target := updates.Target{Channel: "edge", Image: "ghcr.io/ynby233/echo-noise", Digest: "sha256:" + strings.Repeat("a", 64), Revision: strings.Repeat("1", 40)}
	task, _, err := s.Create(1, target)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := s.Authenticate(token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(credential.ID); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/api/updates/executor/tasks/:id/prepare", middleware.UpdateExecutorAuthMiddleware(), PrepareUpdateShutdown)
	call := func(raw, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/updates/executor/tasks/"+task.PublicID+"/prepare", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+raw)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if code := call("administrator-token", "{}"); code != 401 {
		t.Fatalf("admin bypass: %d", code)
	}
	if code := call(token, "{}"); code != 409 {
		t.Fatalf("claimed prepare: %d", code)
	}
	if err := s.RecordEvent(credential.ID, task.PublicID, updates.TaskDownloading, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backup.ReserveUpdate(task.PublicID, backup.DefaultLayout(), true) })
	if code := call(token, "{}"); code != 200 {
		t.Fatalf("prepare: %d", code)
	}
	if _, err := backup.StageRestoreWithResult("missing", backup.DefaultLayout()); err == nil || err.Error() != "更新停机已准备，不能同时恢复备份" {
		t.Fatalf("restore was not blocked: %v", err)
	}
	if code := call(token, `{"cancel":true}`); code != 200 {
		t.Fatalf("cancel: %d", code)
	}
	_, next, err := s.CreateCredential(1, "new")
	if err != nil {
		t.Fatal(err)
	}
	if code := call(next, "{}"); code != 403 {
		t.Fatalf("wrong owner prepare: %d", code)
	}
	if code := call(token, "{}"); code != 200 {
		t.Fatalf("rotated active owner cannot finish its task: %d", code)
	}
	if err := s.RevokeCredential(1); err != nil {
		t.Fatal(err)
	}
	if code := call(next, "{}"); code != 401 {
		t.Fatalf("revoked token prepared: %d", code)
	}
}

func TestUpdateTaskHTTPRequiresPrimaryAndDeduplicatesRepeatedPost(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	request := func(role string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/updates/tasks", bytes.NewBufferString(`{"channel":"edge"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", role)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	for _, role := range []string{"", "ordinary", "delegated", "executor"} {
		if response := request(role); response.Code != http.StatusForbidden {
			t.Fatalf("role=%q status=%d body=%s", role, response.Code, response.Body.String())
		}
	}
	first, second := request("primary"), request("primary")
	if first.Code != http.StatusCreated || second.Code != http.StatusOK {
		t.Fatalf("statuses=%d,%d bodies=%s / %s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	var a, b struct {
		Data models.UpdateTask `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if a.Data.PublicID == "" || a.Data.PublicID != b.Data.PublicID {
		t.Fatalf("task ids differ: %q %q", a.Data.PublicID, b.Data.PublicID)
	}
	var count int64
	if err := db.Model(&models.UpdateTask{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("task count=%d err=%v", count, err)
	}
}

func TestUpdateTaskReadRedactsSensitiveTargetOutsidePrimaryAdministrator(t *testing.T) {
	_, router, _ := setupUpdateControllerTest(t)
	createRequest := httptest.NewRequest(http.MethodPost, "/updates/tasks", bytes.NewBufferString(`{"channel":"edge"}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("X-Test-Role", "primary")
	created := httptest.NewRecorder()
	router.ServeHTTP(created, createRequest)
	var payload struct {
		Data models.UpdateTask `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil || payload.Data.PublicID == "" {
		t.Fatalf("created body=%s err=%v", created.Body.String(), err)
	}
	read := func(role, id string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/updates/tasks/"+id, nil)
		request.Header.Set("X-Test-Role", role)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if response := read("delegated", payload.Data.PublicID); response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte("sha256:")) || bytes.Contains(response.Body.Bytes(), []byte(targetRevisionForController[:12])) {
		t.Fatalf("delegated response leaked target: %d %s", response.Code, response.Body.String())
	}
	if response := read("primary", payload.Data.PublicID); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("sha256:")) || !bytes.Contains(response.Body.Bytes(), []byte(targetRevisionForController)) {
		t.Fatalf("primary response lost target: %d %s", response.Code, response.Body.String())
	}
	if response := read("primary", "1"); response.Code != http.StatusNotFound {
		t.Fatalf("sequential task id was accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestUpdateOverviewRedactsBuildIdentityAndErrorsOutsidePrimary(t *testing.T) {
	_, router, _ := setupUpdateControllerTest(t)
	for _, releaseVersion := range []string{"v2.0.0", targetRevisionForController[:12]} {
		t.Run(releaseVersion, func(t *testing.T) {
			privateError := "Get https://example.test/compare/" + targetRevisionForController + "...target: timeout"
			discoverUpdates = func(*gin.Context) updates.Report {
				return updates.Report{
					Installed: updates.Installed{Revision: targetRevisionForController, Digest: "sha256:private"},
					Source:    updates.Source{Revision: targetRevisionForController, Status: updates.StatusCheckFailed, Error: privateError},
					Release:   updates.SourceRelease{Version: releaseVersion, Revision: targetRevisionForController, Status: updates.StatusCheckFailed, Error: privateError},
					Channels: []updates.Channel{
						{Name: "stable", Version: "v1.2.3", Status: updates.StatusCurrent},
						{Name: "edge", Version: targetRevisionForController[:12], Image: "ghcr.io/ynby233/echo-noise", Digest: "sha256:private", Revision: targetRevisionForController, Status: updates.StatusCheckFailed, Error: privateError},
					},
				}
			}
			for _, role := range []string{"delegated", "ordinary", "primary"} {
				request := httptest.NewRequest(http.MethodGet, "/updates", nil)
				request.Header.Set("X-Test-Role", role)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("role=%s status=%d body=%s", role, response.Code, response.Body.String())
				}
				body := response.Body.String()
				if role == "primary" {
					if !strings.Contains(body, targetRevisionForController) || !strings.Contains(body, "example.test") {
						t.Fatalf("primary lost diagnostic identity: %s", body)
					}
					continue
				}
				for _, private := range []string{targetRevisionForController[:12], "sha256:", "example.test", "ghcr.io"} {
					if strings.Contains(body, private) {
						t.Errorf("%s response leaked %q: %s", role, private, body)
					}
				}
				var payload struct {
					Data struct {
						Report updates.Report `json:"report"`
					} `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				public := payload.Data.Report
				if public.Source.Status != updates.StatusCheckFailed || public.Source.Error == "" || public.Channel("stable").Version != "v1.2.3" {
					t.Fatalf("public status or release version lost: %#v", public)
				}
				if releaseVersion == "v2.0.0" && public.Release.Version != releaseVersion {
					t.Fatalf("public formal version lost: %#v", public.Release)
				}
			}
		})
	}
}

func TestLegacyUpdateStreamIsReadOnlyAndExplicitlyRetired(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	req := httptest.NewRequest(http.MethodGet, "/version/update/stream", nil)
	req.Header.Set("X-Test-Role", "primary")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusGone {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.Model(&models.UpdateTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("legacy GET created %d tasks: %v", count, err)
	}
}

func prepareUpdateHTTP(t *testing.T, router *gin.Engine, role, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/updates/prepare", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Role", role)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func expireControllerDeploymentCheck(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Model(&models.UpdateExecutorCredential{}).Where("id = ?", 1).Update("checked_at", time.Now().UTC().Add(-4*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
}

func decodePreparationHTTP(t *testing.T, response *httptest.ResponseRecorder) (updates.DeploymentPreparation, string) {
	t.Helper()
	var body struct {
		Code int `json:"code"`
		Data struct {
			updates.DeploymentPreparation
			WakeStatus string `json:"wake_status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != 1 {
		t.Fatalf("invalid prepare response: %s err=%v", response.Body.String(), err)
	}
	return body.Data.DeploymentPreparation, body.Data.WakeStatus
}

func TestUpdatePreparationWakeAfterCommitCoalescesAndRecordsOneAudit(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	expireControllerDeploymentCheck(t, db)
	service := updates.NewTaskService(db)
	if err := service.SetChannel(1, "edge"); err != nil {
		t.Fatal(err)
	}
	discoverUpdates = func(*gin.Context) updates.Report {
		t.Error("prepare invoked remote target discovery")
		return updates.Report{}
	}
	file := filepath.Join(t.TempDir(), "prepare-wake-token")
	if err := os.WriteFile(file, []byte("dedicated-prepare-token"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPDATE_EXECUTOR_WAKE_TOKEN_FILE", file)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer dedicated-prepare-token" || r.ContentLength != 0 {
			t.Errorf("wake did not use fixed authenticated empty POST: %s length=%d", r.Method, r.ContentLength)
		}
		preparation, err := service.PreparationStatus(buildinfo.Revision)
		if err != nil || preparation.RequestedAt == nil {
			t.Errorf("request not committed before wake: %#v err=%v", preparation, err)
		}
		var count int64
		if err := db.Model(&models.UpdateTask{}).Count(&count).Error; err != nil || count != 0 {
			t.Errorf("preparation created tasks=%d err=%v", count, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv("UPDATE_EXECUTOR_WAKE_URL", server.URL)
	for _, role := range []string{"delegated", "ordinary", "primary-token", ""} {
		if response := prepareUpdateHTTP(t, router, role, `{}`); response.Code != http.StatusForbidden {
			t.Fatalf("role=%q status=%d", role, response.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("rejected preparation woke executor")
	}
	first := prepareUpdateHTTP(t, router, "primary", `{}`)
	second := prepareUpdateHTTP(t, router, "primary", `{}`)
	if first.Code != http.StatusAccepted || second.Code != http.StatusAccepted || calls.Load() != 1 {
		t.Fatalf("statuses=%d,%d wakes=%d", first.Code, second.Code, calls.Load())
	}
	a, firstWake := decodePreparationHTTP(t, first)
	b, secondWake := decodePreparationHTTP(t, second)
	if a.CredentialID != 1 || a.RequestedAt == nil || b.RequestedAt == nil || !a.RequestedAt.Equal(*b.RequestedAt) || firstWake != "sent" || secondWake != "coalesced" {
		t.Fatalf("first=%#v/%s second=%#v/%s", a, firstWake, b, secondWake)
	}
	var audits int64
	if err := db.Model(&models.AdminAuditLog{}).Where("action = ? AND target_type = ? AND target_id = ? AND summary = ?", "request_deployment_check", "update_executor", "1", "requested deployment check").Count(&audits).Error; err != nil || audits != 1 {
		t.Fatalf("preparation audit count=%d err=%v", audits, err)
	}
	if channel, err := service.GetChannel(); err != nil || channel != "edge" {
		t.Fatalf("preparation changed channel=%q err=%v", channel, err)
	}
}

func TestUpdatePreparationWakeFailureAndMissingConfigurationKeepRequest(t *testing.T) {
	for _, mode := range []string{"unconfigured", "rejected", "timeout", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			db, router, _ := setupUpdateControllerTest(t)
			expireControllerDeploymentCheck(t, db)
			t.Setenv("UPDATE_EXECUTOR_WAKE_URL", "")
			file := filepath.Join(t.TempDir(), "wake-token")
			if err := os.WriteFile(file, []byte("dedicated-token"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("UPDATE_EXECUTOR_WAKE_TOKEN_FILE", file)
			want := "failed"
			switch mode {
			case "unconfigured":
				want = "unconfigured"
			case "invalid":
				t.Setenv("UPDATE_EXECUTOR_WAKE_URL", "not-a-url")
			default:
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if mode == "timeout" {
						<-r.Context().Done()
						return
					}
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				defer server.Close()
				t.Setenv("UPDATE_EXECUTOR_WAKE_URL", server.URL)
			}
			response := prepareUpdateHTTP(t, router, "primary", `{}`)
			if response.Code != http.StatusAccepted {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			preparation, wake := decodePreparationHTTP(t, response)
			if wake != want || preparation.RequestedAt == nil {
				t.Fatalf("wake=%q want=%q preparation=%#v", wake, want, preparation)
			}
			work, err := updates.NewTaskService(db).ExecutorWork(preparation.CredentialID)
			if err != nil || !work.CheckRequested || work.TaskAvailable {
				t.Fatalf("wake failure lost request: %#v err=%v", work, err)
			}
		})
	}
}

func TestUpdatePreparationFreshHardConditionsAndEmptyObjectContract(t *testing.T) {
	for _, mode := range []string{"fresh", "expired", "missing", "active", "invalid-body", "database-failure"} {
		t.Run(mode, func(t *testing.T) {
			db, router, _ := setupUpdateControllerTest(t)
			t.Setenv("UPDATE_EXECUTOR_WAKE_URL", "")
			body, want := `{}`, http.StatusOK
			switch mode {
			case "expired":
				if err := db.Model(&models.UpdateExecutorCredential{}).Where("id = ?", 1).Update("expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
					t.Fatal(err)
				}
				want = http.StatusPreconditionFailed
			case "missing":
				if err := db.Delete(&models.UpdateExecutorCredential{}, 1).Error; err != nil {
					t.Fatal(err)
				}
				want = http.StatusPreconditionFailed
			case "active":
				slot := uint(1)
				if err := db.Create(&models.UpdateTask{PublicID: "attention", Status: updates.TaskNeedsAttention, ActiveSlot: &slot}).Error; err != nil {
					t.Fatal(err)
				}
				want = http.StatusConflict
			case "invalid-body":
				want = http.StatusBadRequest
				for _, body := range []string{``, `null`, `[]`, `{"image":"untrusted"}`, `{"command":"run"}`, `{"path":"/host"}`, `{"url":"https://elsewhere"}`, `{} {}`} {
					if response := prepareUpdateHTTP(t, router, "primary", body); response.Code != want {
						t.Fatalf("body=%s status=%d", body, response.Code)
					}
				}
				return
			case "database-failure":
				if err := db.Callback().Query().Before("gorm:query").Register("test:prepare_database_failure", func(tx *gorm.DB) {
					if tx.Statement.Table == "update_tasks" {
						tx.AddError(errors.New("test database query unavailable"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				want = http.StatusInternalServerError
			}
			response := prepareUpdateHTTP(t, router, "primary", body)
			if response.Code != want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body.String())
			}
			if mode == "fresh" {
				preparation, wake := decodePreparationHTTP(t, response)
				if !preparation.Installation.Available || preparation.RequestedAt != nil || wake != "not_needed" {
					t.Fatalf("fresh check generated request: %#v wake=%s", preparation, wake)
				}
			}
			if mode == "expired" || mode == "missing" {
				var body struct {
					Code int                        `json:"code"`
					Data updates.InstallationStatus `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != 0 || body.Data.Available || body.Data.Reason != map[string]string{"expired": "credential_expired", "missing": "executor_unconfigured"}[mode] {
					t.Fatalf("precondition reason missing: %s err=%v", response.Body.String(), err)
				}
			}
		})
	}
}

func TestUpdatePreparationCompletedFailureReturns429AndRetriesAfterCooldown(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	expireControllerDeploymentCheck(t, db)
	t.Setenv("UPDATE_EXECUTOR_WAKE_URL", "")
	first := prepareUpdateHTTP(t, router, "primary", `{}`)
	p, _ := decodePreparationHTTP(t, first)
	service := updates.NewTaskService(db)
	instance, _ := service.InstanceID()
	if err := service.RecordDeploymentCheck(p.CredentialID, updates.DeploymentCheck{InstanceID: instance, Version: updates.ExecutorVersion, Platform: "linux/amd64", Revision: buildinfo.Revision, OK: false}); err != nil {
		t.Fatal(err)
	}
	response := prepareUpdateHTTP(t, router, "primary", `{}`)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "30" || !strings.Contains(response.Body.String(), "部署检查请求过于频繁，请稍后重试") {
		t.Fatalf("cooldown response=%d %s", response.Code, response.Body.String())
	}
	current, err := service.PreparationStatus(buildinfo.Revision)
	if err != nil || current.RequestedAt == nil || !current.RequestedAt.Equal(*p.RequestedAt) {
		t.Fatalf("cooldown changed request: %#v err=%v", current, err)
	}
	work, err := service.ExecutorWork(p.CredentialID)
	if err != nil || work.CheckRequested {
		t.Fatalf("completed failed request advertised work: %#v err=%v", work, err)
	}
	if err := db.Model(&models.UpdateExecutorCredential{}).Where("id = ?", p.CredentialID).Update("check_requested_at", time.Now().UTC().Add(-31*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	retry := prepareUpdateHTTP(t, router, "primary", `{}`)
	if retry.Code != http.StatusAccepted {
		t.Fatalf("cooldown never released: %d %s", retry.Code, retry.Body.String())
	}
	next, wake := decodePreparationHTTP(t, retry)
	if next.RequestedAt == nil || !next.RequestedAt.After(*p.RequestedAt) || wake != "unconfigured" {
		t.Fatalf("retry did not persist new request: %#v wake=%s", next, wake)
	}
}

func TestUpdatePreparationStateIsOwnerOnlyAndNeverDiscoversTargets(t *testing.T) {
	_, router, _ := setupUpdateControllerTest(t)
	for _, path := range []string{"/updates", "/updates/state"} {
		for _, role := range []string{"primary", "delegated", "ordinary"} {
			if path == "/updates/state" {
				discoverUpdates = func(*gin.Context) updates.Report {
					t.Error("state polling performed remote discovery")
					return updates.Report{}
				}
			}
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("X-Test-Role", role)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			var body struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusOK {
				t.Fatalf("state=%s role=%s status=%d err=%v", path, role, response.Code, err)
			}
			_, present := body.Data["preparation"]
			if present != (role == "primary") {
				t.Fatalf("role=%s preparation presence=%v", role, present)
			}
		}
	}
}

func TestUpdatePreparationPreservesDirectTaskCreationWithOldChecks(t *testing.T) {
	db, router, token := setupUpdateControllerTest(t)
	expireControllerDeploymentCheck(t, db)
	post := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/updates/tasks", strings.NewReader(`{"channel":"edge"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Role", "primary")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if response := post(); response.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale old check created task: %d", response.Code)
	}
	service := updates.NewTaskService(db)
	credential, err := service.Authenticate(token)
	if err != nil {
		t.Fatal(err)
	}
	instance, _ := service.InstanceID()
	if err := service.RecordDeploymentCheck(credential.ID, updates.DeploymentCheck{InstanceID: instance, Version: updates.ExecutorVersion, Platform: "linux/amd64", Revision: buildinfo.Revision, OK: true}); err != nil {
		t.Fatal(err)
	}
	if response := post(); response.Code != http.StatusCreated {
		t.Fatalf("old check no longer sufficient: %d %s", response.Code, response.Body.String())
	}
	var saved models.UpdateExecutorCredential
	if err := db.First(&saved, credential.ID).Error; err != nil || saved.CheckRequestedAt != nil {
		t.Fatalf("direct creation required a new preparation: %#v err=%v", saved, err)
	}
}
