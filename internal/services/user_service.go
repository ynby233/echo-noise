package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"github.com/rcy1314/echo-noise/pkg"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const maxPendingRegistrationApplications = 5

type localLoginResult struct {
	matched        string
	usedOverride   bool
	upgradePlain   string
	upgradeAllowed bool
	needUpgrade    bool
}

type voceChatPasswordLoginFunc func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error)
type voceChatAdminUpdateUserFunc func(ctx context.Context, config vocechat.Config, uid int64, request vocechat.UpdateUserRequest) (*vocechat.User, error)

var voceChatPasswordLogin voceChatPasswordLoginFunc = defaultVoceChatPasswordLogin
var voceChatAdminUpdateUser voceChatAdminUpdateUserFunc = defaultVoceChatAdminUpdateUser

func isHexMD5String(s string) bool {
	if len(s) != 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func passwordMatchesStored(stored string, input string) bool {
	pw := strings.TrimSpace(stored)
	plain := input
	plainTrim := strings.TrimSpace(input)
	md5Plain := pkg.MD5Encrypt(plain)
	md5Trim := pkg.MD5Encrypt(plainTrim)
	isBcrypt := strings.HasPrefix(pw, "$2a$") || strings.HasPrefix(pw, "$2b$") || strings.HasPrefix(pw, "$2y$")
	if isBcrypt {
		if bcrypt.CompareHashAndPassword([]byte(pw), []byte(plain)) == nil {
			return true
		}
		if plainTrim != plain && bcrypt.CompareHashAndPassword([]byte(pw), []byte(plainTrim)) == nil {
			return true
		}
		if bcrypt.CompareHashAndPassword([]byte(pw), []byte(md5Plain)) == nil {
			return true
		}
		if bcrypt.CompareHashAndPassword([]byte(pw), []byte(strings.ToUpper(md5Plain))) == nil {
			return true
		}
		if plainTrim != plain {
			if bcrypt.CompareHashAndPassword([]byte(pw), []byte(md5Trim)) == nil {
				return true
			}
			if bcrypt.CompareHashAndPassword([]byte(pw), []byte(strings.ToUpper(md5Trim))) == nil {
				return true
			}
		}
		return false
	}
	if isHexMD5String(pw) {
		return strings.EqualFold(pw, md5Plain) || strings.EqualFold(pw, md5Trim)
	}
	return pw == plain || pw == plainTrim
}

func validateRegistrationUsername(username string) error {
	username = strings.TrimSpace(username)
	length := 0
	for _, r := range username {
		length++
		if r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || unicode.Is(unicode.Han, r) || unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			continue
		}
		return errors.New("用户名仅支持大小写英文字母、中文、日文、数字和下划线")
	}
	if length < 2 || length > 20 {
		return errors.New("用户名长度需为 2-20 个字符")
	}
	return nil
}

func generateRegistrationApplicationID() (string, error) {
	maxID, err := repository.MaxNumericRegistrationApplicationID()
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(maxID+1, 10), nil
}

