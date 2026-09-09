package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func GetStatus(c *gin.Context) {
	currentUserID, _ := currentMessageViewer(c)
	viewerID := uint(0)
	if currentUserID != nil {
		viewerID = *currentUserID
	}
	status, err := services.GetStatus(viewerID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.GetStatusFailMessage))
		return
	}

	c.JSON(http.StatusOK, dto.OK(status, models.GetStatusSuccessMessage))
}
