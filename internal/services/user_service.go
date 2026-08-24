package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/runtimepolicy"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"github.com/rcy1314/echo-noise/pkg"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const maxPendingRegistrationApplications = 5

type RegisterResult struct {
	ApplicationID string `json:"application_id"`
	Status        string `json:"status"`
	AutoApproved  bool   `json:"auto_approved"`
	LocalUserID   *uint  `json:"local_user_id,omitempty"`
}

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

type passwordChangeLock struct {
	mu   sync.Mutex
	refs int
}

var userPasswordChangeLocks = struct {
	sync.Mutex
	locks map[uint]*passwordChangeLock
}{locks: make(map[uint]*passwordChangeLock)}

type managedPasswordSnapshot struct {
	user               models.User
	passwordRecord     vocechat.PlainPasswordRecord
	passwordRecordSeen bool
}

type passwordUpdateFailure struct {
	incomplete bool
	rolledBack bool
	cause      error
}

type accountStateSaveFailure struct {
	cause error
}

func (e *passwordUpdateFailure) Error() string {
	if e == nil || e.cause == nil {
		return "password update failed"
	}
	return "password update failed"
}

func (e *passwordUpdateFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *accountStateSaveFailure) Error() string {
	return "账号信息保存失败，请稍后重新登录。若问题持续，请联系1号管理员。"
}

func (e *accountStateSaveFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func PasswordChangePublicFailureMessage(err error, resetByAdministrator bool) string {
	var updateFailure *passwordUpdateFailure
	if errors.As(err, &updateFailure) {
		if updateFailure.incomplete {
			if resetByAdministrator {
				return "密码保存未完成，请重新为该用户设置密码。"
			}
			return "密码保存未完成，请重新设置密码。若仍无法登录，请联系1号管理员。"
		}
		if updateFailure.rolledBack {
			if resetByAdministrator {
				return "密码重置失败，请稍后重试。用户原密码未改变。"
			}
			return "密码修改失败，请稍后重试。原密码仍可使用。"
		}
	}
	if err != nil && (err.Error() == models.PasswordIncorrectMessage || err.Error() == models.PasswordCannotBeEmptyMessage || err.Error() == models.PasswordCannotBeSameAsBeforeMessage) {
		return err.Error()
	}
	if resetByAdministrator {
		return "密码重置失败，请稍后重试。"
	}
	return "密码修改失败，请稍后重试。"
}

func lockUserPasswordChange(userID uint) func() {
	userPasswordChangeLocks.Lock()
	lock := userPasswordChangeLocks.locks[userID]
	if lock == nil {
		lock = &passwordChangeLock{}
		userPasswordChangeLocks.locks[userID] = lock
	}
	lock.refs++
	userPasswordChangeLocks.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		userPasswordChangeLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(userPasswordChangeLocks.locks, userID)
		}
		userPasswordChangeLocks.Unlock()
	}
}

func withUserPasswordMutation(userID uint, operation func(*models.User) error) (*models.User, error) {
	if userID == 0 || operation == nil {
		return nil, errors.New("invalid password mutation request")
	}

	unlock := lockUserPasswordChange(userID)
	defer unlock()

	user, err := repository.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	if err := operation(user); err != nil {
		return nil, err
	}
	return user, nil
}

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
	_, err := RegisterWithResult(userdto)
	return err
}