func existingUser(username string) (bool, error) {
	user, err := repository.GetUserByUsername(username)
	if err == nil && user != nil && user.ID != 0 {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func usernameCaseFoldReserved(username string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, nil
	}

	users, err := repository.GetAllUsers()
	if err != nil {
		return false, err
	}
	for _, user := range users {
		if user != nil && strings.EqualFold(strings.TrimSpace(user.Username), username) {
			return true, nil
		}
	}

	applications, _, err := repository.ListRegistrationApplications(models.RegistrationApplicationStatusPending, maxPendingRegistrationApplications, 0)
	if err != nil {
		return false, err
	}
	for _, application := range applications {
		if strings.EqualFold(strings.TrimSpace(application.Username), username) {
			return true, nil
		}
	}
	return false, nil
}

func Register(userdto dto.RegisterDto) error {
	username := strings.TrimSpace(userdto.Username)
	if username == "" || userdto.Password == "" {
		return errors.New(models.UsernameOrPasswordCannotBeEmptyMessage)
	}
	if err := validateRegistrationUsername(username); err != nil {
		return err
	}

	exists, err := existingUser(username)
	if err != nil {
		return errors.New(models.DatabaseErrorMessage)
	}
	if exists {
		return errors.New(models.UsernameAlreadyExistsMessage)
	}

	hasPending, err := repository.HasPendingRegistrationApplication(username)
	if err != nil {
		return errors.New(models.DatabaseErrorMessage)
	}
	if hasPending {
		return errors.New("该用户名正在审核中")
	}

	caseFoldReserved, err := usernameCaseFoldReserved(username)
	if err != nil {
		return errors.New(models.DatabaseErrorMessage)
	}
	if caseFoldReserved {
		return errors.New(models.UsernameAlreadyExistsMessage)
	}

	pendingCount, err := repository.CountPendingRegistrationApplications()
	if err != nil {
		return errors.New(models.DatabaseErrorMessage)
	}
	if pendingCount >= maxPendingRegistrationApplications {
		return errors.New("当前待审核申请已满，请稍后再试")
	}

	hashed := models.HashPassword(userdto.Password)
	if hashed == "" {
		return errors.New("密码加密失败")
	}
	applicationID, err := generateRegistrationApplicationID()
	if err != nil {
		return errors.New("创建注册申请失败")
	}

	provision := registrationVoceChatProvision(applicationID, username, userdto.Password)
	if strings.TrimSpace(provision.Email) == "" {
		provision.Email = buildVoceChatApplicationEmail(applicationID, vocechat.DefaultEmailDomain)
	}
	if strings.TrimSpace(provision.SyncStatus) == "" {
		provision.SyncStatus = models.VoceChatSyncStatusNone
	}

	plainStore := vocechat.DefaultPlainPasswordStore()
	if err := plainStore.UpsertApplicationVoceChatPassword(applicationID, username, userdto.Password, provision.Email, provision.UserID); err != nil {
		return errors.New("创建注册申请失败")
	}

	application := models.RegistrationApplication{
		ApplicationID:      applicationID,
		Username:           username,
		PasswordHash:       hashed,
		Status:             models.RegistrationApplicationStatusPending,
		VoceChatUserID:     strings.TrimSpace(provision.UserID),
		VoceChatEmail:      strings.TrimSpace(provision.Email),
		VoceChatSyncStatus: strings.TrimSpace(provision.SyncStatus),
		VoceChatSyncError:  strings.TrimSpace(provision.SyncError),
	}
	if err := repository.CreateRegistrationApplication(&application); err != nil {
		_ = plainStore.DeleteApplicationPassword(applicationID)
		return errors.New("创建注册申请失败")
	}

	return nil
}

func defaultVoceChatPasswordLogin(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
	client, err := vocechat.NewClient(config)
	if err != nil {
		return nil, err
	}
	return client.LoginWithPassword(ctx, email, password, "echo-noise")
}

func defaultVoceChatAdminUpdateUser(ctx context.Context, config vocechat.Config, uid int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
	if !config.IsReady() {
		return nil, errors.New("VoceChat 未配置完成")
	}
	client, err := vocechat.NewClient(config)
	if err != nil {
		return nil, err
	}
	tokenManager := vocechat.NewAdminTokenManager(client, config)
	apiKey, err := tokenManager.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	return client.UpdateUser(ctx, apiKey, uid, request)
}

func loadVoceChatSiteConfig() (vocechat.Config, error) {
	if database.DB == nil {
		return vocechat.Config{}, nil
	}

	var siteConfig models.SiteConfig
	if err := database.DB.Table("site_configs").First(&siteConfig).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return vocechat.Config{}, nil
		}
		return vocechat.Config{}, err
	}

	return vocechat.FromSiteConfig(siteConfig), nil
}

