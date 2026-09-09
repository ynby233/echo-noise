package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
)

// Friend-link application and moderation endpoints stay together in this controller.
func SubmitFriendLinkApply(c *gin.Context) {
	var req struct {
		Title       string `json:"title"`
		Link        string `json:"link"`
		Icon        string `json:"icon"`
		Description string `json:"description"`
		Email       string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("请求参数错误"))
		return
	}
	req.Link = strings.TrimSpace(req.Link)
	req.Title = strings.TrimSpace(req.Title)
	req.Icon = strings.TrimSpace(req.Icon)
	req.Description = strings.TrimSpace(req.Description)
	req.Email = strings.TrimSpace(req.Email)
	if req.Link == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("网址不能为空"))
		return
	}
	db, _ := database.GetDB()
	apply := models.FriendLinkApply{Title: req.Title, Link: req.Link, Icon: req.Icon, Description: req.Description, Email: req.Email, Status: "pending"}
	if err := db.Create(&apply).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("提交失败"))
		return
	}
	var cfg models.SiteConfig
	_ = db.Table("site_configs").First(&cfg).Error
	if cfg.SmtpEnabled && cfg.FriendLinkEmailEnabled {
		to := strings.TrimSpace(cfg.SmtpFrom)
		if to == "" {
			to = strings.TrimSpace(cfg.SmtpUser)
		}
		if to != "" {
			subject := fmt.Sprintf("新的友链申请 - %s", cfg.SiteTitle)
			body := fmt.Sprintf("站点：%s\n标题：%s\n网址：%s\n邮箱：%s\n说明：%s", cfg.SiteTitle, apply.Title, apply.Link, apply.Email, apply.Description)
			_ = models.SendEmail(to, subject, body)
		}
	}
	c.JSON(http.StatusOK, dto.OK(apply, "已提交，待审核"))
}

func ListFriendLinkApplications(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	db, _ := database.GetDB()
	var list []models.FriendLinkApply
	q := strings.TrimSpace(c.Query("q"))
	tx := db.Order("created_at DESC")
	if q != "" {
		tx = tx.Where("title LIKE ? OR link LIKE ?", "%"+q+"%", "%"+q+"%")
	}
	if err := tx.Find(&list).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(list, models.QuerySuccessMessage))
}

// DeleteFriendLinkApplication 管理员删除单条友链申请记录
func DeleteFriendLinkApplication(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	idStr := c.Param("id")
	id64, e := strconv.ParseUint(idStr, 10, 64)
	if e != nil || id64 == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	db, _ := database.GetDB()
	if err := db.Delete(&models.FriendLinkApply{}, uint(id64)).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "记录已删除"))
}

// ClearFriendLinkApplications 管理员清空友链申请记录
func ClearFriendLinkApplications(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	db, _ := database.GetDB()
	if err := db.Where("1 = 1").Delete(&models.FriendLinkApply{}).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "申请记录已清空"))
}

// AuditFriendLink 审核友链（通过/拒绝）
func AuditFriendLink(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	idStr := c.Param("id")
	id64, e := strconv.ParseUint(idStr, 10, 64)
	if e != nil || id64 == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	var req struct {
		Approve  bool   `json:"approve"`
		Feedback string `json:"feedback"`
	}
	if e := c.ShouldBindJSON(&req); e != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}
	db, _ := database.GetDB()
	var apply models.FriendLinkApply
	if err := db.First(&apply, uint(id64)).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.RecordNotFoundMessage))
		return
	}
	var cfg models.SiteConfig
	_ = db.Table("site_configs").First(&cfg).Error
	if req.Approve {
		apply.Status = "approved"
		apply.Feedback = strings.TrimSpace(req.Feedback)
		if err := db.Save(&apply).Error; err != nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
			return
		}
		link := models.FriendLink{Title: apply.Title, Link: apply.Link, Icon: apply.Icon, Description: apply.Description, Email: apply.Email}
		_ = db.Where("link = ?", link.Link).Delete(&models.FriendLink{})
		if err := db.Create(&link).Error; err != nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
			return
		}
		if cfg.SmtpEnabled && cfg.FriendLinkEmailEnabled && strings.TrimSpace(apply.Email) != "" {
			subject := fmt.Sprintf("友链申请通过 - %s", cfg.SiteTitle)
			body := fmt.Sprintf("你的友链申请已通过：%s\n%s", apply.Link, strings.TrimSpace(req.Feedback))
			_ = models.SendEmail(strings.TrimSpace(apply.Email), subject, body)
		}
		c.JSON(http.StatusOK, dto.OK(link, "已通过"))
		return
	}
	apply.Status = "rejected"
	apply.Feedback = strings.TrimSpace(req.Feedback)
	if err := db.Save(&apply).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
		return
	}
	if cfg.SmtpEnabled && cfg.FriendLinkEmailEnabled && strings.TrimSpace(apply.Email) != "" {
		subject := fmt.Sprintf("友链申请未通过 - %s", cfg.SiteTitle)
		body := fmt.Sprintf("很抱歉，你的友链申请未通过。原因：%s", strings.TrimSpace(req.Feedback))
		_ = models.SendEmail(strings.TrimSpace(apply.Email), subject, body)
	}
	c.JSON(http.StatusOK, dto.OK(apply, "已拒绝"))
}

// 动态生成 Web Manifest