func RegisterWithResult(userdto dto.RegisterDto) (RegisterResult, error) {
	username := strings.TrimSpace(userdto.Username)
	if username == "" || userdto.Password == "" {
		return RegisterResult{}, errors.New(models.UsernameOrPasswordCannotBeEmptyMessage)
	}
	if err := validateRegistrationUsername(username); err != nil {
		return RegisterResult{}, err
	}

	exists, err := existingUser(username)
	if err != nil {
		return RegisterResult{}, errors.New(models.DatabaseErrorMessage)
	}
	if exists {
		return RegisterResult{}, errors.New(models.UsernameAlreadyExistsMessage)
	}

	hasPending, err := repository.HasPendingRegistrationApplication(username)
	if err != nil {
		return RegisterResult{}, errors.New(models.DatabaseErrorMessage)
	}
	if hasPending {
		return RegisterResult{}, errors.New("该用户名正在审核中")
	}

	caseFoldReserved, err := usernameCaseFoldReserved(username)
	if err != nil {
		return RegisterResult{}, errors.New(models.DatabaseErrorMessage)
	}
	if caseFoldReserved {
		return RegisterResult{}, errors.New(models.UsernameAlreadyExistsMessage)
	}

	pendingCount, err := repository.CountPendingRegistrationApplications()
	if err != nil {
		return RegisterResult{}, errors.New(models.DatabaseErrorMessage)
	}
	if pendingCount >= maxPendingRegistrationApplications {
		return RegisterResult{}, errors.New("当前待审核申请已满，请稍后再试")
	}

	hashed := models.HashPassword(userdto.Password)
	if hashed == "" {
		return RegisterResult{}, errors.New("密码加密失败")
	}
	policy, voceConfig, err := loadRuntimePolicyAndVoceChatConfig()
	if err != nil {
		return RegisterResult{}, errors.New("创建注册申请失败")
	}
	application := models.RegistrationApplication{
		Username:           username,
		PasswordHash:       hashed,
		Status:             models.RegistrationApplicationStatusPending,
		VoceChatSyncStatus: models.VoceChatSyncStatusUnbound,
	}
	if policy.ConfiguredMode == runtimepolicy.ModeVoceChat {
		application.VoceChatSyncStatus = models.VoceChatSyncStatusPending
	}
	if err := repository.CreateRegistrationApplicationWithPermanentNumber(&application, voceConfig.EmailDomain); err != nil {
		return RegisterResult{}, errors.New("创建注册申请失败")
	}

	plainStore := vocechat.DefaultPlainPasswordStore()
	if policy.ConfiguredMode == runtimepolicy.ModeLocal {
		err = plainStore.UpsertApplicationLocalFallbackPassword(application.ApplicationID, username, userdto.Password, application.VoceChatCandidateEmail)
	} else {
		err = plainStore.UpsertApplicationVoceChatPassword(application.ApplicationID, username, userdto.Password, application.VoceChatCandidateEmail, "")
	}
	if err != nil {
		_ = repository.UpdateRegistrationApplicationFields(application.ID, map[string]interface{}{
			"status":                models.RegistrationApplicationStatusRejected,
			"voce_chat_sync_status": models.VoceChatSyncStatusFailed,
			"voce_chat_sync_error":  "注册密码状态保存失败",
		})
		return RegisterResult{}, errors.New("创建注册申请失败")
	}

	if policy.UseVoceChatRegistration {
		provision := registrationVoceChatProvision(application.ApplicationID, username, userdto.Password)
		application.VoceChatSyncStatus = strings.TrimSpace(provision.SyncStatus)
		if application.VoceChatSyncStatus == "" {
			application.VoceChatSyncStatus = models.VoceChatSyncStatusPending
		}
		application.VoceChatSyncError = strings.TrimSpace(provision.SyncError)
		if application.VoceChatSyncStatus == models.VoceChatSyncStatusCreated && strings.TrimSpace(provision.UserID) != "" {
			application.VoceChatUserID = strings.TrimSpace(provision.UserID)
			application.VoceChatEmail = strings.TrimSpace(provision.Email)
			if application.VoceChatEmail == "" {
				application.VoceChatEmail = application.VoceChatCandidateEmail
			}
			if err := plainStore.UpsertApplicationVoceChatPassword(application.ApplicationID, username, userdto.Password, application.VoceChatEmail, application.VoceChatUserID); err != nil {
				application.VoceChatSyncStatus = models.VoceChatSyncStatusFailed
				application.VoceChatSyncError = "VoceChat 账户密码状态保存失败"
			}
		}
	} else if policy.ConfiguredMode == runtimepolicy.ModeVoceChat {
		application.VoceChatSyncStatus = models.VoceChatSyncStatusPending
		application.VoceChatSyncError = "VoceChat 暂不可用，等待恢复后处理"
	}
	if err := repository.UpdateRegistrationApplication(&application); err != nil {
		return RegisterResult{ApplicationID: application.ApplicationID, Status: application.Status}, errors.New("更新注册申请失败")
	}

	result := RegisterResult{ApplicationID: application.ApplicationID, Status: application.Status}
	if autoApproveRegistrationEnabled(policy) {
		created, err := ApproveRegistrationApplication(application.ID, 0, "系统自动通过审核")
		if err != nil {
			if !isRegistrationApprovalDeferred(err) {
				return result, err
			}
			return result, nil
		}
		localUserID := created.ID
		result.Status = models.RegistrationApplicationStatusApproved
		result.AutoApproved = true
		result.LocalUserID = &localUserID
	}

	return result, nil
}

func autoApproveRegistrationEnabled(policy runtimepolicy.Policy) bool {
	if !policy.UseVoceChatRegistration {
		return false
	}
	if database.DB == nil {
		return false
	}
	var setting models.Setting
	if err := database.DB.Table("settings").First(&setting).Error; err != nil {
		return false
	}
	return setting.AutoApproveRegistration
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

func loadRuntimePolicyAndVoceChatConfig() (runtimepolicy.Policy, vocechat.Config, error) {
	if database.DB == nil {
		policy := runtimepolicy.Resolve(models.SiteConfig{})
		return policy, vocechat.Config{}, nil
	}

	var siteConfig models.SiteConfig
	if err := database.DB.Table("site_configs").First(&siteConfig).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			policy := runtimepolicy.Resolve(models.SiteConfig{})
			return policy, vocechat.Config{}, nil
		}
		return runtimepolicy.Policy{}, vocechat.Config{}, err
	}

	return runtimepolicy.Resolve(siteConfig), vocechat.FromSiteConfig(siteConfig), nil
}

func loadVoceChatSiteConfig() (vocechat.Config, error) {
	_, config, err := loadRuntimePolicyAndVoceChatConfig()
	return config, err
}