func loadVoceChatLoginConfig() (vocechat.Config, bool, error) {
	config, err := loadVoceChatSiteConfig()
	if err != nil {
		return vocechat.Config{}, false, err
	}
	return config, config.Enabled && config.LoginVerificationEnabled, nil
}

func authenticateLocalPassword(username string, user *models.User, plain string) (localLoginResult, error) {
	result := localLoginResult{}
	md5pwd := pkg.MD5Encrypt(plain)
	pw := strings.TrimSpace(user.Password)
	isMD5 := isHexMD5String(pw)
	isBcrypt := strings.HasPrefix(pw, "$2a$") || strings.HasPrefix(pw, "$2b$") || strings.HasPrefix(pw, "$2y$")

	if isMD5 {
		if strings.EqualFold(pw, md5pwd) {
			result.matched = plain
			result.upgradePlain = plain
			result.upgradeAllowed = true
		} else {
			return result, errors.New(models.PasswordIncorrectMessage)
		}
	} else if isBcrypt {
		if bcrypt.CompareHashAndPassword([]byte(pw), []byte(plain)) == nil {
			result.matched = plain
		} else {
			tplain := strings.TrimSpace(plain)
			if tplain != plain && bcrypt.CompareHashAndPassword([]byte(pw), []byte(tplain)) == nil {
				result.matched = tplain
			} else if bcrypt.CompareHashAndPassword([]byte(pw), []byte(md5pwd)) == nil {
				result.matched = plain
			} else {
				tmd5 := pkg.MD5Encrypt(tplain)
				if tplain != plain && bcrypt.CompareHashAndPassword([]byte(pw), []byte(tmd5)) == nil {
					result.matched = tplain
				} else if bcrypt.CompareHashAndPassword([]byte(pw), []byte(strings.ToUpper(md5pwd))) == nil {
					result.matched = plain
				} else {
					override := strings.TrimSpace(os.Getenv("NOISE_ADMIN_PASSWORD"))
					if override != "" && (strings.EqualFold(username, "noise") || user.IsAdmin) && plain == override {
						result.matched = plain
						result.usedOverride = true
					} else {
						return result, errors.New(models.PasswordIncorrectMessage)
					}
				}
			}
		}
	} else {
		if pw == plain {
			result.matched = plain
			result.upgradePlain = plain
			result.upgradeAllowed = true
		} else {
			return result, errors.New(models.PasswordIncorrectMessage)
		}
	}

	if result.matched == "" {
		return result, errors.New(models.PasswordIncorrectMessage)
	}
	result.needUpgrade = isMD5 || !isBcrypt
	return result, nil
}

func applyLocalLoginUpgrade(user *models.User, result localLoginResult) {
	if !result.needUpgrade || result.usedOverride || !result.upgradeAllowed || result.upgradePlain == "" {
		return
	}
	newHash := models.HashPassword(result.upgradePlain)
	if newHash == "" {
		return
	}
	_ = repository.UpdateUserField(user.ID, "password", newHash)
	user.Password = newHash
}

func ensureLoginToken(user *models.User) error {
	if user.Token != "" {
		return nil
	}
	user.Token = models.GenerateToken(32)
	if err := repository.UpdateUserField(user.ID, "token", user.Token); err != nil {
		return fmt.Errorf("生成用户 token 失败: %v", err)
	}
	return nil
}

func isVoceChatBoundOrdinaryUser(user *models.User) bool {
	return user != nil && !user.IsAdmin && strings.TrimSpace(user.VoceChatEmail) != "" && strings.TrimSpace(user.VoceChatUserID) != ""
}

func shouldUseVoceChatLogin(user *models.User, config vocechat.Config, enabled bool) bool {
	return enabled && isVoceChatBoundOrdinaryUser(user)
}

func inactiveVoceChatLocalFallbackError() error {
	return errors.New("已绑定 VoceChat，当前未启用 VoceChat 登录校验，且未开启本地备用登录")
}

func isVoceChatCredentialRejected(err error) bool {
	var apiErr *vocechat.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		return true
	default:
		return false
	}
}

