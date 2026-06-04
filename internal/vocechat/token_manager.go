package vocechat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const tokenRefreshSkew = 30 * time.Second

type AdminTokenManager struct {
	client *Client
	config Config
	now    func() time.Time

	mu           sync.Mutex
	token        string
	refreshToken string
	expiresAt    time.Time
}

func NewAdminTokenManager(client *Client, config Config) *AdminTokenManager {
	return &AdminTokenManager{
		client: client,
		config: config,
		now:    time.Now,
	}
}

func (m *AdminTokenManager) GetToken(ctx context.Context) (string, error) {
	if m == nil || m.client == nil {
		return "", errors.New("VoceChat 客户端未初始化")
	}
	if token := strings.TrimSpace(m.config.AdminToken); token != "" {
		return token, nil
	}
	if strings.TrimSpace(m.config.AdminUsername) == "" || strings.TrimSpace(m.config.AdminPassword) == "" {
		return "", errors.New("VoceChat 管理员账号或密码未配置")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	if strings.TrimSpace(m.token) != "" && now.Before(m.expiresAt.Add(-tokenRefreshSkew)) {
		return m.token, nil
	}

	if strings.TrimSpace(m.refreshToken) != "" {
		if renewed, err := m.client.RenewToken(ctx, m.token, m.refreshToken); err == nil && renewed != nil && strings.TrimSpace(renewed.Token) != "" {
			m.applyRenewedToken(renewed, now)
			return m.token, nil
		}
	}

	login, err := m.client.LoginWithPassword(ctx, m.config.AdminUsername, m.config.AdminPassword, defaultDevice)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("VoceChat 管理员邮箱不存在，请填写管理员邮箱而不是显示名: %w", err)
		}
		return "", err
	}
	m.applyLoginToken(login, now)
	if strings.TrimSpace(m.token) == "" {
		return "", fmt.Errorf("VoceChat 管理员登录未返回 token")
	}
	return m.token, nil
}

func (m *AdminTokenManager) Invalidate() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.token = ""
	m.refreshToken = ""
	m.expiresAt = time.Time{}
}

func (m *AdminTokenManager) applyLoginToken(response *LoginResponse, now time.Time) {
	if response == nil {
		return
	}
	m.token = strings.TrimSpace(response.Token)
	m.refreshToken = strings.TrimSpace(response.RefreshToken)
	m.expiresAt = tokenExpiry(now, response.ExpiredIn)
}

func (m *AdminTokenManager) applyRenewedToken(response *RenewTokenResponse, now time.Time) {
	if response == nil {
		return
	}
	m.token = strings.TrimSpace(response.Token)
	m.refreshToken = strings.TrimSpace(response.RefreshToken)
	m.expiresAt = tokenExpiry(now, response.ExpiredIn)
}

func tokenExpiry(now time.Time, expiredIn int64) time.Time {
	if expiredIn <= 0 {
		return now
	}
	return now.Add(time.Duration(expiredIn) * time.Second)
}
