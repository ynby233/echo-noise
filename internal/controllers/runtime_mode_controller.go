package controllers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/runtimepolicy"
	"github.com/rcy1314/echo-noise/internal/services"
)

func GetRuntimePolicy(c *gin.Context) {
	actorID, err := requirePrimaryAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	diagnostics, err := services.GetRuntimePolicyDiagnostics(actorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取运行模式失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(diagnostics, "获取运行模式成功"))
}

func UpdateRuntimePolicyMode(c *gin.Context) {
	actorID, err := requirePrimaryAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	var request struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("运行模式参数错误"))
		return
	}
	mode, ok := runtimepolicy.ParseConfiguredMode(request.Mode)
	if !ok {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("运行模式参数错误"))
		return
	}
	policy, err := services.SwitchConfiguredMode(c.Request.Context(), actorID, mode)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrRuntimeModeInvalid):
			c.JSON(http.StatusBadRequest, dto.Fail[any]("运行模式参数错误"))
		case errors.Is(err, services.ErrRuntimeModeConfigIncomplete):
			c.JSON(http.StatusConflict, dto.Fail[any]("VoceChat 管理配置不完整，请先保存服务地址和管理员凭据"))
		case errors.Is(err, services.ErrRuntimeModeHealthCheckFailed):
			c.JSON(http.StatusConflict, dto.Fail[any]("VoceChat 健康检查未通过，已保持本地模式"))
		default:
			c.JSON(http.StatusInternalServerError, dto.Fail[any]("切换运行模式失败"))
		}
		return
	}
	c.JSON(http.StatusOK, dto.OK(policy, "运行模式已更新"))
}

func StartVoceChatProvisioning(c *gin.Context) {
	handleVoceChatProvisioningCommand(c, services.StartVoceChatProvisioning)
}

func RetryVoceChatProvisioning(c *gin.Context) {
	handleVoceChatProvisioningCommand(c, services.RetryVoceChatProvisioningFailures)
}

func handleVoceChatProvisioningCommand(c *gin.Context, command func(context.Context, uint) (services.RuntimePolicyDiagnostics, error)) {
	actorID, err := requirePrimaryAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	diagnostics, err := command(c.Request.Context(), actorID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrVoceChatProvisioningPrimaryRequired):
			c.JSON(http.StatusForbidden, dto.Fail[any]("仅站长可执行 VoceChat 账户补建"))
		case errors.Is(err, services.ErrVoceChatProvisioningModeRequired):
			c.JSON(http.StatusConflict, dto.Fail[any]("请先确认 VoceChat 配置和健康检查正常，再启动账户补建"))
		default:
			c.JSON(http.StatusInternalServerError, dto.Fail[any]("启动 VoceChat 账户补建失败"))
		}
		return
	}
	c.JSON(http.StatusOK, dto.OK(diagnostics, "VoceChat 账户补建任务已更新"))
}
