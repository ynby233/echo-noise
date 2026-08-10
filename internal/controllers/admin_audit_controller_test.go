package controllers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAdminAuditControllerTest(t *testing.T) (*gorm.DB, models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate audit database: %v", err)
	}
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatalf("create primary administrator: %v", err)
	}
	database.DB = db
	t.Cleanup(func() { database.DB = nil })
	return db, primary
}

func TestListAdminAuditLogsIncludesChinesePresentationWithoutDroppingTechnicalFields(t *testing.T) {
	db, primary := setupAdminAuditControllerTest(t)
	if err := db.Create(&models.AdminAuditLog{
		ActorUserID: primary.ID, ActorUsername: primary.Username, ActorIsPrimary: true,
		Capability: "notes.pin_global", Module: "notes", Action: "set_global_pin",
		TargetType: "message", TargetID: "59", Result: "denied", Reason: "missing_grant",
		Summary: "capability request denied",
	}).Error; err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.GET("/audit", ListAdminAuditLogs)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/audit?module=notes&result=denied&target_id=59", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				Module               string `json:"module"`
				Action               string `json:"action"`
				Summary              string `json:"summary"`
				OperationDescription string `json:"operation_description"`
				ResultDescription    string `json:"result_description"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 1 || len(body.Data.Items) != 1 {
		t.Fatalf("unexpected list response: %#v", body)
	}
	item := body.Data.Items[0]
	if item.Module != "notes" || item.Action != "set_global_pin" || item.Summary != "capability request denied" {
		t.Fatalf("technical fields were not preserved: %#v", item)
	}
	if item.OperationDescription != "尝试全站置顶笔记 #59" || item.ResultDescription != "已拒绝：无此权限" {
		t.Fatalf("unexpected presentation: %#v", item)
	}
}

func TestExportAdminAuditLogsStreamsAllFilteredRowsAsUtf8BOMCSV(t *testing.T) {
	db, primary := setupAdminAuditControllerTest(t)
	records := []models.AdminAuditLog{
		{
			ActorUserID: primary.ID, ActorUsername: primary.Username, ActorIsPrimary: true,
			Capability: "notes.pin_global", Module: "notes", Action: "set_global_pin",
			TargetType: "message", TargetID: "59", Result: "success",
			Summary: "global pin mutation completed", IP: "198.51.100.4", UserAgent: "secret-agent",
		},
		{
			ActorUserID: primary.ID, ActorUsername: primary.Username, ActorIsPrimary: true,
			Capability: "notes.pin_global", Module: "notes", Action: "set_global_pin",
			TargetType: "message", TargetID: "60", Result: "failure", Reason: string(authorization.DenialMissingGrant),
			Summary: "token=secret-value global pin failure", IP: "198.51.100.5", UserAgent: "another-secret-agent",
		},
		{
			ActorUserID: primary.ID, ActorUsername: primary.Username, ActorIsPrimary: true,
			Capability: "security.view", Module: "security", Action: "GET",
			TargetType: "route", TargetID: "/security", Result: "denied",
			Summary: "capability request denied",
		},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.GET("/audit", ListAdminAuditLogs)
	r.GET("/audit/export", ExportAdminAuditLogs)
	list := httptest.NewRecorder()
	r.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/audit?module=notes&q=global+pin", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("filtered list status=%d body=%s", list.Code, list.Body.String())
	}
	var listBody struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if listBody.Data.Total != 2 {
		t.Fatalf("filtered list total=%d, want 2", listBody.Data.Total)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/audit/export?module=notes&q=global+pin", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("export status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("unexpected download headers: %#v", w.Header())
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("export must start with UTF-8 BOM")
	}
	reader := csv.NewReader(bytes.NewReader(w.Body.Bytes()[3:]))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("expected metadata, header, and all two matching rows; got %d rows: %#v", len(rows), rows)
	}
	if rows[0][0] != "筛选条件" || !strings.Contains(rows[0][1], "模块=notes") || !strings.Contains(rows[0][1], "关键词已设置") {
		t.Fatalf("unexpected filter metadata: %#v", rows[0])
	}
	if got, want := rows[1], []string{"时间", "操作人", "操作说明", "结果", "模块", "动作", "能力", "摘要", "目标类型", "目标 ID", "拒绝原因"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("csv header=%#v, want %#v", got, want)
	}
	content := w.Body.String()
	if strings.Contains(content, "secret-value") || strings.Contains(content, "198.51.100") || strings.Contains(content, "secret-agent") {
		t.Fatalf("export leaked sensitive audit fields: %s", content)
	}
}

func TestGetAdminAuditLogIncludesChinesePresentation(t *testing.T) {
	db, primary := setupAdminAuditControllerTest(t)
	log := models.AdminAuditLog{
		ActorUserID: primary.ID, ActorUsername: primary.Username, ActorIsPrimary: true,
		Capability: "notes.pin_global", Module: "notes", Action: "unset_global_pin",
		TargetType: "message", TargetID: "59", Result: "success", Summary: "global pin mutation completed",
	}
	if err := db.Create(&log).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/audit/:id", func(c *gin.Context) { c.Set("user_id", primary.ID); c.Next() }, GetAdminAuditLog)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/audit/"+fmt.Sprint(log.ID), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			OperationDescription string `json:"operation_description"`
			ResultDescription    string `json:"result_description"`
			Module               string `json:"module"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.OperationDescription != "已取消笔记 #59的全站置顶" || body.Data.ResultDescription != "成功" || body.Data.Module != "notes" {
		t.Fatalf("unexpected detail presentation: %#v", body)
	}
}

