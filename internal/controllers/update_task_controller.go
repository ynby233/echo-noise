package controllers

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/buildinfo"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/updates"
)

func updateTaskService() (*updates.TaskService, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return updates.NewTaskService(db), nil
}

// U2 deliberately does not expose installation until the U3/U4 executor and
// backup verification path exists. The coordinator itself is exercised with
// isolated database tests; a real HTTP client cannot report a fake success.
func UpdateTaskInstallationUnavailable(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, dto.Fail[any]("更新任务协调接口已就绪；宿主执行与备份尚未接入，当前不可安装"))
}

func GetUpdates(c *gin.Context) {
	actorID, ok := commentUint(c.GetUint("user_id"))
	if !ok || actorID == 0 {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录"))
		return
	}
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	report := discoverUpdates(c)
	channel, err := service.GetChannel()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取更新设置失败"))
		return
	}
	credential, err := service.CurrentCredential()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取执行器状态失败"))
		return
	}
	instanceID := ""
	if actorID == models.PrimaryAdminUserID {
		instanceID, err = service.InstanceID()
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取实例身份失败"))
			return
		}
	}
	if actorID != models.PrimaryAdminUserID {
		report.Installed.Revision, report.Installed.Digest = "", ""
		report.Source.Revision, report.Release.Revision = "", ""
		for index := range report.Channels {
			report.Channels[index].Image = ""
			report.Channels[index].Digest = ""
			report.Channels[index].Revision = ""
		}
		credential = nil
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"follow_channel": channel, "instance_id": instanceID, "report": report, "executor": credential}, "更新状态读取成功"))
}

func UpdateFollowChannel(c *gin.Context) {
	actorID, err := requirePrimaryAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	var request struct {
		Channel string `json:"channel"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("渠道参数错误"))
		return
	}
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	if err := service.SetChannel(actorID, request.Channel); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("渠道参数错误"))
		return
	}
	writeUpdateAudit(c, actorID, "set_channel", "update_preference", "1", "updated follow channel")
	c.JSON(http.StatusOK, dto.OK(gin.H{"channel": strings.ToLower(strings.TrimSpace(request.Channel))}, "跟随渠道已保存；当前安装保持不变"))
}

func CreateUpdateTask(c *gin.Context) {
	actorID, err := requirePrimaryAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	var request struct {
		Channel string `json:"channel"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("更新任务参数错误"))
		return
	}
	channelName := strings.ToLower(strings.TrimSpace(request.Channel))
	if channelName != "stable" && channelName != "edge" {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("更新渠道无效"))
		return
	}
	target := discoverUpdates(c).Channel(channelName)
	if !target.Installable || !target.HasUpdate || target.Status != updates.StatusUpdateAvailable {
		c.JSON(http.StatusConflict, dto.Fail[any]("所选渠道当前没有可自动安装的后代版本"))
		return
	}
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	task, created, err := service.Create(actorID, updates.Target{Channel: target.Name, Image: target.Image, Digest: target.Digest, Revision: target.Revision, Version: target.Version})
	if err != nil {
		if errors.Is(err, updates.ErrExecutorNotConfigured) {
			c.JSON(http.StatusPreconditionFailed, dto.Fail[any]("更新执行器尚未配置"))
			return
		}
		c.JSON(http.StatusConflict, dto.Fail[any]("无法创建更新任务"))
		return
	}
	if created {
		writeUpdateAudit(c, actorID, "create_task", "update_task", task.PublicID, "created pinned update task")
		c.JSON(http.StatusCreated, dto.OK(task, "更新任务已创建"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(task, "已有更新任务，未重复创建"))
}

func GetUpdateTask(c *gin.Context) {
	actorID, ok := commentUint(c.GetUint("user_id"))
	if !ok || actorID == 0 {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录"))
		return
	}
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	task, err := service.Get(c.Param("id"))
	if errors.Is(err, updates.ErrTaskNotFound) {
		c.JSON(http.StatusNotFound, dto.Fail[any]("更新任务不存在"))
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取更新任务失败"))
		return
	}
	if actorID != models.PrimaryAdminUserID {
		task.TargetImage, task.TargetDigest, task.TargetRevision, task.ErrorSummary = "", "", "", ""
	}
	c.JSON(http.StatusOK, dto.OK(task, "更新任务读取成功"))
}

func GetExecutorCredential(c *gin.Context) {
	if _, err := requirePrimaryAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	credential, err := service.CurrentCredential()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取执行器凭据失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(credential, "执行器凭据状态读取成功"))
}

