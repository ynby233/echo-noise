package services

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"gorm.io/gorm"
)

const AICredentialTokenPrefix = "noise_ai_"

var (
	ErrInvalidAICredentialName = errors.New("凭据名称必须为 1–100 个字符")
	ErrInvalidBearerIdentity   = errors.New("无效的 Bearer 凭据")
	ErrBearerLoginExpired      = errors.New("登录已过期")
)

// AI credential operations take this lock before Login takes its password
// mutation lock. No password-mutation path acquires this lock in reverse order.
// This serializes operations within one server process, including SQLite;
// it is not a distributed lock for multiple servers sharing a database.
var aiCredentialUserLocks sync.Map

func aiCredentialUserMutex(userID uint) *sync.Mutex {
	value, _ := aiCredentialUserLocks.LoadOrStore(userID, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func hashAICredentialToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

// CreateAICredential returns the secret once, separately from the persisted model.
func CreateAICredential(userID uint, name string) (*models.AICredential, string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 {
		return nil, "", ErrInvalidAICredentialName
	}
	mutex := aiCredentialUserMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()
	db, err := database.GetDB()
	if err != nil {
		return nil, "", err
	}
	var user models.User
	if userID == 0 {
		return nil, "", ErrInvalidBearerIdentity
	}
	if err := db.First(&user, userID).Error; err != nil {
		return nil, "", err
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, "", err
	}
	token := AICredentialTokenPrefix + base64.RawURLEncoding.EncodeToString(random[:])
	credential := models.AICredential{UserID: userID, Name: name, TokenHash: hashAICredentialToken(token)}
	if err := db.Create(&credential).Error; err != nil {
		return nil, "", err
	}
	return &credential, token, nil
}

func ListAICredentials(userID uint) ([]models.AICredential, error) {
	credentials := make([]models.AICredential, 0)
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	err = db.Where("user_id = ?", userID).Order("id DESC").Find(&credentials).Error
	return credentials, err
}

// RevokeAICredential deliberately gives the same result for missing and foreign IDs.
func RevokeAICredential(userID, credentialID uint) error {
	mutex := aiCredentialUserMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	result := db.Where("id = ? AND user_id = ?", credentialID, userID).Delete(&models.AICredential{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ResolveBearerIdentity reads current database identity and login policy on every
// call. allowExpired bypasses login expiry only for an existing AI credential;
// it never bypasses revocation, missing owners, or ordinary-token expiry.
// The source is "token" or "ai_token"; ordinary tokens have a nil credential.
func ResolveBearerIdentity(header string, allowExpired bool) (*models.User, *models.AICredential, string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, nil, "", ErrInvalidBearerIdentity
	}
	token := parts[1]
	db, err := database.GetDB()
	if err != nil {
		return nil, nil, "", err
	}
	var user models.User
	var credential *models.AICredential
	via := "token"
	if strings.HasPrefix(token, AICredentialTokenPrefix) {
		secret, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(token, AICredentialTokenPrefix))
		if err != nil || len(secret) != 32 {
			return nil, nil, "", ErrInvalidBearerIdentity
		}
		credential = &models.AICredential{}
		if err := db.Where("token_hash = ?", hashAICredentialToken(token)).First(credential).Error; err != nil {
			return nil, nil, "", ErrInvalidBearerIdentity
		}
		if credential.UserID == 0 || db.First(&user, credential.UserID).Error != nil {
			return nil, nil, "", ErrInvalidBearerIdentity
		}
		via = "ai_token"
	} else {
		if err := db.Where("token = ?", token).First(&user).Error; err != nil {
			return nil, nil, "", ErrInvalidBearerIdentity
		}
		// SQL collations may be case-insensitive; bearer secrets never are.
		if subtle.ConstantTimeCompare([]byte(user.Token), []byte(token)) != 1 {
			return nil, nil, "", ErrInvalidBearerIdentity
		}
	}
	var issuedAt int64
	if user.LoginIssuedAt != nil {
		issuedAt = user.LoginIssuedAt.Unix()
	}
	if !(allowExpired && credential != nil) && IsUserLoginExpired(&user, issuedAt, time.Now()) {
		return nil, nil, "", ErrBearerLoginExpired
	}
	return &user, credential, via, nil
}

// LoginWithAICredential checks ownership BEFORE Login, which can refresh login
// state and synchronize passwords. Cookie/session state is never used here.
func LoginWithAICredential(header string, input dto.LoginDto) (*models.User, *models.AICredential, error) {
	owner, credential, via, err := ResolveBearerIdentity(header, true)
	if err != nil {
		return nil, nil, err
	}
	if via != "ai_token" || credential == nil {
		return nil, nil, ErrInvalidBearerIdentity
	}
	mutex := aiCredentialUserMutex(owner.ID)
	mutex.Lock()
	defer mutex.Unlock()
	// The first resolution only locates the account lock. Revocation may have
	// completed while we waited, so validate again before any Login side effect.
	owner, credential, via, err = ResolveBearerIdentity(header, true)
	if err != nil {
		return nil, nil, err
	}
	if via != "ai_token" || credential == nil {
		return nil, nil, ErrInvalidBearerIdentity
	}
	candidate, err := repository.GetUserByUsername(strings.TrimSpace(input.Username))
	if err != nil || candidate.ID != owner.ID {
		return nil, nil, ErrInvalidBearerIdentity
	}
	// Login uses the user repository cache inside its password-mutation lock.
	// Discard a stale password/role snapshot before invoking the existing flow.
	repository.InvalidateUserCache(owner.ID)
	user, err := Login(input)
	if err != nil {
		return nil, nil, err
	}
	if user == nil || user.ID != owner.ID {
		return nil, nil, ErrInvalidBearerIdentity
	}
	// Re-read current identity while still holding the credential-operation lock.
	user, credential, _, err = ResolveBearerIdentity(header, false)
	if err != nil {
		return nil, nil, err
	}
	return user, credential, nil
}