func loadVoceChatLoginConfig() (runtimepolicy.Policy, vocechat.Config, bool, error) {
	policy, config, err := loadRuntimePolicyAndVoceChatConfig()
	if err != nil {
		return runtimepolicy.Policy{}, vocechat.Config{}, false, err
	}
	loginEnabled := policy.ConfiguredMode == runtimepolicy.ModeVoceChat
	if policy.LegacyConfiguration {
		loginEnabled = config.Enabled && config.LoginVerificationEnabled
	}
	return policy, config, loginEnabled, nil
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

func applyLocalLoginUpgrade(user *models.User, result localLoginResult) error {
	if !result.needUpgrade || result.usedOverride || !result.upgradeAllowed || result.upgradePlain == "" {
		return nil
	}
	newHash := models.HashPassword(result.upgradePlain)
	if newHash == "" {
		return errors.New("local login password upgrade hashing failed")
	}
	if err := repository.UpdateUserField(user.ID, "password", newHash); err != nil {
		return err
	}
	user.Password = newHash
	return nil
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

func issueLoginState(user *models.User) error {
	if err := ensureLoginToken(user); err != nil {
		return err
	}
	issuedAt := time.Now()
	if err := database.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("login_issued_at", issuedAt).Error; err != nil {
		return err
	}
	user.LoginIssuedAt = &issuedAt
	return nil
}

func isVoceChatBoundNonPrimaryUser(user *models.User) bool {
	return user != nil && !(user.ID == models.PrimaryAdminUserID && user.IsAdmin) && strings.TrimSpace(user.VoceChatEmail) != "" && strings.TrimSpace(user.VoceChatUserID) != ""
}

func shouldUseVoceChatLogin(user *models.User, config vocechat.Config, enabled bool) bool {
	return enabled && isVoceChatBoundNonPrimaryUser(user)
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

// isVoceChatAccountCredentialInvalid is deliberately narrower than a generic
// VoceChat request failure. A temporary outage must not be reported as a bad
// account. A 404 only counts when VoceChat explicitly identifies the account
// or credential in its response body.
func isVoceChatAccountCredentialInvalid(err error) bool {
	if isVoceChatCredentialRejected(err) {
		return true
	}
	var apiErr *vocechat.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		return false
	}
	body := strings.ToLower(strings.TrimSpace(apiErr.Body))
	for _, marker := range []string{"account", "user", "credential", "email", "账号", "账户", "用户", "邮箱"} {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

func isVoceChatSiteTransientFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *vocechat.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= http.StatusInternalServerError
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
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

func applyVoceChatLoginResponse(user *models.User, response *vocechat.LoginResponse) error {
	if user == nil {
		return errors.New("missing VoceChat login user")
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"voce_chat_sync_status":  models.VoceChatSyncStatusLinked,
		"voce_chat_sync_error":   "",
		"voce_chat_last_sync_at": now,
	}
	next := *user
	next.VoceChatSyncStatus = models.VoceChatSyncStatusLinked
	next.VoceChatSyncError = ""
	next.VoceChatLastSyncAt = &now

	if response != nil {
		if response.User.UID > 0 {
			uid := strconv.FormatInt(response.User.UID, 10)
			if uid != strings.TrimSpace(next.VoceChatUserID) {
				updates["voce_chat_user_id"] = uid
				next.VoceChatUserID = uid
			}
		}
		if email := strings.TrimSpace(response.User.Email); email != "" && email != strings.TrimSpace(next.VoceChatEmail) {
			updates["voce_chat_email"] = email
			next.VoceChatEmail = email
		}
		if name := strings.TrimSpace(response.User.Name); name != "" && name != strings.TrimSpace(next.VoceChatUsername) {
			updates["voce_chat_username"] = name
			next.VoceChatUsername = name
		}
	}

	if err := database.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		return err
	}
	repository.ClearUserCache()
	*user = next
	return nil
}

func syncPasswordAfterVoceChatLogin(user *models.User, plain string, snapshot managedPasswordSnapshot) error {
	if user == nil || strings.TrimSpace(plain) == "" {
		return &accountStateSaveFailure{cause: errors.New("missing login password state")}
	}
	if err := saveLocalUserPassword(user, plain); err != nil {
		return rollbackVoceChatLoginState(user, snapshot, err)
	}
	return nil
}

func syncPasswordAfterLocalFallbackLogin(user *models.User, result localLoginResult) error {
	if !isVoceChatBoundNonPrimaryUser(user) || result.usedOverride || result.matched == "" {
		return nil
	}
	return vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(user.ID, user.Username, result.matched, user.VoceChatEmail, user.VoceChatUserID)
}

func maintainLocalFallbackLoginState(user *models.User, result localLoginResult) {
	if err := applyLocalLoginUpgrade(user, result); err != nil {
		log.Printf("local login maintenance failed: user_id=%d stage=password_upgrade error_type=%T", user.ID, err)
	}
	if err := syncPasswordAfterLocalFallbackLogin(user, result); err != nil {
		log.Printf("local login maintenance failed: user_id=%d stage=fallback_record error_type=%T", user.ID, err)
	}
}

func updatePlainPasswordUserMetadata(user *models.User) error {
	if user == nil || user.ID == 0 {
		return nil
	}
	store := vocechat.DefaultPlainPasswordStore()
	record, ok, err := store.GetUserPassword(user.ID)
	if err != nil {
		return err
	}
	if !ok || !record.HasAnyPassword() {
		return nil
	}
	return store.UpsertUserPasswordMetadata(user.ID, user.Username, user.VoceChatEmail, user.VoceChatUserID)
}

func markVoceChatUserSync(user *models.User, status string, syncErr error, vcUser *vocechat.User, fallbackName string) error {
	if user == nil || user.ID == 0 {
		return nil
	}
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"voce_chat_sync_status":  status,
		"voce_chat_last_sync_at": now,
	}
	next := *user
	next.VoceChatSyncStatus = status
	next.VoceChatLastSyncAt = &now
	if syncErr != nil {
		errorText := strings.TrimSpace(syncErr.Error())
		updates["voce_chat_sync_error"] = errorText
		next.VoceChatSyncError = errorText
	} else {
		updates["voce_chat_sync_error"] = ""
		next.VoceChatSyncError = ""
	}
	if vcUser != nil {
		if vcUser.UID > 0 {
			uid := strconv.FormatInt(vcUser.UID, 10)
			updates["voce_chat_user_id"] = uid
			next.VoceChatUserID = uid
		}
		if email := strings.TrimSpace(vcUser.Email); email != "" {
			updates["voce_chat_email"] = email
			next.VoceChatEmail = email
		}
		if name := strings.TrimSpace(vcUser.Name); name != "" {
			updates["voce_chat_username"] = name
			next.VoceChatUsername = name
		}
	} else if name := strings.TrimSpace(fallbackName); name != "" && syncErr == nil {
		updates["voce_chat_username"] = name
		next.VoceChatUsername = name
	}
	if err := database.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		return err
	}
	repository.ClearUserCache()
	*user = next
	return updatePlainPasswordUserMetadata(user)
}

func updateVoceChatBoundUserRemote(user *models.User, request vocechat.UpdateUserRequest, required bool) (*vocechat.User, error) {
	if user == nil || (user.ID == models.PrimaryAdminUserID && user.IsAdmin) {
		return nil, nil
	}
	if !isVoceChatBoundNonPrimaryUser(user) {
		return nil, nil
	}
	config, err := loadVoceChatSiteConfig()
	if err != nil {
		return nil, errors.New(models.DatabaseErrorMessage)
	}
	if !config.Enabled {
		if required {
			return nil, errors.New("VoceChat 未配置完成")
		}
		return nil, nil
	}
	if !config.IsReady() {
		return nil, errors.New("VoceChat 未配置完成")
	}
	uid, err := strconv.ParseInt(strings.TrimSpace(user.VoceChatUserID), 10, 64)
	if err != nil || uid <= 0 {
		return nil, errors.New("VoceChat 用户ID无效")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	vcUser, err := voceChatAdminUpdateUser(ctx, config, uid, request)
	if err != nil {
		return nil, fmt.Errorf("同步 VoceChat 账号失败: %w", err)
	}
	return vcUser, nil
}

func syncVoceChatBoundUserUpdate(user *models.User, request vocechat.UpdateUserRequest, requireForLoginVerification bool, fallbackName string) error {
	vcUser, err := updateVoceChatBoundUserRemote(user, request, requireForLoginVerification)
	if err != nil {
		markErr := markVoceChatUserSync(user, models.VoceChatSyncStatusFailed, err, nil, "")
		if requireForLoginVerification {
			return errors.Join(err, markErr)
		}
		if markErr != nil {
			log.Printf("VoceChat sync status persistence failed: user_id=%d stage=remote_failure error_type=%T", user.ID, markErr)
		}
		return nil
	}
	if vcUser == nil {
		return nil
	}
	if err := markVoceChatUserSync(user, models.VoceChatSyncStatusLinked, nil, vcUser, fallbackName); err != nil {
		if requireForLoginVerification {
			return err
		}
		log.Printf("VoceChat sync status persistence failed: user_id=%d stage=remote_success error_type=%T", user.ID, err)
	}
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
	if isVoceChatBoundNonPrimaryUser(user) {
		if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, plain, user.VoceChatEmail, user.VoceChatUserID); err != nil {
			return fmt.Errorf("更新 VoceChat 凭据存储失败: %w", err)
		}
	} else {
		if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(user.ID, user.Username, plain, user.VoceChatEmail, user.VoceChatUserID); err != nil {
			return fmt.Errorf("更新本地凭据存储失败: %w", err)
		}
	}
	return nil
}

func readManagedPasswordSnapshot(userID uint) (managedPasswordSnapshot, error) {
	if userID == 0 {
		return managedPasswordSnapshot{}, errors.New("用户信息不能为空")
	}

	var current models.User
	if err := database.DB.First(&current, userID).Error; err != nil {
		return managedPasswordSnapshot{}, err
	}
	record, found, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(userID)
	if err != nil {
		return managedPasswordSnapshot{}, err
	}
	if !found {
		record.UserID = userID
	}
	return managedPasswordSnapshot{user: current, passwordRecord: record, passwordRecordSeen: found}, nil
}

func restoreManagedPasswordLocalState(user *models.User, snapshot managedPasswordSnapshot) error {
	if err := repository.UpdateUser(&snapshot.user); err != nil {
		return err
	}
	if user != nil {
		*user = snapshot.user
	}
	return nil
}

func sameOptionalTime(left *time.Time, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func restoreManagedPasswordSnapshot(user *models.User, snapshot managedPasswordSnapshot) error {
	var restoreErrors []error
	if err := restoreManagedPasswordLocalState(user, snapshot); err != nil {
		restoreErrors = append(restoreErrors, fmt.Errorf("restore local state: %w", err))
	}
	if err := vocechat.DefaultPlainPasswordStore().RestoreUserPasswordSnapshot(snapshot.passwordRecord, snapshot.passwordRecordSeen); err != nil {
		restoreErrors = append(restoreErrors, fmt.Errorf("restore password record: %w", err))
	}
	if err := restoreManagedPasswordLocalState(user, snapshot); err != nil {
		restoreErrors = append(restoreErrors, fmt.Errorf("confirm local state: %w", err))
	}
	if err := managedPasswordSnapshotMatches(snapshot.user.ID, snapshot); err != nil {
		restoreErrors = append(restoreErrors, fmt.Errorf("verify restored state: %w", err))
	}
	return errors.Join(restoreErrors...)
}

func rollbackVoceChatLoginState(user *models.User, snapshot managedPasswordSnapshot, cause error) error {
	restoreErr := restoreManagedPasswordSnapshot(user, snapshot)
	if restoreErr == nil {
		return &accountStateSaveFailure{cause: cause}
	}
	statusErr := markPasswordUpdateIncomplete(user)
	return &accountStateSaveFailure{cause: errors.Join(cause, restoreErr, statusErr)}
}

func managedPasswordSnapshotMatches(userID uint, snapshot managedPasswordSnapshot) error {
	var current models.User
	if err := database.DB.First(&current, userID).Error; err != nil {
		return err
	}
	if current.Password != snapshot.user.Password ||
		current.Token != snapshot.user.Token ||
		!sameOptionalTime(current.LoginIssuedAt, snapshot.user.LoginIssuedAt) ||
		current.VoceChatUserID != snapshot.user.VoceChatUserID ||
		current.VoceChatEmail != snapshot.user.VoceChatEmail ||
		current.VoceChatUsername != snapshot.user.VoceChatUsername ||
		current.VoceChatSyncStatus != snapshot.user.VoceChatSyncStatus ||
		current.VoceChatSyncError != snapshot.user.VoceChatSyncError ||
		!sameOptionalTime(current.VoceChatLastSyncAt, snapshot.user.VoceChatLastSyncAt) {
		return errors.New("local password state differs from snapshot")
	}
	record, found, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(userID)
	if err != nil {
		return err
	}
	if found != snapshot.passwordRecordSeen {
		return errors.New("password record presence differs from snapshot")
	}
	if found && (record.VoceChatPassword != snapshot.passwordRecord.VoceChatPassword ||
		record.LocalFallbackPassword != snapshot.passwordRecord.LocalFallbackPassword ||
		record.Username != snapshot.passwordRecord.Username ||
		record.VoceChatEmail != snapshot.passwordRecord.VoceChatEmail ||
		record.VoceChatUserID != snapshot.passwordRecord.VoceChatUserID) {
		return errors.New("password record differs from snapshot")
	}
	return nil
}

func managedPasswordStateMatches(user *models.User, plain string) error {
	if user == nil || user.ID == 0 {
		return errors.New("missing managed login user")
	}
	var current models.User
	if err := database.DB.First(&current, user.ID).Error; err != nil {
		return err
	}
	if current.Password != user.Password || current.Token != user.Token ||
		!sameOptionalTime(current.LoginIssuedAt, user.LoginIssuedAt) ||
		current.VoceChatUserID != user.VoceChatUserID ||
		current.VoceChatEmail != user.VoceChatEmail ||
		current.VoceChatUsername != user.VoceChatUsername ||
		current.VoceChatSyncStatus != models.VoceChatSyncStatusLinked ||
		current.VoceChatSyncError != "" ||
		!sameOptionalTime(current.VoceChatLastSyncAt, user.VoceChatLastSyncAt) {
		return errors.New("managed login user state was not fully persisted")
	}
	record, found, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(user.ID)
	if err != nil {
		return err
	}
	if !found || record.VoceChatPasswordValue() != plain || record.Username != user.Username ||
		record.VoceChatEmail != user.VoceChatEmail || record.VoceChatUserID != user.VoceChatUserID {
		return errors.New("managed login password state was not fully persisted")
	}
	return nil
}

func compensateManagedPasswordChange(user *models.User, snapshot managedPasswordSnapshot, previousRemotePassword string) error {
	var compensationErrors []error
	if err := restoreManagedPasswordLocalState(user, snapshot); err != nil {
		compensationErrors = append(compensationErrors, fmt.Errorf("restore local state: %w", err))
	}
	if _, err := updateVoceChatBoundUserRemote(user, vocechat.UpdateUserRequest{Password: &previousRemotePassword}, true); err != nil {
		compensationErrors = append(compensationErrors, fmt.Errorf("restore remote state: %w", err))
	}
	if err := vocechat.DefaultPlainPasswordStore().RestoreUserPasswordSnapshot(snapshot.passwordRecord, snapshot.passwordRecordSeen); err != nil {
		compensationErrors = append(compensationErrors, fmt.Errorf("restore password record: %w", err))
	}
	if err := restoreManagedPasswordLocalState(user, snapshot); err != nil {
		compensationErrors = append(compensationErrors, fmt.Errorf("confirm local state: %w", err))
	}
	if err := managedPasswordSnapshotMatches(snapshot.user.ID, snapshot); err != nil {
		compensationErrors = append(compensationErrors, fmt.Errorf("verify restored state: %w", err))
	}
	return errors.Join(compensationErrors...)
}

func failManagedPasswordChange(user *models.User, snapshot managedPasswordSnapshot, previousRemotePassword string, cause error) error {
	compensationErr := compensateManagedPasswordChange(user, snapshot, previousRemotePassword)
	if compensationErr != nil {
		statusErr := markPasswordUpdateIncomplete(user)
		return &passwordUpdateFailure{incomplete: true, cause: errors.Join(cause, compensationErr, statusErr)}
	}
	return &passwordUpdateFailure{rolledBack: true, cause: cause}
}

func markPasswordUpdateIncomplete(user *models.User) error {
	if user == nil || user.ID == 0 {
		return errors.New("用户信息不能为空")
	}
	if err := database.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"voce_chat_sync_status": models.VoceChatSyncStatusConflicted,
		"voce_chat_sync_error":  "password_update_incomplete",
	}).Error; err != nil {
		return err
	}
	repository.ClearUserCache()
	user.VoceChatSyncStatus = models.VoceChatSyncStatusConflicted
	user.VoceChatSyncError = "password_update_incomplete"
	return CreatePasswordUpdateIncompleteAlertOnce(user.ID)
}

