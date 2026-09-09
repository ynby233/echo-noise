package services

import (
	"fmt"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"io"
	"os"
	"strings"
)

// Default-setting helpers define initial values and normalize imported feed-source configuration.
func getDefaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"allowRegistration":       true,
		"autoApproveRegistration": false,
		"frontendSettings": map[string]interface{}{
			"siteTitle":           neutralSiteTitle,
			"subtitleText":        "欢迎访问，点击头像可更换封面背景！",
			"avatarURL":           neutralAvatarURL,
			"username":            neutralOwnerName,
			"description":         neutralDescription,
			"notifyEnabled":       false,
			"backgrounds":         defaultHeaderImages(),
			"pageFooterHTML":      "",
			"rssTitle":            neutralRSSTitle,
			"rssDescription":      neutralRSSDescription,
			"rssAuthorName":       neutralOwnerName,
			"rssFaviconURL":       "/favicon-32x32.png",
			"rssEnabled":          false,
			"rssMemberIDs":        []uint{},
			"rssAvailableMembers": []map[string]interface{}{},
			"enableGithubCard":    false,
			// 页面文案与关于页内容
			"commentPageTitle":            "留言",
			"commentPageDescription":      "欢迎留下你的看法",
			"notificationPageTitle":       "通知",
			"notificationPageDescription": "欢迎彼此间互相交流",
			"announcementPageTitle":       "公告",
			"announcementPageDescription": "查看站点发布的最新公告",
			"aboutPageTitle":              "关于本站",
			"aboutPageDescription":        "这里是站点的介绍与说明",
			"aboutMarkdown":               "# 关于我\n\n这里是一个默认的个人简介示例：\n\n- 喜欢记录与分享\n- 热爱开源与学习\n- 持续打磨产品体验\n\n欢迎留言与我交流！",
			"loginExpireDays":             3,
			"loginExpireHours":            0,
			"feedEnabled":                 false,
			"feedPageTitle":               "实时聚合内容动态",
			"feedPageDescription":         "聚合综合内容信息源内容，当前结果 {count} 条",
			"feedLimit":                   100,
			"feedRefreshSeconds":          7200,
			"feedSources": []map[string]interface{}{
				{"type": "rss", "group": "默认分组", "name": "站点 RSS", "url": "/rss", "enabled": true, "visible": true},
			},
			// 系统欢迎组件默认参数
			"welcomeAvatarURL":       neutralAvatarURL,
			"welcomeName":            neutralOwnerName,
			"welcomeDescription":     neutralDescription,
			"welcomeUseAdmin":        true,
			"pwaEnabled":             true,
			"pwaTitle":               "",
			"pwaDescription":         neutralPwaDescription,
			"pwaIconURL":             "",
			"defaultContentTheme":    "light",
			"homeLayoutDefault":      "three",
			"announcementText":       neutralAnnouncement,
			"announcementEnabled":    true,
			"musicEnabled":           false,
			"musicPlaylistId":        "",
			"musicSongId":            "",
			"musicPosition":          "bottom-left",
			"musicTheme":             "auto",
			"musicLyric":             true,
			"musicAutoplay":          false,
			"musicDefaultMinimized":  true,
			"musicEmbed":             false,
			"musicHideOnMobile":      true,
			"musicCssCdnURL":         "",
			"musicJsCdnURL":          "",
			"commentEnabled":         true,
			"hitokotoEnabled":        true,
			"lifeCountdownEnabled":   false,
			"lifeCountdownBirthDate": "",
			"lifeExpectancyYears":    80,
			// 广告默认参数（多广告位）
			"leftAdEnabled": true,
			"leftAds": []map[string]string{
				{"imageURL": "https://picsum.photos/seed/ad-1/640/640", "linkURL": "", "description": "写作与记录", "textColor": defaultAdTextColor, "textDisplayMode": defaultAdTextDisplayMode},
				{"imageURL": "https://picsum.photos/seed/ad-2/640/640", "linkURL": "", "description": "探索新主题与小工具", "textColor": defaultAdTextColor, "textDisplayMode": defaultAdTextDisplayMode},
				{"imageURL": "https://picsum.photos/seed/ad-3/640/640", "linkURL": "", "description": "记录日常内容", "textColor": defaultAdTextColor, "textDisplayMode": defaultAdTextDisplayMode},
			},
			"leftAdsIntervalMs": 4000,
			// 社交链接默认
			"socialLinksEnabled": true,
			"socialLinks":        []map[string]string{},
		},
		"voceChatConfig": vocechat.DefaultPublicConfig(),
		"storageEnabled": false,
		"storageConfig": map[string]interface{}{
			"provider":           "",
			"endpoint":           "",
			"region":             "",
			"bucket":             "",
			"accessKey":          "",
			"secretKey":          "",
			"usePathStyle":       true,
			"publicBaseURL":      "",
			"syncRole":           "primary",
			"autoSyncEnabled":    false,
			"syncMode":           "instant",
			"syncIntervalMinute": 15,
		},
		"attachmentStorageEnabled":       false,
		"recycleBinRetentionDays":        0,
		"commentRecycleBinRetentionDays": 0,
		"notifyNoteDeletionByPrimary":    false,
		"notifyCommentDeletionByPrimary": false,
		"attachmentStorageConfig": map[string]interface{}{
			"provider":          "",
			"endpoint":          "",
			"region":            "",
			"bucket":            "",
			"accessKey":         "",
			"secretKey":         "",
			"usePathStyle":      true,
			"publicBaseURL":     "",
			"enableCompression": false,
		},
	}
}

