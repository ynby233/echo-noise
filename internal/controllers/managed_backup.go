package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	backupservice "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
)

const managedBackupUploadLimit int64 = 2 << 30

func managedBackupKind(kind string) bool {
	return kind == "data" || kind == "attachments" || kind == "server-config"
}

func managedBackupCapability(kind, action string) authorization.Capability {
	return authorization.Capability("backup." + strings.ReplaceAll(kind, "-", "_") + "." + action)
}

func managedBackupAllowed(c *gin.Context, kind, action string) bool {
	actor, err := checkAdmin(c)
	if err != nil || !managedBackupKind(kind) {
		return false
	}
	if kind == "server-config" || action == "download" || action == "restore" {
		return actor == models.PrimaryAdminUserID
	}
	db, err := database.GetDB()
	if err != nil {
		return false
	}
	return authorization.New(db).Authorize(actor, managedBackupCapability(kind, action), nil).Allowed
}

func managedBackupRequire(c *gin.Context, kind, action string) bool {
	if !managedBackupKind(kind) {
		managedBackupError(c, http.StatusBadRequest, errors.New("无效备份类型"))
		return false
	}
	if !managedBackupAllowed(c, kind, action) {
		managedBackupError(c, http.StatusForbidden, errors.New("没有此备份操作权限"))
		return false
	}
	return true
}

func managedBackupError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"code": 0, "msg": err.Error(), "message": err.Error()})
}

func managedBackupJSON(c *gin.Context, target interface{}) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("请求参数错误: %w", err)
	}
	if err := decoder.Decode(new(interface{})); !errors.Is(err, io.EOF) {
		return errors.New("请求必须仅包含一个 JSON 对象")
	}
	return nil
}

func HandleManagedBackupStatus(c *gin.Context) {
	visible := map[string]bool{}
	for _, kind := range []string{"data", "attachments", "server-config"} {
		visible[kind] = managedBackupAllowed(c, kind, "view")
	}
	if !visible["data"] && !visible["attachments"] && !visible["server-config"] {
		managedBackupError(c, http.StatusForbidden, errors.New("没有备份查看权限"))
		return
	}
	plans, err := syncmanager.DefaultManager.Plans()
	if err != nil {
		managedBackupError(c, http.StatusInternalServerError, err)
		return
	}
	filtered := make([]models.ManagedBackupPlan, 0, len(plans))
	directories := map[string]string{}
	for _, plan := range plans {
		if visible[plan.Kind] {
			filtered = append(filtered, plan)
			directories[plan.Kind] = syncmanager.DefaultManager.Directory(plan.Kind)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"plans": filtered, "directories": directories, "warnings": []string{}, "pendingRestart": backupservice.HasPendingRestore(backupservice.DefaultLayout())}})
}

func HandleManagedBackupPlan(c *gin.Context) {
	kind := c.Param("kind")
	if !managedBackupRequire(c, kind, "plan") {
		return
	}
	var input struct {
		Enabled   bool   `json:"enabled"`
		Time      string `json:"time"`
		Timezone  string `json:"timezone"`
		KeepLocal int    `json:"keepLocal"`
		KeepCloud int    `json:"keepCloud"`
		Target    string `json:"target"`
		Scope     string `json:"scope"`
	}
	if err := managedBackupJSON(c, &input); err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	plan, err := syncmanager.DefaultManager.SavePlan(models.ManagedBackupPlan{Kind: kind, Enabled: input.Enabled, Time: input.Time, Timezone: input.Timezone, KeepLocal: input.KeepLocal, KeepCloud: input.KeepCloud, Target: input.Target, Scope: input.Scope})
	if err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": plan})
}

func HandleManagedBackupCreate(c *gin.Context) {
	kind := c.Param("kind")
	if !managedBackupRequire(c, kind, "create") {
		return
	}
	var input struct {
		Target string `json:"target"`
		Scope  string `json:"scope"`
	}
	if err := managedBackupJSON(c, &input); err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	record, err := syncmanager.DefaultManager.Create(c.Request.Context(), kind, input.Target, input.Scope, "manual")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": err.Error(), "message": err.Error(), "data": gin.H{"record": record}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"record": record, "message": "备份已生成"}})
}