func changeManagedVoceChatPassword(user *models.User, newPassword, previousRemotePassword string) error {
	snapshot, err := readManagedPasswordSnapshot(user.ID)
	if err != nil {
		return &passwordUpdateFailure{cause: err}
	}
	if strings.TrimSpace(previousRemotePassword) == "" {
		previousRemotePassword = snapshot.passwordRecord.VoceChatPasswordValue()
	}
	if strings.TrimSpace(previousRemotePassword) == "" {
		return &passwordUpdateFailure{cause: errors.New("missing recoverable remote password state")}
	}

	vcUser, err := updateVoceChatBoundUserRemote(user, vocechat.UpdateUserRequest{Password: &newPassword}, true)
	if err != nil {
		markErr := markVoceChatUserSync(user, models.VoceChatSyncStatusFailed, err, nil, "")
		return errors.Join(err, markErr)
	}
	if err := markVoceChatUserSync(user, models.VoceChatSyncStatusLinked, nil, vcUser, ""); err != nil {
		return failManagedPasswordChange(user, snapshot, previousRemotePassword, err)
	}
	if err := saveLocalUserPassword(user, newPassword); err != nil {
		return failManagedPasswordChange(user, snapshot, previousRemotePassword, err)
	}
	if err := managedPasswordStateMatches(user, newPassword); err != nil {
		return failManagedPasswordChange(user, snapshot, previousRemotePassword, err)
	}
	reconcileResolvedPasswordAlertsBestEffort(user.ID, "password_change")
	return nil
}