func recordVoceChatLoginHealth(status string, healthErr error) {
	if database.DB == nil {
		return
	}
	var siteConfig models.SiteConfig
	if err := database.DB.Table("site_configs").Select("id").First(&siteConfig).Error; err != nil || siteConfig.ID == 0 {
		return
	}

	errorText := ""
	if healthErr != nil {
		errorText = strings.TrimSpace(healthErr.Error())
	}
	now := time.Now().UTC()
	_ = database.DB.Model(&models.SiteConfig{}).Where("id = ?", siteConfig.ID).Updates(map[string]interface{}{
		"voce_chat_last_health_status":   status,
		"voce_chat_last_health_error":    errorText,
		"voce_chat_last_health_check_at": now,
	}).Error
}

func applyVoceChatLoginResponse(user *models.User, response *vocechat.LoginResponse) {
	if user == nil {
		return
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"voce_chat_sync_status":  models.VoceChatSyncStatusLinked,
		"voce_chat_sync_error":   "",
		"voce_chat_last_sync_at": now,
	}
	user.VoceChatSyncStatus = models.VoceChatSyncStatusLinked
	user.VoceChatSyncError = ""
	user.VoceChatLastSyncAt = &now

	if response != nil {
		if response.User.UID > 0 {
			uid := strconv.FormatInt(response.User.UID, 10)
			if uid != strings.TrimSpace(user.VoceChatUserID) {
				updates["voce_chat_user_id"] = uid
				user.VoceChatUserID = uid
			}
		}
		if email := strings.TrimSpace(response.User.Email); email != "" && email != strings.TrimSpace(user.VoceChatEmail) {
			updates["voce_chat_email"] = email
			user.VoceChatEmail = email
		}
		if name := strings.TrimSpace(response.User.Name); name != "" && name != strings.TrimSpace(user.VoceChatUsername) {
			updates["voce_chat_username"] = name
			user.VoceChatUsername = name
		}
	}

	for field, value := range updates {
		_ = repository.UpdateUserField(user.ID, field, value)
	}
}

func syncPasswordAfterVoceChatLogin(user *models.User, plain string) {
	if user == nil || plain == "" {
		return
	}

	hashed := models.HashPassword(plain)
	if hashed != "" {
		if err := repository.UpdateUserField(user.ID, "password", hashed); err == nil {
			user.Password = hashed
		}
	}
	_ = vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, plain, user.VoceChatEmail, user.VoceChatUserID)
}

func syncPasswordAfterLocalFallbackLogin(user *models.User, result localLoginResult) {
	if !isVoceChatBoundOrdinaryUser(user) || result.usedOverride || result.matched == "" {
		return
	}
	hashed := models.HashPassword(result.matched)
	if hashed != "" {
		if err := repository.UpdateUserField(user.ID, "password", hashed); err == nil {
			user.Password = hashed
		}
	}
	_ = vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(user.ID, user.Username, result.matched, user.VoceChatEmail, user.VoceChatUserID)
}

func updatePlainPasswordUserMetadata(user *models.User) {
	if user == nil || user.ID == 0 {
		return
	}
	store := vocechat.DefaultPlainPasswordStore()
	record, ok, err := store.GetUserPassword(user.ID)
	if err != nil || !ok || !record.HasAnyPassword() {
		return
	}
	_ = store.UpsertUserPasswordMetadata(user.ID, user.Username, user.VoceChatEmail, user.VoceChatUserID)
}