func HandleManagedBackupHistory(c *gin.Context) {
	kind := c.Param("kind")
	if !managedBackupRequire(c, kind, "history") {
		return
	}
	records, err := syncmanager.DefaultManager.HistoryContext(c.Request.Context(), kind)
	if err != nil {
		managedBackupError(c, http.StatusInternalServerError, err)
		return
	}
	if records == nil {
		records = []models.ManagedBackupRecord{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"records": records, "warnings": syncmanager.DefaultManager.DiscoveryWarnings(kind), "pendingRestart": backupservice.HasPendingRestore(backupservice.DefaultLayout())}})
}

func managedBackupRecord(c *gin.Context, action, source string) (models.ManagedBackupRecord, io.ReadCloser, string, error) {
	kind := c.Param("kind")
	if !managedBackupRequire(c, kind, action) {
		return models.ManagedBackupRecord{}, nil, "", errors.New("forbidden")
	}
	if source != "local" && source != "cloud" {
		return models.ManagedBackupRecord{}, nil, "", errors.New("请选择 local 或 cloud 来源")
	}
	record, err := syncmanager.DefaultManager.FetchRecord(c.Request.Context(), c.Param("id"))
	if err != nil {
		return record, nil, "", err
	}
	if record.Kind != kind {
		return record, nil, "", errors.New("备份记录类型不匹配")
	}
	reader, name, err := syncmanager.DefaultManager.OpenSource(c.Request.Context(), record.ID, source)
	return record, reader, name, err
}

func HandleManagedBackupDownload(c *gin.Context) {
	release, lockErr := syncmanager.LockOperationContext(c.Request.Context())
	if lockErr != nil {
		managedBackupError(c, http.StatusRequestTimeout, lockErr)
		return
	}
	defer release()
	_, reader, name, err := managedBackupRecord(c, "download", c.DefaultQuery("source", "local"))
	if err != nil {
		if !c.Writer.Written() {
			managedBackupError(c, http.StatusBadRequest, err)
		}
		return
	}
	defer reader.Close()
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(name)))
	c.Header("Cache-Control", "no-store")
	if _, err := io.Copy(c.Writer, reader); err != nil {
		_ = c.Error(err)
	}
}

type managedRestoreInput struct {
	Confirm               bool   `json:"confirm"`
	Source                string `json:"source"`
	AttachmentMode        string `json:"attachmentMode"`
	ConfirmLocalMigration bool   `json:"confirmLocalMigration"`
}

func validateManagedRestore(input managedRestoreInput) error {
	if !input.Confirm {
		return errors.New("恢复前必须明确确认")
	}
	if input.AttachmentMode != "original" && input.AttachmentMode != "local" {
		return errors.New("请选择 original 或 local 附件恢复模式")
	}
	if input.AttachmentMode == "local" && !input.ConfirmLocalMigration {
		return errors.New("转为本地存储前必须再次明确确认")
	}
	return nil
}

func HandleManagedBackupRestoreRecord(c *gin.Context) {
	if !managedBackupRequire(c, c.Param("kind"), "restore") {
		return
	}
	var input managedRestoreInput
	if err := managedBackupJSON(c, &input); err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	if input.AttachmentMode == "" {
		input.AttachmentMode = "original"
	}
	if err := validateManagedRestore(input); err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	release, lockErr := syncmanager.LockOperationContext(c.Request.Context())
	if lockErr != nil {
		managedBackupError(c, http.StatusRequestTimeout, lockErr)
		return
	}
	defer release()
	_, reader, _, err := managedBackupRecord(c, "restore", input.Source)
	if err != nil {
		if !c.Writer.Written() {
			managedBackupError(c, http.StatusBadRequest, err)
		}
		return
	}
	defer reader.Close()
	path, cleanup, err := managedBackupSpoolLimit(c.Request.Context(), reader, int64(backupservice.MaxArchiveTotalBytes))
	if err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	defer cleanup()
	managedBackupStage(c, path, input.AttachmentMode, c.Param("kind"))
}

func managedBackupSpool(ctx context.Context, reader io.Reader) (string, func(), error) {
	return managedBackupSpoolLimit(ctx, reader, managedBackupUploadLimit)
}