func changeBoundUserPasswordForRuntimePolicy(user *models.User, newPassword, previousRemotePassword string) error {
	policy, _, _, err := loadVoceChatLoginConfig()
	if err != nil {
		return errors.New(models.DatabaseErrorMessage)
	}
	if policy.LegacyConfiguration {
		return changeManagedVoceChatPassword(user, newPassword, previousRemotePassword)
	}
	switch policy.RuntimeState {
	case runtimepolicy.StateLocal:
		snapshot, err := readManagedPasswordSnapshot(user.ID)
		if err != nil {
			return &passwordUpdateFailure{cause: err}
		}
		rollback := func(cause error) error {
			if restoreErr := restoreManagedPasswordSnapshot(user, snapshot); restoreErr != nil {
				statusErr := markPasswordUpdateIncomplete(user)
				return &passwordUpdateFailure{incomplete: true, cause: errors.Join(cause, restoreErr, statusErr)}
			}
			return &passwordUpdateFailure{rolledBack: true, cause: cause}
		}
		hashed := models.HashPassword(newPassword)
		if hashed == "" {
			return &passwordUpdateFailure{cause: errors.New("password hashing failed")}
		}
		if err := repository.UpdateUserField(user.ID, "password", hashed); err != nil {
			return rollback(err)
		}
		user.Password = hashed
		if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(user.ID, user.Username, newPassword, user.VoceChatEmail, user.VoceChatUserID); err != nil {
			return rollback(err)
		}
		if err := database.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
			"voce_chat_sync_status": models.VoceChatSyncStatusPasswordSyncRequired,
			"voce_chat_sync_error":  "",
		}).Error; err != nil {
			return rollback(err)
		}
		repository.ClearUserCache()
		user.VoceChatSyncStatus = models.VoceChatSyncStatusPasswordSyncRequired
		user.VoceChatSyncError = ""
		return nil
	case runtimepolicy.StateVoceChatDegraded:
		return errors.New("VoceChat 暂不可用，绑定账户暂时不能修改密码，请稍后重试")
	default:
		return changeManagedVoceChatPassword(user, newPassword, previousRemotePassword)
	}
}