func markVoceChatUserSync(user *models.User, status string, syncErr error, vcUser *vocechat.User, fallbackName string) {
	if user == nil || user.ID == 0 {
		return
	}
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"voce_chat_sync_status":  status,
		"voce_chat_last_sync_at": now,
	}
	user.VoceChatSyncStatus = status
	user.VoceChatLastSyncAt = &now
	if syncErr != nil {
		errorText := strings.TrimSpace(syncErr.Error())
		updates["voce_chat_sync_error"] = errorText
		user.VoceChatSyncError = errorText
	} else {
		updates["voce_chat_sync_error"] = ""
		user.VoceChatSyncError = ""
	}
	if vcUser != nil {
		if vcUser.UID > 0 {
			uid := strconv.FormatInt(vcUser.UID, 10)
			updates["voce_chat_user_id"] = uid
			user.VoceChatUserID = uid
		}
		if email := strings.TrimSpace(vcUser.Email); email != "" {
			updates["voce_chat_email"] = email
			user.VoceChatEmail = email
		}
		if name := strings.TrimSpace(vcUser.Name); name != "" {
			updates["voce_chat_username"] = name
			user.VoceChatUsername = name
		}
	} else if name := strings.TrimSpace(fallbackName); name != "" && syncErr == nil {
		updates["voce_chat_username"] = name
		user.VoceChatUsername = name
	}
	for field, value := range updates {
		_ = repository.UpdateUserField(user.ID, field, value)
	}
	updatePlainPasswordUserMetadata(user)
}

func syncVoceChatBoundUserUpdate(user *models.User, request vocechat.UpdateUserRequest, requireForLoginVerification bool, fallbackName string) error {
	if user == nil || user.IsAdmin || strings.TrimSpace(user.VoceChatUserID) == "" {
		return nil
	}
	config, err := loadVoceChatSiteConfig()
	if err != nil {
		return errors.New(models.DatabaseErrorMessage)
	}
	if !config.Enabled {
		return nil
	}
	required := requireForLoginVerification
	if !config.IsReady() {
		err := errors.New("VoceChat 未配置完成")
		if required {
			return err
		}
		markVoceChatUserSync(user, models.VoceChatSyncStatusFailed, err, nil, "")
		return nil
	}
	uid, err := strconv.ParseInt(strings.TrimSpace(user.VoceChatUserID), 10, 64)
	if err != nil || uid <= 0 {
		err = errors.New("VoceChat 用户ID无效")
		if required {
			return err
		}
		markVoceChatUserSync(user, models.VoceChatSyncStatusFailed, err, nil, "")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	vcUser, err := voceChatAdminUpdateUser(ctx, config, uid, request)
	if err != nil {
		markVoceChatUserSync(user, models.VoceChatSyncStatusFailed, err, nil, "")
		if required {
			return fmt.Errorf("同步 VoceChat 账号失败: %w", err)
		}
		return nil
	}
	markVoceChatUserSync(user, models.VoceChatSyncStatusLinked, nil, vcUser, fallbackName)
	return nil
}

func saveLocalUserPassword(user *models.User, plain string) error {
	hashed := models.HashPassword(plain)
	if hashed == "" {
		return fmt.Errorf("密码加密失败")
	}
	if err := repository.UpdateUserField(user.ID, "password", hashed); err != nil {
		return fmt.Errorf("更新密码失败: %v", err)
	}
	user.Password = hashed
	if isVoceChatBoundOrdinaryUser(user) {
		_ = vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, plain, user.VoceChatEmail, user.VoceChatUserID)
	} else {
		_ = vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(user.ID, user.Username, plain, user.VoceChatEmail, user.VoceChatUserID)
	}
	return nil
}

