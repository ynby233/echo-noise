package vocechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	apiKeyHeader        = "X-API-Key"
	thirdPartySecretKey = "X-SECRET"
	defaultHTTPTimeout  = 15 * time.Second
	defaultDevice       = "echo-noise"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("VoceChat %s %s failed with HTTP %d", e.Method, e.Path, e.StatusCode)
	}
	return fmt.Sprintf("VoceChat %s %s failed with HTTP %d: %s", e.Method, e.Path, e.StatusCode, body)
}

type User struct {
	UID                 int64  `json:"uid"`
	Email               string `json:"email"`
	Password            string `json:"password"`
	Name                string `json:"name"`
	Gender              int    `json:"gender"`
	IsAdmin             bool   `json:"is_admin"`
	Language            string `json:"language"`
	CreateBy            string `json:"create_by"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
	Status              string `json:"status"`
	IsBot               bool   `json:"is_bot"`
	MsgSMTPNotifyEnable bool   `json:"msg_smtp_notify_enable"`
}

type UserInfo struct {
	UID                 int64  `json:"uid"`
	Email               string `json:"email"`
	Name                string `json:"name"`
	Gender              int    `json:"gender"`
	Language            string `json:"language"`
	IsAdmin             bool   `json:"is_admin"`
	IsBot               bool   `json:"is_bot"`
	CreateBy            string `json:"create_by"`
	MsgSMTPNotifyEnable bool   `json:"msg_smtp_notify_enable"`
}

type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Gender   int    `json:"gender"`
	IsAdmin  bool   `json:"is_admin"`
	Language string `json:"language,omitempty"`
	IsBot    bool   `json:"is_bot,omitempty"`
}

type UpdateUserRequest struct {
	Email    *string `json:"email,omitempty"`
	Password *string `json:"password,omitempty"`
	Name     *string `json:"name,omitempty"`
	Gender   *int    `json:"gender,omitempty"`
	IsAdmin  *bool   `json:"is_admin,omitempty"`
	Language *string `json:"language,omitempty"`
	Status   *string `json:"status,omitempty"`
}

type LoginResponse struct {
	ServerID     string   `json:"server_id"`
	Token        string   `json:"token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiredIn    int64    `json:"expired_in"`
	User         UserInfo `json:"user"`
}

type RenewTokenResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	ExpiredIn    int64  `json:"expired_in"`
}

type UserContact struct {
	TargetUID   int64    `json:"target_uid"`
	TargetInfo  UserInfo `json:"target_info"`
	ContactInfo struct {
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at"`
		UpdatedAt int64  `json:"updated_at"`
	} `json:"contact_info"`
}

type UserConflict struct {
	Reason string `json:"reason"`
}

func NewClient(config Config) (*Client, error) {
	return NewClientWithHTTPClient(config, nil)
}