func CreateExecutorCredential(c *gin.Context) {
	actorID, err := requirePrimaryAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("执行器名称参数错误"))
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if len([]rune(request.Name)) > 100 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("执行器名称过长"))
		return
	}
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	credential, raw, err := service.CreateCredential(actorID, request.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("创建执行器凭据失败"))
		return
	}
	writeUpdateAudit(c, actorID, "rotate_executor_credential", "update_executor", "1", "rotated update executor credential")
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, dto.OK(gin.H{"credential": credential, "token": raw}, "凭据仅显示一次，请立即保存"))
}

func RevokeExecutorCredential(c *gin.Context) {
	actorID, err := requirePrimaryAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	if err := service.RevokeCredential(actorID); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("撤销执行器凭据失败"))
		return
	}
	writeUpdateAudit(c, actorID, "revoke_executor_credential", "update_executor", "1", "revoked update executor credential")
	c.JSON(http.StatusOK, dto.OK[any](nil, "执行器凭据已撤销"))
}

func ClaimUpdateTask(c *gin.Context) {
	credentialID := c.GetUint("executor_credential_id")
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	task, err := service.Claim(credentialID)
	if errors.Is(err, updates.ErrNoPendingTask) {
		c.Status(http.StatusNoContent)
		return
	}
	if err != nil {
		c.JSON(http.StatusConflict, dto.Fail[any]("无法领取更新任务"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(task, "更新任务领取成功"))
}

func RecordUpdateTaskEvent(c *gin.Context) {
	var request struct {
		Status  string `json:"status"`
		Summary string `json:"summary"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("任务事件参数错误"))
		return
	}
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	err = service.RecordEvent(c.GetUint("executor_credential_id"), c.Param("id"), request.Status, request.Summary)
	switch {
	case errors.Is(err, updates.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, dto.Fail[any]("更新任务不存在"))
	case errors.Is(err, updates.ErrTaskOwnership):
		c.JSON(http.StatusForbidden, dto.Fail[any]("无权回报此任务"))
	case errors.Is(err, updates.ErrInvalidTransition):
		c.JSON(http.StatusConflict, dto.Fail[any]("任务状态转换无效"))
	case err != nil:
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("记录任务事件失败"))
	default:
		c.JSON(http.StatusOK, dto.OK[any](nil, "任务事件已记录"))
	}
}

func GetExecutorRuntime(c *gin.Context) {
	service, err := updateTaskService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("更新服务不可用"))
		return
	}
	instanceID, err := service.InstanceID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取实例身份失败"))
		return
	}
	metadata := buildinfo.CurrentMetadata()
	c.JSON(http.StatusOK, dto.OK(gin.H{"instance_id": instanceID, "build_identity": metadata.Identity, "version": metadata.Version, "revision": metadata.Revision, "built_at": metadata.BuiltAt, "image_digest": strings.TrimSpace(os.Getenv("IMAGE_DIGEST"))}, "运行身份读取成功"))
}

func writeUpdateAudit(c *gin.Context, actorID uint, action, targetType, targetID, summary string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}
	authorization.New(db).WriteAuditBestEffort(models.AdminAuditLog{ActorUserID: actorID, Capability: string(authorization.CapabilityVersionUpdate), Module: "version", Action: action, TargetType: targetType, TargetID: targetID, Result: "success", Summary: summary, IP: c.ClientIP(), UserAgent: c.GetHeader("User-Agent"), AuthVia: c.GetString("auth_via")})
	middleware.MarkSemanticAuditWritten(c)
}