func authenticateVoceChatPassword(user *models.User, config vocechat.Config, plain string) (bool, error) {
	email := strings.TrimSpace(user.VoceChatEmail)
	if email == "" {
		return false, errors.New("账号尚未绑定 VoceChat，请联系管理员")
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		if config.LocalFallbackEnabled {
			return false, nil
		}
		return false, errors.New("VoceChat 登录校验未配置，请联系管理员")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	response, err := voceChatPasswordLogin(ctx, config, email, plain)
	if err != nil {
		if isVoceChatCredentialRejected(err) {
			recordVoceChatLoginHealth("ok", nil)
			return false, errors.New(models.PasswordIncorrectMessage)
		}
		recordVoceChatLoginHealth("failed", err)
		if config.LocalFallbackEnabled {
			return false, nil
		}
		return false, errors.New("VoceChat 登录校验暂不可用，请稍后再试")
	}

	recordVoceChatLoginHealth("ok", nil)
	applyVoceChatLoginResponse(user, response)
	syncPasswordAfterVoceChatLogin(user, plain)
	return true, nil
}

func Login(userdto dto.LoginDto) (*models.User, error) {
	if userdto.Username == "" || userdto.Password == "" {
		return nil, errors.New(models.UsernameOrPasswordCannotBeEmptyMessage)
	}

	username := strings.TrimSpace(userdto.Username)
	plain := userdto.Password

	user, err := repository.GetUserByUsername(username)
	if err != nil || user == nil {
		return nil, errors.New(models.UserNotFoundMessage)
	}

	voceConfig, voceLoginEnabled, err := loadVoceChatLoginConfig()
	if err != nil {
		return nil, errors.New(models.DatabaseErrorMessage)
	}
	if shouldUseVoceChatLogin(user, voceConfig, voceLoginEnabled) {
		verified, err := authenticateVoceChatPassword(user, voceConfig, plain)
		if err != nil {
			return nil, err
		}
		if !verified {
			result, err := authenticateLocalPassword(username, user, plain)
			if err != nil {
				return nil, err
			}
			applyLocalLoginUpgrade(user, result)
			syncPasswordAfterLocalFallbackLogin(user, result)
		}
	} else {
		if isVoceChatBoundOrdinaryUser(user) && !voceConfig.LocalFallbackEnabled {
			return nil, inactiveVoceChatLocalFallbackError()
		}
		result, err := authenticateLocalPassword(username, user, plain)
		if err != nil {
			return nil, err
		}
		applyLocalLoginUpgrade(user, result)
		syncPasswordAfterLocalFallbackLogin(user, result)
	}

	if err := ensureLoginToken(user); err != nil {
		return nil, err
	}

	return user, nil
}

func GetStatus(currentUserID uint) (models.Status, error) {
	sysuser, err := repository.GetSysAdmin()
	if err != nil {
		return models.Status{}, errors.New(models.UserNotFoundMessage)
	}

	var users []models.UserStatus
	allusers, err := repository.GetAllUsers()
	if err != nil {
		return models.Status{}, errors.New(models.GetAllUsersFailMessage)
	}
	for _, user := range allusers {
		users = append(users, models.UserStatus{
			ID:            user.ID,
			Username:      user.Username,
			IsAdmin:       user.IsAdmin,
			AvatarURL:     strings.TrimSpace(user.AvatarURL),
			VoceChatEmail: strings.TrimSpace(user.VoceChatEmail),
		})
	}

	status := models.Status{}

	var currentUser models.User
	var viewerUserID *uint
	isAdmin := false
	if currentUserID > 0 {
		if err := database.DB.Select("id, is_admin").First(&currentUser, currentUserID).Error; err == nil && currentUser.ID != 0 {
			id := currentUser.ID
			viewerUserID = &id
			isAdmin = currentUser.IsAdmin
		}
	}

	var total int64
	messageQuery := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), viewerUserID, isAdmin)
	if viewerUserID != nil && !isAdmin {
		messageQuery = messageQuery.Where("user_id = ?", *viewerUserID)
	}
	if err := messageQuery.
		Where("content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ?",
			"%#guestbook%", "%#留言%", "%留言板%",
			"%#友链%", "%友情链接%",
			"%#关于%", "%关于本站%").
		Count(&total).Error; err != nil {
		return status, errors.New(models.GetAllMessagesFailMessage)
	}

	var totalComments int64
	if err := database.DB.Model(&models.Comment{}).Where("parent_id IS NULL").Count(&totalComments).Error; err != nil {
		return status, errors.New(models.GetStatusFailMessage)
	}

	var totalReplies int64
	if err := database.DB.Model(&models.Comment{}).Where("parent_id IS NOT NULL").Count(&totalReplies).Error; err != nil {
		return status, errors.New(models.GetStatusFailMessage)
	}

	status.SysAdminID = sysuser.ID
	status.Username = sysuser.Username
	status.Users = users
	status.TotalMessages = int(total)
	status.TotalUsers = len(users)
	status.TotalComments = int(totalComments)
	status.TotalReplies = int(totalReplies)

	if currentUser.ID != 0 {
		receivedComments, receivedReplies, err := countReceivedCommentStats(currentUser.ID)
		if err != nil {
			return status, errors.New(models.GetStatusFailMessage)
		}
		status.ReceivedComments = int(receivedComments)
		status.ReceivedReplies = int(receivedReplies)
	}

	return status, nil
}

