package controllers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
)

func GetNotifyConfig(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	config := models.GetNotifyConfig()
	if config == nil {
		// 如果配置不存在，返回空配置（所有字段默认值）
		config = &models.NotifyConfig{
			WebhookEnabled:           false,
			WebhookURL:               "",
			TelegramEnabled:          false,
			TelegramToken:            "",
			TelegramChatID:           "",
			WeworkEnabled:            false,
			WeworkKey:                "",
			FeishuEnabled:            false,
			FeishuWebhook:            "",
			FeishuSecret:             "",
			TwitterEnabled:           false,
			TwitterApiKey:            "",
			TwitterApiSecret:         "",
			TwitterAccessToken:       "",
			TwitterAccessTokenSecret: "",
			CustomHttpEnabled:        false,
			CustomHttpUrl:            "",
			CustomHttpMethod:         "",
			CustomHttpHeaders:        "",
			CustomHttpBody:           "",
		}
	}
	c.JSON(http.StatusOK, dto.OK(config, "获取成功"))
}

// SaveNotifyConfig 保存推送配置
func SaveNotifyConfig(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	var config models.NotifyConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("无效的配置数据"))
		return
	}
	// Twitter校验
	if config.TwitterEnabled {
		if config.TwitterApiKey == "" || config.TwitterApiSecret == "" || config.TwitterAccessToken == "" || config.TwitterAccessTokenSecret == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("Twitter配置不完整"))
			return
		}
	}
	// 自定义HTTP校验
	if config.CustomHttpEnabled {
		if config.CustomHttpUrl == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("自定义HTTP URL不能为空"))
			return
		}
	}
	// 根据启用状态验证配置
	if config.WebhookEnabled {
		if config.WebhookURL == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("Webhook URL 不能为空"))
			return
		}
	}
	if config.TelegramEnabled {
		if config.TelegramToken == "" || config.TelegramChatID == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("Telegram 配置不完整"))
			return
		}
	}
	if config.WeworkEnabled {
		if config.WeworkKey == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("企业微信 Key 不能为空"))
			return
		}
	}
	if config.FeishuEnabled {
		if config.FeishuWebhook == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("飞书 Webhook 不能为空"))
			return
		}
	}

	if err := models.SaveNotifyConfig(config); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("保存配置失败: "+err.Error()))
		return
	}

	savedConfig := models.GetNotifyConfig()
	c.JSON(http.StatusOK, dto.OK(savedConfig, "配置已更新"))
}

// TestNotify 测试推送
func TestNotify(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	var request struct {
		Type string `json:"type" binding:"required"`
		To   string `json:"to"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("无效的请求参数"))
		return
	}

	testMsg := "这是一条测试消息 - " + time.Now().Format("2006-01-02 15:04:05")
	var emptyImages []string

	var testErr error
	switch request.Type {
	case "webhook":
		testErr = models.SendWebhook(testMsg)
	case "telegram":
		testErr = models.SendTelegram(testMsg, emptyImages)
	case "wework":
		testErr = models.SendWework(testMsg, emptyImages)
	case "feishu":
		testErr = models.SendFeishu(testMsg)
	case "twitter":
		testErr = models.SendTwitter(testMsg)
	case "customHttp":
		testErr = models.SendCustomHttp(testMsg)
	case "email":
		to := strings.TrimSpace(request.To)
		if to == "" {
			db, _ := database.GetDB()
			var cfg models.SiteConfig
			_ = db.Table("site_configs").First(&cfg).Error
			if cfg.SmtpFrom != "" {
				to = cfg.SmtpFrom
			} else {
				to = cfg.SmtpUser
			}
		}
		testErr = models.SendTestEmail(to)
	default:
		c.JSON(http.StatusOK, dto.Fail[string]("不支持的推送类型"))
		return
	}

	if testErr != nil {
		c.JSON(http.StatusOK, dto.Fail[string](fmt.Sprintf("推送测试失败: %v", testErr)))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, "推送测试已发送"))
}

// 上传视频

func SendNotify(c *gin.Context) {
	var request struct {
		Content string   `json:"content"`
		Images  []string `json:"images"`
		Format  string   `json:"format"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}

	// 验证内容不为空
	if request.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "推送内容不能为空"})
		return
	}

	// 获取推送配置
	config := models.GetNotifyConfig()
	if config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "推送配置不存在"})
		return
	}

	// 并发处理所有启用的推送渠道
	type notifyResult struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	results := map[string]notifyResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Telegram
	if config.TelegramEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendTelegram(request.Content, request.Images)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["telegram"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["telegram"] = notifyResult{Success: true}
			}
		}()
	}

	// 企业微信
	if config.WeworkEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendWework(request.Content, request.Images)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["wework"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["wework"] = notifyResult{Success: true}
			}
		}()
	}

	// 飞书
	if config.FeishuEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendFeishu(request.Content)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["feishu"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["feishu"] = notifyResult{Success: true}
			}
		}()
	}

	// Webhook
	if config.WebhookEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendWebhook(request.Content)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["webhook"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["webhook"] = notifyResult{Success: true}
			}
		}()
	}
	// Twitter
	if config.TwitterEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Twitter 字数限制 280
			tweet := request.Content
			if len([]rune(tweet)) > 280 {
				tweet = string([]rune(tweet)[:280]) + "...(内容截断)"
			}
			err := models.SendTwitter(tweet)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["twitter"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["twitter"] = notifyResult{Success: true}
			}
		}()
	}

	// 自定义 HTTP
	if config.CustomHttpEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendCustomHttp(request.Content)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["customHttp"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["customHttp"] = notifyResult{Success: true}
			}
		}()
	}

	// 等待所有推送完成
	wg.Wait()

	anyFail := false
	for _, r := range results {
		if !r.Success {
			anyFail = true
			break
		}
	}
	if anyFail {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "部分推送失败", "data": results})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "推送成功", "data": results})
}

func sendTelegramErrorNotify(c *gin.Context, err error) {
	log.Printf("Telegram 推送失败: %v", err)
}
