package controllers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
)

// checkAdmin validates the shared administrator boundary used by controller domains.
func checkAdmin(c *gin.Context) (uint, error) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, fmt.Errorf("未授权访问")
	}
	id, ok := commentUint(userID)
	if !ok || id == 0 {
		return 0, fmt.Errorf("未授权访问")
	}
	db, err := database.GetDB()
	if err != nil {
		return 0, err
	}
	var user models.User
	if err := db.Select("id,is_admin").First(&user, id).Error; err != nil {
		return 0, err
	}
	if !user.IsAdmin {
		return 0, fmt.Errorf("需要管理员权限")
	}
	return id, nil
}

func requirePrimaryAdmin(c *gin.Context) (uint, error) {
	userID, err := checkAdmin(c)
	if err != nil {
		return 0, err
	}
	if userID != models.PrimaryAdminUserID {
		return 0, fmt.Errorf("仅站长可执行此操作")
	}
	return userID, nil
}