// 选择第一个非空字符串
func choose(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func pickAny(m map[string]interface{}, keys ...string) interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return ""
}

func normalizeFeedSources(raw interface{}) []map[string]interface{} {
	list := []map[string]interface{}{}
	switch arr := raw.(type) {
	case []map[string]interface{}:
		for _, m := range arr {
			itemType := normalizeFeedSourceTypeRaw(pickAny(m, "type", "Type"))
			item := map[string]interface{}{
				"type":    itemType,
				"group":   strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "group", "Group"))),
				"name":    strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "name", "Name"))),
				"url":     strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "url", "URL"))),
				"enabled": parseBoolLike(pickAny(m, "enabled", "Enabled"), true),
				"visible": parseBoolLike(pickAny(m, "visible", "Visible"), true),
			}
			if strings.TrimSpace(fmt.Sprintf("%v", item["url"])) == "" {
				continue
			}
			if strings.TrimSpace(fmt.Sprintf("%v", item["group"])) == "" {
				item["group"] = "默认分组"
			}
			if itemType == "" {
				item["type"] = "rss"
			}
			list = append(list, item)
		}
	case []interface{}:
		for _, it := range arr {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			itemType := normalizeFeedSourceTypeRaw(pickAny(m, "type", "Type"))
			item := map[string]interface{}{
				"type":    itemType,
				"group":   strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "group", "Group"))),
				"name":    strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "name", "Name"))),
				"url":     strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "url", "URL"))),
				"enabled": parseBoolLike(pickAny(m, "enabled", "Enabled"), true),
				"visible": parseBoolLike(pickAny(m, "visible", "Visible"), true),
			}
			if strings.TrimSpace(fmt.Sprintf("%v", item["url"])) == "" {
				continue
			}
			if strings.TrimSpace(fmt.Sprintf("%v", item["group"])) == "" {
				item["group"] = "默认分组"
			}
			if itemType == "" {
				item["type"] = "rss"
			}
			list = append(list, item)
		}
	}
	return list
}

func normalizeFeedSourceTypeRaw(raw interface{}) string {
	candidate := raw
	if obj, ok := raw.(map[string]interface{}); ok {
		candidate = pickAny(obj, "value", "type", "label")
	}
	t := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", candidate)))
	switch t {
	case "rss":
		return "rss"
	case "note", "custom", "说说笔记", "本项目api", "本项目 api":
		return "note"
	case "ech0":
		return "ech0"
	case "memos":
		return "memos"
	case "mastodon":
		return "mastodon"
	default:
		return "rss"
	}
}

func parseBoolLike(v interface{}, def bool) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		if s == "true" || s == "1" || s == "yes" || s == "on" {
			return true
		}
		if s == "false" || s == "0" || s == "no" || s == "off" {
			return false
		}
	case float64:
		return int(x) == 1
	case int:
		return x == 1
	}
	return def
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