func managedBackupSpoolLimit(ctx context.Context, reader io.Reader, limit int64) (string, func(), error) {
	file, err := os.CreateTemp("", "echo-noise-managed-upload-*.zip")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	limited := io.LimitReader(reader, limit+1)
	buffer := make([]byte, 128<<10)
	var size int64
	for {
		if err = ctx.Err(); err != nil {
			break
		}
		n, readErr := limited.Read(buffer)
		if n > 0 {
			var written int
			written, err = file.Write(buffer[:n])
			size += int64(written)
			if err != nil {
				break
			}
			if written != n {
				err = io.ErrShortWrite
				break
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				err = readErr
			}
			break
		}
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if size > limit {
		err = fmt.Errorf("备份文件超过 %d GiB 读取上限", limit>>30)
	}
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return file.Name(), cleanup, nil
}

func managedBackupUpload(c *gin.Context) (string, func(), error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, managedBackupUploadLimit+(1<<20))
	file, err := c.FormFile("file")
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		return "", func() {}, errors.New("请选择备份文件")
	}
	if file.Size > managedBackupUploadLimit {
		return "", func() {}, errors.New("备份文件超过 2 GiB 上传上限")
	}
	reader, err := file.Open()
	if err != nil {
		return "", func() {}, err
	}
	defer reader.Close()
	return managedBackupSpool(c.Request.Context(), reader)
}

func managedBackupMayUpload(c *gin.Context) bool {
	for _, kind := range []string{"data", "attachments", "server-config"} {
		if managedBackupAllowed(c, kind, "restore") {
			return true
		}
	}
	managedBackupError(c, http.StatusForbidden, errors.New("没有备份恢复权限"))
	return false
}

func managedBackupInspectAuthorized(c *gin.Context, path string) (backupservice.TypedArchiveInspection, error) {
	inspection, err := backupservice.InspectTypedArchive(path)
	if err != nil {
		return inspection, err
	}
	kind := string(inspection.Kind)
	if inspection.Legacy {
		if _, err := requirePrimaryAdmin(c); err != nil {
			return inspection, errors.New("完整旧版备份仅站长可恢复")
		}
		return inspection, nil
	}
	if !managedBackupAllowed(c, kind, "restore") {
		return inspection, errors.New("没有此备份类型的恢复权限")
	}
	return inspection, nil
}

func HandleManagedBackupInspect(c *gin.Context) {
	if !managedBackupMayUpload(c) {
		return
	}
	path, cleanup, err := managedBackupUpload(c)
	if err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	defer cleanup()
	inspection, err := managedBackupInspectAuthorized(c, path)
	if err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": inspection})
}

func HandleManagedBackupRestoreUpload(c *gin.Context) {
	if !managedBackupMayUpload(c) {
		return
	}
	path, cleanup, err := managedBackupUpload(c)
	if err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	defer cleanup()
	input := managedRestoreInput{Confirm: c.PostForm("confirm") == "true", AttachmentMode: c.PostForm("attachmentMode"), ConfirmLocalMigration: c.PostForm("confirmLocalMigration") == "true"}
	if input.AttachmentMode == "" {
		input.AttachmentMode = "original"
	}
	if err := validateManagedRestore(input); err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	inspection, err := managedBackupInspectAuthorized(c, path)
	if err != nil {
		managedBackupError(c, http.StatusForbidden, err)
		return
	}
	release, lockErr := syncmanager.LockOperationContext(c.Request.Context())
	if lockErr != nil {
		managedBackupError(c, http.StatusRequestTimeout, lockErr)
		return
	}
	defer release()
	managedBackupStage(c, path, input.AttachmentMode, string(inspection.Kind))
}

func managedBackupStage(c *gin.Context, path, mode, expectedKind string) {
	inspection, err := managedBackupInspectAuthorized(c, path)
	if err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	if !inspection.Legacy && string(inspection.Kind) != expectedKind {
		managedBackupError(c, http.StatusBadRequest, errors.New("备份类型与记录不匹配"))
		return
	}
	result, err := backupservice.StageTypedRestore(c.Request.Context(), path, backupservice.DefaultLayout(), mode)
	if err != nil {
		var fallback *backupservice.FallbackRequiredError
		if errors.As(err, &fallback) {
			c.JSON(http.StatusConflict, gin.H{"code": 0, "msg": err.Error(), "message": err.Error(), "data": gin.H{"fallbackRequired": true, "cloudAvailable": false}})
			return
		}
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	warnings := result.Inspection.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	if result.Warning != "" {
		warnings = append(warnings, result.Warning)
	}
	c.JSON(http.StatusAccepted, gin.H{"code": 1, "data": gin.H{"message": "备份已校验并暂存，请重启服务后完成恢复", "restartRequired": true, "pendingRestart": true, "warnings": warnings}})
}