func authenticateVoceChatPassword(user *models.User, policy runtimepolicy.Policy, config vocechat.Config, plain string) (bool, error) {
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
	snapshot, err := readManagedPasswordSnapshot(user.ID)
	if err != nil {
		return false, &accountStateSaveFailure{cause: err}
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
		fallbackAllowed := policy.ConfiguredMode == runtimepolicy.ModeVoceChat
		if policy.LegacyConfiguration {
			fallbackAllowed = config.LocalFallbackEnabled
		}
		if fallbackAllowed {
			return false, nil
		}
		return false, errors.New("VoceChat 登录校验暂不可用，请稍后再试")
	}

	recordVoceChatLoginHealth("ok", nil)
	if err := applyVoceChatLoginResponse(user, response); err != nil {
		return false, rollbackVoceChatLoginState(user, snapshot, err)
	}
	if err := syncPasswordAfterVoceChatLogin(user, plain, snapshot); err != nil {
		return false, err
	}
	if err := issueLoginState(user); err != nil {
		return false, rollbackVoceChatLoginState(user, snapshot, err)
	}
	if err := managedPasswordStateMatches(user, plain); err != nil {
		return false, rollbackVoceChatLoginState(user, snapshot, err)
	}
	reconcileResolvedPasswordAlertsBestEffort(user.ID, "vocechat_login")
	return true, nil
}

func Login(userdto dto.LoginDto) (*models.User, error) {
	if userdto.Username == "" || userdto.Password == "" {
		return nil, errors.New(models.UsernameOrPasswordCannotBeEmptyMessage)
	}

	username := strings.TrimSpace(userdto.Username)
	plain := userdto.Password

	initialUser, err := repository.GetUserByUsername(username)
	if err != nil || initialUser == nil {
		return nil, errors.New(models.UserNotFoundMessage)
	}

	return withUserPasswordMutation(initialUser.ID, func(user *models.User) error {
		policy, voceConfig, voceLoginEnabled, err := loadVoceChatLoginConfig()
		if err != nil {
			return errors.New(models.DatabaseErrorMessage)
		}
		if shouldUseVoceChatLogin(user, voceConfig, voceLoginEnabled) {
			verified, err := authenticateVoceChatPassword(user, policy, voceConfig, plain)
			if err != nil {
				return err
			}
			if verified {
				return nil
			}

			result, err := authenticateLocalPassword(username, user, plain)
			if err != nil {
				return err
			}
			maintainLocalFallbackLoginState(user, result)
		} else {
			if policy.LegacyConfiguration && isVoceChatBoundNonPrimaryUser(user) && !voceConfig.LocalFallbackEnabled {
				return inactiveVoceChatLocalFallbackError()
			}
			result, err := authenticateLocalPassword(username, user, plain)
			if err != nil {
				return err
			}
			maintainLocalFallbackLoginState(user, result)
		}

		if err := issueLoginState(user); err != nil {
			return err
		}
		return nil
	})
}

