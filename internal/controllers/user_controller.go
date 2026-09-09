package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/services"
)

func GetUserProfile(c *gin.Context) {
	username := strings.TrimSpace(c.Query("username"))
	idStr := strings.TrimSpace(c.Query("id"))
	var user *models.User
	var err error
	if username != "" {
		user, err = services.GetUserByUsername(username)
		if err != nil || user == nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.UserNotFoundMessage))
			return
		}
	} else if idStr != "" {
		uid, parseErr := strconv.ParseUint(idStr, 10, 64)
		if parseErr != nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
			return
		}
		user, err = services.GetUserByID(uint(uid))
		if err != nil || user == nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.UserNotFoundMessage))
			return
		}
	} else {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}
	var total int64
	currentUserID, _ := currentMessageViewer(c)
	q := services.ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), currentUserID).
		Where("user_id = ?", user.ID).
		Where(services.GuestbookSQLPredicate("messages.is_guestbook")).
		Where("content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ?",
			"%#友链%", "%友情链接%",
			"%#关于%", "%关于本站%")
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.GetAllMessagesFailMessage))
		return
	}
	c.JSON(http.StatusOK, dto.OK(map[string]interface{}{
		"id":             user.ID,
		"username":       user.Username,
		"avatar_url":     strings.TrimSpace(user.AvatarURL),
		"description":    strings.TrimSpace(user.Description),
		"total_messages": int(total),
	}, "获取用户资料成功"))
}

func UpdateUser(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	var userdto dto.UserInfoDto
	if err := c.ShouldBindJSON(&userdto); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}

	if err := services.UpdateUser(user, userdto); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	safeUser := *user
	safeUser.Password = ""
	c.JSON(http.StatusOK, dto.OK(safeUser, models.UpdateUserSuccessMessage))
}

func ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		Password    string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}

	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	if err := services.ChangePasswordWithOld(user, req.OldPassword, req.Password); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](services.PasswordChangePublicFailureMessage(err, false)))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, models.ChangePasswordSuccessMessage))
}

func UpdateUserAdmin(c *gin.Context) {
	_, err := requirePrimaryAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	idStr := c.Query("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 1 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}

	currentID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	if err := services.UpdateUserAdmin(uint(id), currentID); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, models.UpdateUserSuccessMessage))
}

type registrationApplicationReviewRequest struct {
	Note string `json:"note"`
}

func ListRegistrationApplications(c *gin.Context) {
	viewerUserID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	result, err := services.ListRegistrationApplicationsForViewer(viewerUserID, status, limit, offset)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("获取注册申请失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK(result, models.QuerySuccessMessage))
}

func ApproveRegistrationApplication(c *gin.Context) {
	reviewerUserID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	var req registrationApplicationReviewRequest
	_ = c.ShouldBindJSON(&req)
	user, err := services.ApproveRegistrationApplication(uint(id), reviewerUserID, req.Note)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	user.Password = ""
	services.RedactManagedUserVoceChatEmailForViewer(reviewerUserID, user)
	c.JSON(http.StatusOK, dto.OK(user, models.UpdateUserSuccessMessage))
}

func RejectRegistrationApplication(c *gin.Context) {
	reviewerUserID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	var req registrationApplicationReviewRequest
	_ = c.ShouldBindJSON(&req)
	if err := services.RejectRegistrationApplication(uint(id), reviewerUserID, req.Note); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, models.UpdateUserSuccessMessage))
}

func GetUserInfo(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	// 返回副本，避免污染缓存中的用户对象。
	safeUser := *user
	safeUser.Password = ""
	c.JSON(http.StatusOK, dto.OK(safeUser, models.QuerySuccessMessage))
}

func BindPrimaryAdminVoceChatEmail(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var request struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](models.InvalidRequestBodyMessage))
		return
	}
	bound, err := services.BindPrimaryAdminVoceChatEmail(c.Request.Context(), user.ID, request.Email, request.Password)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	bound.Password = ""
	c.JSON(http.StatusOK, dto.OK(bound, "注册绑定 VoceChat 邮箱已更新"))
}

