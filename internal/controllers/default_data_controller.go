package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/services"
)

func ResetDefaultData(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	if err := services.SeedDefaultData(); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("重置失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, "重置成功"))
}