func GetStatus(currentUserID uint) (models.Status, error) {
	sysuser, err := repository.GetSysAdmin()
	if err != nil {
		return models.Status{}, errors.New(models.UserNotFoundMessage)
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
	canViewAllVoceChatEmails := currentUser.ID == models.PrimaryAdminUserID
	if !canViewAllVoceChatEmails && currentUser.IsAdmin {
		canViewAllVoceChatEmails = authorization.New(database.DB).
			Authorize(currentUser.ID, authorization.CapabilityUsersView, nil).Allowed
	}
	canViewAllVoceChatNotificationPreferences := currentUser.ID == models.PrimaryAdminUserID

	var users []models.UserStatus
	allusers, err := repository.GetAllUsers()
	if err != nil {
		return models.Status{}, errors.New(models.GetAllUsersFailMessage)
	}
	for _, user := range allusers {
		item := models.UserStatus{
			ID:        user.ID,
			Username:  user.Username,
			IsAdmin:   user.IsAdmin,
			AvatarURL: strings.TrimSpace(user.AvatarURL),
		}
		if canViewAllVoceChatEmails || currentUser.ID == user.ID {
			item.VoceChatEmail = strings.TrimSpace(user.VoceChatEmail)
		}
		if canViewAllVoceChatNotificationPreferences || currentUser.ID == user.ID {
			notificationEnabled := user.VoceChatNotificationEnabled
			item.VoceChatNotificationEnabled = &notificationEnabled
		}
		users = append(users, item)
	}

	var total int64
	messageQuery := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), viewerUserID, isAdmin)
	if viewerUserID != nil && !isAdmin {
		messageQuery = messageQuery.Where("user_id = ?", *viewerUserID)
	}
	if err := excludeDashboardSpecialMessages(messageQuery).Count(&total).Error; err != nil {
		return status, errors.New(models.GetAllMessagesFailMessage)
	}
	var personalMessages int64
	if currentUser.ID != 0 {
		if err := excludeDashboardSpecialMessages(database.DB.Model(&models.Message{}).Where("deleted_at IS NULL AND user_id = ?", currentUser.ID)).
			Count(&personalMessages).Error; err != nil {
			return status, errors.New(models.GetAllMessagesFailMessage)
		}
	}

	totalComments, totalReplies, totalGuestbook, err := countVisibleCommentStats(viewerUserID, isAdmin)
	if err != nil {
		return status, errors.New(models.GetStatusFailMessage)
	}

	status.SysAdminID = sysuser.ID
	status.Username = sysuser.Username
	status.Users = users
	status.TotalMessages = int(total)
	status.PersonalMessages = int(personalMessages)
	status.TotalUsers = len(users)
	status.TotalComments = int(totalComments)
	status.TotalReplies = int(totalReplies)
	status.TotalGuestbook = int(totalGuestbook)

	if currentUser.ID != 0 {
		receivedLikes, receivedComments, receivedReplies, receivedGuestbook, err := countReceivedInteractionStats(currentUser.ID, isAdmin)
		if err != nil {
			return status, errors.New(models.GetStatusFailMessage)
		}
		status.ReceivedLikes = int(receivedLikes)
		status.ReceivedComments = int(receivedComments)
		status.ReceivedReplies = int(receivedReplies)
		status.ReceivedGuestbook = int(receivedGuestbook)

		autoBanEnabled := false
		var securityConfig models.SecurityConfig
		if err := database.DB.Order("id asc").First(&securityConfig).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return status, errors.New(models.GetStatusFailMessage)
		} else if err == nil {
			autoBanEnabled = securityConfig.AutoBanEnabled
		}
		status.AutoBanEnabled = &autoBanEnabled
	}

	return status, nil
}

func excludeDashboardSpecialMessages(query *gorm.DB) *gorm.DB {
	if descriptor, err := ResolveGuestbook(database.DB); err == nil && descriptor.MessageID != 0 {
		query = query.Where("messages.id <> ?", descriptor.MessageID)
	} else {
		query = query.Where(GuestbookSQLPredicate("messages.is_guestbook"))
	}
	return query.Where(
		"messages.content NOT LIKE ? AND messages.content NOT LIKE ? AND messages.content NOT LIKE ? AND messages.content NOT LIKE ?",
		"%#友链%", "%友情链接%",
		"%#关于%", "%关于本站%",
	)
}

func countVisibleCommentStats(viewerUserID *uint, _ bool) (int64, int64, int64, error) {
	scope, err := ResolveContentReadScope(database.DB, viewerUserID)
	if err != nil {
		return 0, 0, 0, err
	}
	var messages []models.Message
	messageQuery := scope.ApplyMessageVisibility(database.DB.Model(&models.Message{}).Select("id", "user_id", "private", "visibility", "content", "is_guestbook"))
	if err := messageQuery.Find(&messages).Error; err != nil {
		return 0, 0, 0, err
	}
	if len(messages) == 0 {
		return 0, 0, 0, nil
	}

	messageIDs := make([]uint, 0, len(messages))
	messageMap := make(map[uint]models.Message, len(messages))
	for _, message := range messages {
		messageIDs = append(messageIDs, message.ID)
		messageMap[message.ID] = message
	}
	var comments []models.Comment
	if err := database.DB.Where("message_id IN ?", messageIDs).Order("created_at ASC, id ASC").Find(&comments).Error; err != nil {
		return 0, 0, 0, err
	}
	commentsByMessage := make(map[uint][]models.Comment)
	for _, comment := range comments {
		commentsByMessage[comment.MessageID] = append(commentsByMessage[comment.MessageID], comment)
	}

	var totalComments, totalReplies, totalGuestbook int64
	for messageID, messageComments := range commentsByMessage {
		message := messageMap[messageID]
		isGuestbook := IsGuestbookMessage(message)
		commentMap := CommentMap(messageComments)
		for _, comment := range messageComments {
			if !scope.CanReadComment(message, comment, commentMap) {
				continue
			}
			if comment.ParentID == nil && isGuestbook {
				totalGuestbook++
			} else if comment.ParentID == nil {
				totalComments++
			} else {
				totalReplies++
			}
		}
	}
	return totalComments, totalReplies, totalGuestbook, nil
}

