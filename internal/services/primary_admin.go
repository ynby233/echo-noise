package services

import (
	"errors"
	"fmt"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

var errPrimaryAdminUnavailable = errors.New("站长账户（用户 ID 1）不存在或不是管理员")

// loadPrimaryAdmin centralizes the invariant for features that require the
// site owner: the local user with ID 1 must exist and still be an administrator.
func loadPrimaryAdmin(db *gorm.DB) (*models.User, error) {
	if db == nil {
		return nil, fmt.Errorf("读取站长账户失败: 数据库未初始化")
	}

	var primary models.User
	err := db.Where("id = ? AND is_admin = ?", models.PrimaryAdminUserID, true).First(&primary).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errPrimaryAdminUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("读取站长账户失败: %w", err)
	}
	return &primary, nil
}