func TestAuditListDetailAndExportRedactInvisibleMessageDenials(t *testing.T) {
	db, primary := setupAdminAuditControllerTest(t)
	ownerID := uint(9090)
	log := models.AdminAuditLog{
		ActorUserID: primary.ID, ActorUsername: primary.Username, ActorIsPrimary: true,
		Capability: string(authorization.CapabilityNotesEdit), Module: "notes", Action: "update",
		TargetType: "message", TargetID: "4242", TargetOwnerUserID: &ownerID,
		Result: "denied", Reason: string(authorization.DenialContentNotReadable),
		Summary: "private body visibility=private author=secret",
	}
	if err := db.Create(&log).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", primary.ID); c.Next() })
	r.GET("/audit", ListAdminAuditLogs)
	r.GET("/audit/:id", GetAdminAuditLog)
	r.GET("/audit/export", ExportAdminAuditLogs)

	list := httptest.NewRecorder()
	r.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/audit?module=notes", nil))
	var listBody struct {
		Data struct {
			Items []models.AdminAuditLog `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Data.Items) != 1 {
		t.Fatalf("unexpected list response: %s", list.Body.String())
	}
	assertRedactedAudit := func(label string, audit models.AdminAuditLog) {
		t.Helper()
		if audit.TargetID != "" || audit.TargetOwnerUserID != nil || audit.Summary != "capability request denied" {
			t.Fatalf("%s leaked invisible target data: %#v", label, audit)
		}
	}
	assertRedactedAudit("list", listBody.Data.Items[0])
	filtered := httptest.NewRecorder()
	r.ServeHTTP(filtered, httptest.NewRequest(http.MethodGet, "/audit?target_id=4242", nil))
	var filteredBody struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(filtered.Body.Bytes(), &filteredBody); err != nil {
		t.Fatal(err)
	}
	if filteredBody.Data.Total != 0 {
		t.Fatalf("target-id filtering exposed a redacted denial through total: %s", filtered.Body.String())
	}

	detail := httptest.NewRecorder()
	r.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/audit/"+fmt.Sprint(log.ID), nil))
	var detailBody struct {
		Data models.AdminAuditLog `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailBody); err != nil {
		t.Fatal(err)
	}
	assertRedactedAudit("detail", detailBody.Data)

	export := httptest.NewRecorder()
	r.ServeHTTP(export, httptest.NewRequest(http.MethodGet, "/audit/export?module=notes", nil))
	exported := export.Body.String()
	for _, secret := range []string{"4242", "9090", "private body", "visibility=private", "author=secret"} {
		if strings.Contains(exported, secret) {
			t.Fatalf("export leaked %q: %s", secret, exported)
		}
	}
}
