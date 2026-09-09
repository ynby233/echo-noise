package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
)

// Douyin URL resolution and media proxying own their upstream request behavior here.
func ResolveDouyinShortURL(c *gin.Context) {
	vid := strings.TrimSpace(c.Query("vid"))
	raw := strings.TrimSpace(c.Query("url"))
	if vid == "" && raw == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("url 或 vid 不能为空"))
		return
	}

	resolvedURL := ""
	var err error
	if vid == "" {
		vid, resolvedURL, err = resolveDouyinVideoIDFromURL(raw)
		if err != nil {
			c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
			return
		}
	}
	if vid == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("未提取到视频ID"))
		return
	}

	playURL, playErr := fetchDouyinPlayURLByID(vid)
	resp := gin.H{
		"video_id":     vid,
		"resolved_url": resolvedURL,
	}
	if playErr == nil && strings.TrimSpace(playURL) != "" {
		resp["play_url"] = playURL
	}
	c.JSON(http.StatusOK, dto.OK(resp, "解析成功"))
}

// ProxyDouyinVideo 由后端中转抖音视频流，避免前端直连被防盗链策略拦截
func ProxyDouyinVideo(c *gin.Context) {
	vid := strings.TrimSpace(c.Query("vid"))
	raw := strings.TrimSpace(c.Query("url"))
	if vid == "" {
		if raw == "" {
			c.String(http.StatusBadRequest, "url 或 vid 不能为空")
			return
		}
		var err error
		vid, _, err = resolveDouyinVideoIDFromURL(raw)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
	}
	if vid == "" {
		c.String(http.StatusBadRequest, "未提取到视频ID")
		return
	}
	playURL, err := fetchDouyinPlayURLByID(vid)
	if err != nil || strings.TrimSpace(playURL) == "" {
		c.String(http.StatusBadGateway, "获取抖音视频地址失败")
		return
	}

	req, _ := http.NewRequest(http.MethodGet, playURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile")
	req.Header.Set("Referer", "https://www.douyin.com/")
	req.Header.Set("Accept", "*/*")
	if r := strings.TrimSpace(c.GetHeader("Range")); r != "" {
		req.Header.Set("Range", r)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.String(http.StatusBadGateway, "拉取抖音视频流失败")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.String(http.StatusBadGateway, "抖音视频源不可用")
		return
	}

	if v := strings.TrimSpace(resp.Header.Get("Content-Type")); v != "" {
		c.Header("Content-Type", v)
	} else {
		c.Header("Content-Type", "video/mp4")
	}
	for _, h := range []string{
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Cache-Control",
		"Expires",
		"Last-Modified",
		"ETag",
	} {
		if v := strings.TrimSpace(resp.Header.Get(h)); v != "" {
			c.Header(h, v)
		}
	}

	c.Status(resp.StatusCode)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
}

func resolveDouyinVideoIDFromURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("url 不能为空")
	}
	if regexp.MustCompile(`^\d+$`).MatchString(raw) {
		return raw, "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("url 格式错误")
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	if !strings.Contains(host, "douyin.com") && !strings.Contains(host, "iesdouyin.com") {
		return "", "", fmt.Errorf("仅支持抖音链接")
	}
	client := &http.Client{Timeout: 8 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, raw, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("短链解析失败")
	}
	defer resp.Body.Close()
	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	target := finalURL
	if target == "" {
		target = raw
	}
	rePath := regexp.MustCompile(`/video/(\d+)`)
	if m := rePath.FindStringSubmatch(target); len(m) > 1 {
		return strings.TrimSpace(m[1]), target, nil
	}
	if pu, e := url.Parse(target); e == nil {
		if id := strings.TrimSpace(pu.Query().Get("modal_id")); id != "" {
			return id, target, nil
		}
	}
	return "", target, fmt.Errorf("未提取到视频ID")
}

func fetchDouyinPlayURLByID(videoID string) (string, error) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return "", fmt.Errorf("video_id 不能为空")
	}
	apiURL := fmt.Sprintf("https://www.iesdouyin.com/web/api/v2/aweme/iteminfo/?item_ids=%s", url.QueryEscape(videoID))
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("请求抖音视频信息失败")
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	itemList, _ := data["item_list"].([]interface{})
	if len(itemList) == 0 {
		return "", fmt.Errorf("未获取到抖音视频信息")
	}
	item, _ := itemList[0].(map[string]interface{})
	video, _ := item["video"].(map[string]interface{})
	playAddr, _ := video["play_addr"].(map[string]interface{})
	urlList, _ := playAddr["url_list"].([]interface{})
	for _, it := range urlList {
		u, _ := it.(string)
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		// 优先去水印地址
		u = strings.ReplaceAll(u, "playwm", "play")
		return u, nil
	}
	return "", fmt.Errorf("未获取到可播放地址")
}

// ListFriendLinkApplications 管理员查看友链申请列表