func countReceivedInteractionStats(userID uint, _ bool) (int64, int64, int64, int64, error) {
	var receivedLikes int64
	likeQuery := database.DB.Model(&models.MessageLike{}).
		Joins("JOIN messages ON messages.id = message_likes.message_id").
		Where("messages.deleted_at IS NULL").
		Where("messages.user_id = ?", userID).
		Where("(message_likes.user_id IS NULL OR message_likes.user_id <> ?)", userID)
	if err := excludeDashboardSpecialMessages(likeQuery).Count(&receivedLikes).Error; err != nil {
		return 0, 0, 0, 0, err
	}

	var receivedComments int64
	commentQuery := database.DB.Model(&models.Comment{}).
		Joins("JOIN messages ON messages.id = comments.message_id").
		Where("messages.deleted_at IS NULL").
		Where("messages.user_id = ?", userID).
		Where("comments.parent_id IS NULL").
		Where("(comments.user_id IS NULL OR comments.user_id <> ?)", userID)
	if err := excludeDashboardSpecialMessages(commentQuery).Count(&receivedComments).Error; err != nil {
		return 0, 0, 0, 0, err
	}

	var receivedReplies int64
	if err := database.DB.Model(&models.Comment{}).
		Joins("JOIN messages ON messages.id = comments.message_id").
		Joins("JOIN comments AS parent_comments ON parent_comments.id = comments.parent_id").
		Where("messages.deleted_at IS NULL").
		Where("parent_comments.user_id = ?", userID).
		Where("(comments.user_id IS NULL OR comments.user_id <> ?)", userID).
		Count(&receivedReplies).Error; err != nil {
		return 0, 0, 0, 0, err
	}

	var receivedGuestbook int64
	if userID == GuestbookRecipientID() {
		descriptor, resolveErr := ResolveGuestbook(database.DB)
		if resolveErr != nil && !errors.Is(resolveErr, gorm.ErrRecordNotFound) {
			return 0, 0, 0, 0, resolveErr
		}
		if descriptor.MessageID == 0 {
			return receivedLikes, receivedComments, receivedReplies, 0, nil
		}
		if err := database.DB.Model(&models.Comment{}).
			Joins("JOIN messages ON messages.id = comments.message_id").
			Where("messages.deleted_at IS NULL").
			Where("comments.parent_id IS NULL").
			Where("(comments.user_id IS NULL OR comments.user_id <> ?)", userID).
			Where("comments.message_id = ?", descriptor.MessageID).
			Count(&receivedGuestbook).Error; err != nil {
			return 0, 0, 0, 0, err
		}
	}

	return receivedLikes, receivedComments, receivedReplies, receivedGuestbook, nil
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
	if userdto.VoceChatNotificationEnabled != nil && *userdto.VoceChatNotificationEnabled != user.VoceChatNotificationEnabled {
		updates["voce_chat_notification_enabled"] = *userdto.VoceChatNotificationEnabled
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
	if v, ok := updates["voce_chat_notification_enabled"]; ok && v != nil {
		user.VoceChatNotificationEnabled = v.(bool)
	}
	if err := updatePlainPasswordUserMetadata(user); err != nil {
		log.Printf("password metadata maintenance failed: user_id=%d trigger=profile_update error_type=%T", user.ID, err)
	}

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
	_, err := withUserPasswordMutation(user.ID, func(current *models.User) error {
		newPassword := strings.TrimSpace(userdto.Password)
		if newPassword == "" {
			return errors.New(models.PasswordCannotBeEmptyMessage)
		}

		// 如果新密码与旧密码一致，则拒绝
		if passwordMatchesStored(current.Password, newPassword) {
			return errors.New(models.PasswordCannotBeSameAsBeforeMessage)
		}

		if isVoceChatBoundNonPrimaryUser(current) {
			return changeBoundUserPasswordForRuntimePolicy(current, newPassword, "")
		}
		return saveLocalUserPassword(current, newPassword)
	})
	return err
}

func verifyVoceChatPasswordForPasswordChange(user *models.User, config vocechat.Config, plain string) error {
	if !config.Enabled || !config.IsReady() {
		return errors.New("VoceChat 登录校验暂不可用，请稍后再试")
	}
	_, err := voceChatPasswordLogin(context.Background(), config, strings.TrimSpace(user.VoceChatEmail), plain)
	if err == nil {
		recordVoceChatLoginHealth("ok", nil)
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
	policy, config, _, err := loadVoceChatLoginConfig()
	if err != nil {
		return errors.New(models.DatabaseErrorMessage)
	}
	if isVoceChatBoundNonPrimaryUser(user) {
		if policy.LegacyConfiguration {
			return verifyVoceChatPasswordForPasswordChange(user, config, old)
		}
		if policy.RuntimeState == runtimepolicy.StateLocal {
			if !passwordMatchesStored(user.Password, old) {
				return errors.New(models.PasswordIncorrectMessage)
			}
			return nil
		}
		if policy.RuntimeState == runtimepolicy.StateVoceChatDegraded {
			return errors.New("VoceChat 暂不可用，绑定账户暂时不能修改密码，请稍后重试")
		}
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
	_, err := withUserPasswordMutation(user.ID, func(current *models.User) error {
		old = strings.TrimSpace(old)
		new = strings.TrimSpace(new)
		if new == "" {
			return errors.New(models.PasswordCannotBeEmptyMessage)
		}

		if err := verifyPasswordChangeOldPassword(current, old); err != nil {
			return err
		}

		// 新密码不得与旧密码一致
		if old == new {
			return errors.New(models.PasswordCannotBeSameAsBeforeMessage)
		}
		if passwordMatchesStored(current.Password, new) {
			return errors.New(models.PasswordCannotBeSameAsBeforeMessage)
		}

		if isVoceChatBoundNonPrimaryUser(current) {
			return changeBoundUserPasswordForRuntimePolicy(current, new, old)
		}
		return saveLocalUserPassword(current, new)
	})
	return err
}

func UpdateUserAdmin(userID uint, currentUserID uint) error {
	if currentUserID != models.PrimaryAdminUserID {
		return errors.New("仅 1 号管理员可以变更管理员身份")
	}
	if userID == models.PrimaryAdminUserID {
		return errors.New("不能变更 1 号管理员身份")
	}
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
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var fresh models.User
		if err := tx.First(&fresh, userID).Error; err != nil {
			return err
		}
		fresh.IsAdmin = !fresh.IsAdmin
		if err := tx.Save(&fresh).Error; err != nil {
			return err
		}
		if !fresh.IsAdmin {
			if err := tx.Where("user_id = ?", fresh.ID).Delete(&models.AdminCapabilityGrant{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func GetUserByUsername(username string) (*models.User, error) {
	user, err := repository.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	return user, nil
}