func countReceivedCommentStats(userID uint) (int64, int64, error) {
	var receivedComments int64
	if err := database.DB.Model(&models.Comment{}).
		Joins("JOIN messages ON messages.id = comments.message_id").
		Where("messages.user_id = ?", userID).
		Where("comments.parent_id IS NULL").
		Where("(comments.user_id IS NULL OR comments.user_id <> ?)", userID).
		Count(&receivedComments).Error; err != nil {
		return 0, 0, err
	}

	var receivedReplies int64
	if err := database.DB.Model(&models.Comment{}).
		Joins("JOIN comments AS parent_comments ON parent_comments.id = comments.parent_id").
		Where("parent_comments.user_id = ?", userID).
		Where("(comments.user_id IS NULL OR comments.user_id <> ?)", userID).
		Count(&receivedReplies).Error; err != nil {
		return 0, 0, err
	}

	return receivedComments, receivedReplies, nil
}

func GetUserByID(userID uint) (*models.User, error) {
	user, err := repository.GetUserByID(userID)
	if err != nil {
		return nil, errors.New(models.UserNotFoundMessage)
	}
	return user, nil
}

func IsUserAdmin(userID uint) (bool, error) {
	user, err := repository.GetUserByID(userID)
	if err != nil {
		return false, errors.New(models.UserNotFoundMessage)
	}
	return user.IsAdmin, nil
}

func UpdateUser(user *models.User, userdto dto.UserInfoDto) error {
	if user == nil {
		return errors.New("用户信息不能为空")
	}

	updates := make(map[string]interface{})

	// 用户名更新
	if userdto.Username != "" && userdto.Username != user.Username {
		username := strings.TrimSpace(userdto.Username)
		if err := validateRegistrationUsername(username); err != nil {
			return err
		}
		updates["username"] = username
	}

	// 头像地址更新
	if userdto.AvatarURL != "" && userdto.AvatarURL != user.AvatarURL {
		updates["avatar_url"] = userdto.AvatarURL
	}
	if strings.TrimSpace(userdto.Description) != "" && strings.TrimSpace(userdto.Description) != strings.TrimSpace(user.Description) {
		updates["description"] = strings.TrimSpace(userdto.Description)
	}

	if len(updates) == 0 {
		return nil
	}

	// 基本校验：如果包含用户名，不能为空
	if v, ok := updates["username"]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) == "" {
			return errors.New(models.UsernameCannotBeEmptyMessage)
		}
	}

	// 仅更新请求中实际变化的字段，避免整对象保存时覆盖密码等敏感字段。
	for field, value := range updates {
		if err := repository.UpdateUserField(user.ID, field, value); err != nil {
			return errors.New(err.Error())
		}
	}

	// 同步到本地结构体
	if v, ok := updates["username"]; ok && v != nil {
		user.Username = v.(string)
	}
	if v, ok := updates["avatar_url"]; ok && v != nil {
		user.AvatarURL = v.(string)
	}
	if v, ok := updates["description"]; ok && v != nil {
		user.Description = v.(string)
	}
	updatePlainPasswordUserMetadata(user)

	if _, ok := updates["username"]; ok {
		name := user.Username
		_ = syncVoceChatBoundUserUpdate(user, vocechat.UpdateUserRequest{Name: &name}, false, name)
	}

	return nil
}