func NewClientWithHTTPClient(config Config, httpClient *http.Client) (*Client, error) {
	baseURL := NormalizeAPIBaseURL(config.BaseURL)
	if baseURL == "" {
		return nil, errors.New("VoceChat 地址不能为空")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("VoceChat 地址无效: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}, nil
}

func (c *Client) CheckEmail(ctx context.Context, email string) (bool, error) {
	var available bool
	path := "/user/check_email?email=" + url.QueryEscape(strings.TrimSpace(email))
	if err := c.do(ctx, http.MethodGet, path, "", nil, &available, nil); err != nil {
		return false, err
	}
	return available, nil
}

func (c *Client) LoginWithPassword(ctx context.Context, email, password, device string) (*LoginResponse, error) {
	if strings.TrimSpace(device) == "" {
		device = defaultDevice
	}
	body := map[string]interface{}{
		"credential": map[string]string{
			"type":     "password",
			"email":    strings.TrimSpace(email),
			"password": password,
		},
		"device": device,
	}
	var response LoginResponse
	if err := c.do(ctx, http.MethodPost, "/token/login", "", body, &response, nil); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) RenewToken(ctx context.Context, token, refreshToken string) (*RenewTokenResponse, error) {
	body := map[string]string{
		"token":         strings.TrimSpace(token),
		"refresh_token": strings.TrimSpace(refreshToken),
	}
	var response RenewTokenResponse
	if err := c.do(ctx, http.MethodPost, "/token/renew", "", body, &response, nil); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) CreateUser(ctx context.Context, apiKey string, request CreateUserRequest) (*User, error) {
	if strings.TrimSpace(request.Language) == "" {
		request.Language = "zh-CN"
	}
	var user User
	if err := c.do(ctx, http.MethodPost, "/admin/user", apiKey, request, &user, nil); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) ListUsers(ctx context.Context, apiKey string) ([]User, error) {
	var users []User
	if err := c.do(ctx, http.MethodGet, "/admin/user", apiKey, nil, &users, nil); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *Client) CheckHealth(ctx context.Context, apiKey string) error {
	_, err := c.ListUsers(ctx, apiKey)
	return err
}

func (c *Client) SendMarkdownToUser(ctx context.Context, botAPIKey string, uid string, markdown string) error {
	botAPIKey = strings.TrimSpace(botAPIKey)
	uid = strings.TrimSpace(uid)
	markdown = strings.TrimSpace(markdown)
	if botAPIKey == "" {
		return fmt.Errorf("VoceChat Bot API Key 未配置")
	}
	if uid == "" {
		return fmt.Errorf("VoceChat 用户 ID 为空")
	}
	if markdown == "" {
		return fmt.Errorf("VoceChat 推送内容为空")
	}

	path := "/bot/send_to_user/" + url.PathEscape(uid)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(markdown))
	if err != nil {
		return fmt.Errorf("创建 VoceChat Bot 请求失败: %w", err)
	}
	req.Header.Set(apiKeyHeader, botAPIKey)
	req.Header.Set("Content-Type", "text/markdown")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 VoceChat Bot 失败: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取 VoceChat Bot 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Method: http.MethodPost, Path: path, Body: string(data)}
	}
	return nil
}

func (c *Client) GetUser(ctx context.Context, apiKey string, uid int64) (*User, error) {
	var user User
	if err := c.do(ctx, http.MethodGet, "/admin/user/"+strconv.FormatInt(uid, 10), apiKey, nil, &user, nil); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) UpdateUser(ctx context.Context, apiKey string, uid int64, request UpdateUserRequest) (*User, error) {
	var user User
	if err := c.do(ctx, http.MethodPut, "/admin/user/"+strconv.FormatInt(uid, 10), apiKey, request, &user, nil); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) DeleteUser(ctx context.Context, apiKey string, uid int64) error {
	return c.do(ctx, http.MethodDelete, "/admin/user/"+strconv.FormatInt(uid, 10), apiKey, nil, nil, nil)
}

func (c *Client) ListContacts(ctx context.Context, apiKey string) ([]UserContact, error) {
	var contacts []UserContact
	if err := c.do(ctx, http.MethodGet, "/user/contacts", apiKey, nil, &contacts, nil); err != nil {
		return nil, err
	}
	return contacts, nil
}

func (c *Client) CreateThirdPartyKey(ctx context.Context, secret, userID, username string) (string, error) {
	body := map[string]string{
		"userid":   strings.TrimSpace(userID),
		"username": strings.TrimSpace(username),
	}
	var key string
	headers := map[string]string{thirdPartySecretKey: strings.TrimSpace(secret)}
	if err := c.do(ctx, http.MethodPost, "/token/create_third_party_key", "", body, &key, headers); err != nil {
		return "", err
	}
	return key, nil
}

func (c *Client) do(ctx context.Context, method, path, apiKey string, body interface{}, out interface{}, headers map[string]string) error {
	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("序列化 VoceChat 请求失败: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("创建 VoceChat 请求失败: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if token := strings.TrimSpace(apiKey); token != "" {
		req.Header.Set(apiKeyHeader, token)
	}
	for key, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 VoceChat 失败: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取 VoceChat 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Method: method, Path: path, Body: string(data)}
	}
	if out == nil || len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析 VoceChat 响应失败: %w", err)
	}
	return nil
}