func EmailTest(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	var req struct {
		To string `json:"to" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("无效的请求参数"))
		return
	}
	if req.To == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("收件人不能为空"))
		return
	}
	if err := models.SendTestEmail(req.To); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "测试邮件已发送"))
}
func BindEmail(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	db, _ := database.GetDB()
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil || !cfg.SmtpEnabled {
		c.JSON(http.StatusOK, dto.Fail[any]("邮件未开启"))
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if e := c.ShouldBindJSON(&req); e != nil || strings.TrimSpace(req.Email) == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("邮箱不能为空"))
		return
	}
	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	exp := time.Now().Add(10 * time.Minute)
	user.EmailPending = strings.TrimSpace(req.Email)
	user.EmailVerifyCode = code
	user.EmailVerifyExpires = &exp
	if e := repository.UpdateUser(user); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存失败"))
		return
	}
	if e := models.SendEmail(user.EmailPending, "邮箱绑定验证码", "验证码: "+code+"，10分钟内有效"); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any](e.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "验证码已发送"))
}

func VerifyEmail(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if e := c.ShouldBindJSON(&req); e != nil || strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码不能为空"))
		return
	}
	if user.EmailVerifyExpires == nil || time.Now().After(*user.EmailVerifyExpires) {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码已过期"))
		return
	}
	if strings.TrimSpace(req.Code) != strings.TrimSpace(user.EmailVerifyCode) {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码错误"))
		return
	}
	if strings.TrimSpace(user.EmailPending) == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("无待绑定邮箱"))
		return
	}
	user.Email = user.EmailPending
	user.EmailPending = ""
	user.EmailVerified = true
	user.EmailVerifyCode = ""
	user.EmailVerifyExpires = nil
	if e := repository.UpdateUser(user); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("更新失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "邮箱已绑定"))
}

func SendChangeEmailCode(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	if strings.TrimSpace(user.Email) == "" || !user.EmailVerified {
		c.JSON(http.StatusOK, dto.Fail[any]("未绑定邮箱或未验证"))
		return
	}
	db, _ := database.GetDB()
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil || !cfg.SmtpEnabled {
		c.JSON(http.StatusOK, dto.Fail[any]("邮件未开启"))
		return
	}
	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	exp := time.Now().Add(10 * time.Minute)
	user.EmailVerifyCode = code
	user.EmailVerifyExpires = &exp
	if e := repository.UpdateUser(user); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存失败"))
		return
	}
	if e := models.SendEmail(strings.TrimSpace(user.Email), "更换邮箱验证码", "验证码: "+code+"，10分钟内有效"); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any](e.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "验证码已发送"))
}

func ChangeEmail(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var req struct {
		Code     string `json:"code"`
		NewEmail string `json:"newEmail"`
	}
	if e := c.ShouldBindJSON(&req); e != nil || strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.NewEmail) == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("参数错误"))
		return
	}
	if user.EmailVerifyExpires == nil || time.Now().After(*user.EmailVerifyExpires) {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码已过期"))
		return
	}
	if strings.TrimSpace(req.Code) != strings.TrimSpace(user.EmailVerifyCode) {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码错误"))
		return
	}
	// 第一步验证通过旧邮箱验证码，进入第二步：向新邮箱发送验证码并挂起
	newEmail := strings.TrimSpace(req.NewEmail)
	code2 := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	exp2 := time.Now().Add(10 * time.Minute)
	user.EmailPending = newEmail
	user.EmailVerified = false
	user.EmailVerifyCode = code2
	user.EmailVerifyExpires = &exp2
	if e := repository.UpdateUser(user); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存失败"))
		return
	}
	if e := models.SendEmail(newEmail, "新邮箱验证", "验证码: "+code2+"，10分钟内有效"); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any](e.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "已向新邮箱发送验证码，请完成验证"))
}

// 删除用户
func DeleteUser(c *gin.Context) {
	currentID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	userIDStr := c.Query("id")
	if userIDStr == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("缺少用户ID"))
		return
	}
	id, _ := strconv.Atoi(userIDStr)

	// 不允许删除自己
	if uint(id) == currentID {
		c.JSON(http.StatusOK, dto.Fail[string]("不允许删除当前登录用户"))
		return
	}
	// 至少保留一位管理员：当删除目标是管理员时检查数量
	target, err := repository.GetUserByID(uint(id))
	if err == nil && target.IsAdmin {
		if target.ID == models.PrimaryAdminUserID {
			c.JSON(http.StatusForbidden, dto.Fail[string]("不能删除站长"))
			return
		}
		if currentID != models.PrimaryAdminUserID {
			c.JSON(http.StatusForbidden, dto.Fail[string]("仅站长可以删除管理员账号"))
			return
		}
		cnt, err := repository.CountAdmins()
		if err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("校验管理员数量失败"))
			return
		}
		if cnt <= 1 {
			c.JSON(http.StatusOK, dto.Fail[string]("系统至少保留一位管理员，无法删除最后一位管理员"))
			return
		}
	}
	if err := repository.DeleteUser(uint(id)); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("删除失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "已删除用户"))
}

// ResetManagedUserPassword applies the user-management password-reset matrix.
func ResetManagedUserPassword(c *gin.Context) {
	var req struct {
		ID       uint   `json:"id"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == 0 || strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}
	user, err := services.GetUserByID(req.ID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.UserNotFoundMessage))
		return
	}
	actor, err := checkUser(c)
	if err != nil || actor == nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[string](models.UserNotFoundMessage))
		return
	}
	if user.ID == models.PrimaryAdminUserID || (user.IsAdmin && actor.ID != models.PrimaryAdminUserID) {
		c.JSON(http.StatusForbidden, dto.Fail[string]("无权重置该用户密码"))
		return
	}
	if err := services.ChangePassword(user, dto.UserInfoDto{Password: req.Password}); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](services.PasswordChangePublicFailureMessage(err, true)))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "密码重置成功"))
}