func ChangePassword(user *models.User, userdto dto.UserInfoDto) error {
	if user == nil {
		return errors.New("用户信息不能为空")
	}

	newPassword := strings.TrimSpace(userdto.Password)
	if newPassword == "" {
		return errors.New(models.PasswordCannotBeEmptyMessage)
	}

	// 如果新密码与旧密码一致，则拒绝
	if passwordMatchesStored(user.Password, newPassword) {
		return errors.New(models.PasswordCannotBeSameAsBeforeMessage)
	}

	if err := syncVoceChatBoundUserUpdate(user, vocechat.UpdateUserRequest{Password: &newPassword}, true, ""); err != nil {
		return err
	}
	return saveLocalUserPassword(user, newPassword)
}

func verifyVoceChatPasswordForPasswordChange(user *models.User, config vocechat.Config, plain string) error {
	if !config.Enabled || !config.IsReady() {
		return errors.New("VoceChat 登录校验暂不可用，请稍后再试")
	}
	response, err := voceChatPasswordLogin(context.Background(), config, strings.TrimSpace(user.VoceChatEmail), plain)
	if err == nil {
		recordVoceChatLoginHealth("ok", nil)
		applyVoceChatLoginResponse(user, response)
		return nil
	}

	if isVoceChatCredentialRejected(err) {
		recordVoceChatLoginHealth("ok", nil)
		return errors.New(models.PasswordIncorrectMessage)
	}

	recordVoceChatLoginHealth("failed", err)
	return errors.New("VoceChat 登录校验暂不可用，请稍后再试")
}

func verifyPasswordChangeOldPassword(user *models.User, old string) error {
	config, _, err := loadVoceChatLoginConfig()
	if err != nil {
		return errors.New(models.DatabaseErrorMessage)
	}
	if isVoceChatBoundOrdinaryUser(user) {
		return verifyVoceChatPasswordForPasswordChange(user, config, old)
	}
	if !passwordMatchesStored(user.Password, old) {
		return errors.New(models.PasswordIncorrectMessage)
	}
	return nil
}

// ChangePasswordWithOld 验证旧密码后更新为新密码（兼容历史明文/MD5/bcrypt）
func ChangePasswordWithOld(user *models.User, old string, new string) error {
	if user == nil {
		return errors.New("用户信息不能为空")
	}
	old = strings.TrimSpace(old)
	new = strings.TrimSpace(new)
	if new == "" {
		return errors.New(models.PasswordCannotBeEmptyMessage)
	}

	if err := verifyPasswordChangeOldPassword(user, old); err != nil {
		return err
	}

	// 新密码不得与旧密码一致
	if old == new {
		return errors.New(models.PasswordCannotBeSameAsBeforeMessage)
	}
	if passwordMatchesStored(user.Password, new) {
		return errors.New(models.PasswordCannotBeSameAsBeforeMessage)
	}

	if err := syncVoceChatBoundUserUpdate(user, vocechat.UpdateUserRequest{Password: &new}, true, ""); err != nil {
		return err
	}
	return saveLocalUserPassword(user, new)
}

func UpdateUserAdmin(userID uint, currentUserID uint) error {
	user, err := repository.GetUserByID(userID)
	if err != nil {
		return err
	}
	// 不允许取消当前登录用户的管理员身份
	if userID == currentUserID && user.IsAdmin {
		return fmt.Errorf("不允许取消当前登录用户的管理员身份")
	}
	// 至少保留一位管理员
	if user.IsAdmin {
		count, err := repository.CountAdmins()
		if err != nil {
			return err
		}
		if count <= 1 {
			return fmt.Errorf("系统至少保留一位管理员")
		}
	}
	user.IsAdmin = !user.IsAdmin
	if err := repository.UpdateUser(user); err != nil {
		return err
	}
	return nil
}

func GetUserByUsername(username string) (*models.User, error) {
	user, err := repository.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	return user, nil
}
