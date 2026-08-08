package controllers

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rcy1314/echo-noise/config"
	attachmentregistry "github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
)

type AttachmentInfo struct {
	Key            string       `json:"key"`
	LogicalID      string       `json:"logical_id,omitempty"`
	GroupID        string       `json:"group_id,omitempty"`
	Name           string       `json:"name"`
	URL            string       `json:"url"`
	Size           int64        `json:"size"`
	ModifiedAt     time.Time    `json:"modified_at"`
	Belongs        []BelongItem `json:"belongs"`
	ReferenceCount int64        `json:"reference_count,omitempty"`
}

type BelongItem struct {
	ID              uint      `json:"id"`
	CreatedAt       time.Time `json:"created_at"`
	Snippet         string    `json:"snippet"`
	Kind            string    `json:"kind,omitempty"`
	Label           string    `json:"label,omitempty"`
	SourceType      string    `json:"source_type,omitempty"`
	SourceID        uint      `json:"source_id,omitempty"`
	MessageID       uint      `json:"message_id,omitempty"`
	CommentID       *uint     `json:"comment_id,omitempty"`
	ParentCommentID *uint     `json:"parent_comment_id,omitempty"`
	OwnerUserID     uint      `json:"owner_user_id,omitempty"`
	Visibility      string    `json:"visibility,omitempty"`
}

type imageAttachmentUsage struct {
	URL       string
	Kind      string
	Label     string
	ID        uint
	CreatedAt time.Time
}

type AttachmentZipRequest struct {
	Items []AttachmentZipItem `json:"items"`
}

type AttachmentZipItem struct {
	Type      string `json:"type"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	LogicalID string `json:"logical_id"`
}

func escapeObjectKeyForURL(key string) string {
	s := strings.TrimLeft(key, "/")
	if s == "" {
		return ""
	}
	return strings.ReplaceAll(url.PathEscape(s), "%2F", "/")
}

func splitPublicBaseURL(raw string) (string, string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ""
	}
	s = strings.TrimRight(s, "/")
	if strings.HasPrefix(s, "//") {
		s = "https:" + s
	}
	parseStr := s
	if !strings.Contains(parseStr, "://") {
		parseStr = "https://" + strings.TrimLeft(parseStr, "/")
	}
	u, err := url.Parse(parseStr)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return s, ""
	}
	origin := strings.TrimRight(u.Scheme+"://"+u.Host, "/")
	prefix := strings.Trim(u.Path, "/")
	return origin, prefix
}

func loadImageAttachmentUsages() []imageAttachmentUsage {
	usages := make([]imageAttachmentUsage, 0)
	var users []models.User
	if err := database.DB.Select("id", "username", "avatar_url").Where("avatar_url <> ?", "").Find(&users).Error; err == nil {
		for _, user := range users {
			if value := strings.TrimSpace(user.AvatarURL); value != "" {
				usages = append(usages, imageAttachmentUsage{URL: value, Kind: "user_avatar", Label: "用户头像：" + user.Username, ID: user.ID})
			}
		}
	}
	return append(usages, loadPublicSiteImageAttachmentUsages()...)
}

func loadPublicSiteImageAttachmentUsages() []imageAttachmentUsage {
	usages := make([]imageAttachmentUsage, 0)
	var site models.SiteConfig
	if err := database.DB.Table("site_configs").First(&site).Error; err != nil {
		return usages
	}
	appendSiteUsage := func(raw, kind, label string) {
		if value := strings.TrimSpace(raw); value != "" {
			usages = append(usages, imageAttachmentUsage{URL: value, Kind: kind, Label: label, CreatedAt: site.UpdatedAt})
		}
	}
	appendSiteUsage(site.AvatarURL, "site_avatar", "站点头像")
	appendSiteUsage(site.WelcomeAvatarURL, "welcome_avatar", "欢迎组件头像")
	appendSiteUsage(site.RSSFaviconURL, "rss_icon", "RSS 图标")
	appendSiteUsage(site.PwaIconURL, "pwa_icon", "PWA 图标")
	for index, background := range site.GetBackgroundsConfig() {
		appendSiteUsage(background.URL, "header_background", fmt.Sprintf("头部图 #%d", index+1))
	}
	var ads []map[string]interface{}
	if json.Unmarshal([]byte(site.LeftAds), &ads) == nil {
		for index, ad := range ads {
			imageURL, _ := ad["imageURL"].(string)
			appendSiteUsage(imageURL, "advertisement", fmt.Sprintf("广告 #%d", index+1))
		}
	}
	return usages
}

func appendImageUsageBelongs(belongs []BelongItem, usages []imageAttachmentUsage, needles ...string) []BelongItem {
	for _, usage := range usages {
		matched := false
		for _, needle := range needles {
			if needle != "" && strings.Contains(usage.URL, needle) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		belongs = append(belongs, BelongItem{
			ID:        usage.ID,
			CreatedAt: usage.CreatedAt,
			Snippet:   usage.Label,
			Kind:      usage.Kind,
			Label:     usage.Label,
		})
	}
	return belongs
}

func ListImageAttachments(c *gin.Context) {
	imageUsages := loadImageAttachmentUsages()
	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		viewerID, _ := currentMessageViewer(c)
		list, err := listCloudAttachments(siteCfg, viewerID, func(name string) bool {
			return isImageExt(name)
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}

	dir := localImageDir()

	var messages []models.Message
	database.DB.Select("id", "content", "image_url", "user_id", "created_at").Order("created_at DESC").Find(&messages)

	viewerID, _ := currentMessageViewer(c)
	list, err := listRegisteredAttachmentsForViewer("image", "local", viewerID, messages, imageUsages)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		p := filepath.Join(dir, name)
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		urlPath := "/api/images/" + url.PathEscape(name)
		belongs, visible := legacyAttachmentBelongsForViewer(viewerID, "image", name, messages)
		if !visible {
			continue
		}
		belongs = appendImageUsageBelongs(belongs, imageUsages, "/images/"+name, "/api/images/"+name, "/images/"+url.PathEscape(name), "/api/images/"+url.PathEscape(name))
		list = append(list, AttachmentInfo{Key: name, Name: name, URL: urlPath, Size: fi.Size(), ModifiedAt: fi.ModTime(), Belongs: belongs})
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
}

func listRegisteredAttachments(kind, backend string, messages []models.Message, suppliedImageUsages ...[]imageAttachmentUsage) ([]AttachmentInfo, error) {
	return listRegisteredAttachmentsForViewer(kind, backend, nil, messages, suppliedImageUsages...)
}

func listRegisteredAttachmentsForViewer(kind, backend string, actorID *uint, messages []models.Message, suppliedImageUsages ...[]imageAttachmentUsage) ([]AttachmentInfo, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	resolved, err := attachmentregistry.NewRegistry(db).List(kind, backend)
	if err != nil {
		return nil, err
	}
	out := make([]AttachmentInfo, 0, len(resolved))
	var imageUsages []imageAttachmentUsage
	if kind == "image" {
		if len(suppliedImageUsages) > 0 {
			imageUsages = suppliedImageUsages[0]
		} else {
			imageUsages = loadImageAttachmentUsages()
		}
	}
	for _, item := range resolved {
		visibleSources, err := services.VisibleAttachmentSources(db, actorID, item.Reference, backend)
		if err != nil {
			return nil, err
		}
		if len(visibleSources) == 0 {
			if actorID == nil {
				needle := attachmentReferenceURLPrefix(item.Reference.Kind, backend, item.Reference.PublicID)
				for _, message := range messages {
					if message.UserID == item.Reference.OwnerUserID && (strings.Contains(message.Content, needle) || strings.Contains(message.ImageURL, needle)) {
						visibleSources = append(visibleSources, services.AttachmentSource{SourceType: "message", SourceID: message.ID, MessageID: message.ID, OwnerUserID: message.UserID, Message: message})
					}
				}
			}
			// Unreferenced logical uploads remain visible only to their owner or
			// the primary administrator; hidden references are never inferred.
			if actorID != nil && *actorID != models.PrimaryAdminUserID && *actorID != item.Reference.OwnerUserID {
				continue
			}
		}
		needle := attachmentReferenceURLPrefix(item.Reference.Kind, backend, item.Reference.PublicID)
		belongs := make([]BelongItem, 0)
		for _, source := range visibleSources {
			belongs = append(belongs, belongItemFromAttachmentSource(source))
		}
		belongs = appendImageUsageBelongs(belongs, imageUsages, needle)
		out = append(out, AttachmentInfo{
			Key:            item.Reference.PublicID,
			LogicalID:      item.Reference.PublicID,
			GroupID:        attachmentGroupID(item.Blob),
			Name:           item.Reference.OriginalName,
			URL:            attachmentregistry.ReferenceURL(item.Reference, backend),
			Size:           item.Blob.Size,
			ModifiedAt:     item.Reference.CreatedAt,
			Belongs:        belongs,
			ReferenceCount: visibleReferenceCount(db, actorID, item.Blob.ID, backend),
		})
	}
	return out, nil
}

func visibleReferenceCount(db *gorm.DB, actorID *uint, blobID uint, backend string) int64 {
	var refs []models.AttachmentReference
	if db == nil || db.Where("blob_id = ?", blobID).Find(&refs).Error != nil {
		return 0
	}
	var count int64
	for _, ref := range refs {
		if actorID == nil {
			count++
			continue
		}
		sources, err := services.VisibleAttachmentSources(db, actorID, ref, backend)
		if err != nil {
			continue
		}
		if len(sources) > 0 || actorID == nil || *actorID == models.PrimaryAdminUserID || *actorID == ref.OwnerUserID {
			count++
		}
	}
	return count
}

func ListVideoAttachments(c *gin.Context) {
	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		viewerID, _ := currentMessageViewer(c)
		list, err := listCloudAttachments(siteCfg, viewerID, func(name string) bool {
			return isVideoExt(name)
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	dir := pickDir([]string{
		"./data/video",
		filepath.Join(wd, "data/video"),
		filepath.Join(exeDir, "data/video"),
		"/data/video",
		"/app/data/video",
	}, "./data/video")
	var messages []models.Message
	database.DB.Select("id", "content", "image_url", "user_id", "created_at").Order("created_at DESC").Find(&messages)
	viewerID, _ := currentMessageViewer(c)
	list, err := listRegisteredAttachmentsForViewer("video", "local", viewerID, messages)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		p := filepath.Join(dir, name)
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		urlPath := "/video/" + url.PathEscape(name)
		belongs, visible := legacyAttachmentBelongsForViewer(viewerID, "video", name, messages)
		if !visible {
			continue
		}
		list = append(list, AttachmentInfo{Key: name, Name: name, URL: urlPath, Size: fi.Size(), ModifiedAt: fi.ModTime(), Belongs: belongs})
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
}

func ListAudioAttachments(c *gin.Context) {
	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		viewerID, _ := currentMessageViewer(c)
		list, err := listCloudAttachments(siteCfg, viewerID, func(name string) bool {
			return isAudioExt(name)
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	dir := pickDir([]string{
		"./data/audio",
		filepath.Join(wd, "data/audio"),
		filepath.Join(exeDir, "data/audio"),
		"/data/audio",
		"/app/data/audio",
	}, "./data/audio")
	var messages []models.Message
	database.DB.Select("id", "content", "image_url", "user_id", "created_at").Order("created_at DESC").Find(&messages)
	viewerID, _ := currentMessageViewer(c)
	list, err := listRegisteredAttachmentsForViewer("audio", "local", viewerID, messages)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		p := filepath.Join(dir, name)
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		urlPath := "/api/audio/" + url.PathEscape(name)
		belongs, visible := legacyAttachmentBelongsForViewer(viewerID, "audio", name, messages)
		if !visible {
			continue
		}
		list = append(list, AttachmentInfo{Key: name, Name: name, URL: urlPath, Size: fi.Size(), ModifiedAt: fi.ModTime(), Belongs: belongs})
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
}

func ListOtherAttachments(c *gin.Context) {
	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		viewerID, _ := currentMessageViewer(c)
		list, err := listCloudAttachments(siteCfg, viewerID, func(name string) bool {
			return !isImageExt(name) && !isVideoExt(name) && !isAudioExt(name)
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	dir := pickDir([]string{
		"./data/attachments",
		filepath.Join(wd, "data/attachments"),
		filepath.Join(exeDir, "data/attachments"),
		"/data/attachments",
		"/app/data/attachments",
	}, "./data/attachments")
	var messages []models.Message
	database.DB.Select("id", "content", "image_url", "user_id", "created_at").Order("created_at DESC").Find(&messages)
	viewerID, _ := currentMessageViewer(c)
	list, err := listRegisteredAttachmentsForViewer("file", "local", viewerID, messages)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		p := filepath.Join(dir, name)
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		urlPath := "/api/files/" + url.PathEscape(name)
		belongs, visible := legacyAttachmentBelongsForViewer(viewerID, "file", name, messages)
		if !visible {
			continue
		}
		list = append(list, AttachmentInfo{Key: name, Name: name, URL: urlPath, Size: fi.Size(), ModifiedAt: fi.ModTime(), Belongs: belongs})
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
}

func findBelongs(messages []models.Message, name string, prefixes ...string) []BelongItem {
	var out []BelongItem
	encodedName := url.PathEscape(name)
	needles := make([]string, 0, len(prefixes)*2)
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		needles = append(needles, prefix+name, prefix+encodedName)
	}

	for _, m := range messages {
		has := false
		for _, needle := range needles {
			if strings.Contains(m.Content, needle) || strings.Contains(m.ImageURL, needle) {
				has = true
				break
			}
		}
		if has {
			snip := m.Content
			if len(snip) > 80 {
				snip = snip[:80]
			}
			out = append(out, BelongItem{ID: m.ID, CreatedAt: m.CreatedAt, Snippet: snip, Kind: "message", Label: fmt.Sprintf("笔记 #%d", m.ID), SourceType: "message", SourceID: m.ID, MessageID: m.ID, OwnerUserID: m.UserID, Visibility: services.StoredMessageVisibility(m)})
		}
	}
	return out
}

func legacyAttachmentBelongsForViewer(actorID *uint, kind, name string, messages []models.Message) ([]BelongItem, bool) {
	db, err := database.GetDB()
	if err != nil {
		return nil, false
	}
	sources, err := services.VisibleLegacyAttachmentSources(db, actorID, kind, name)
	if err != nil {
		return nil, false
	}
	if actorID != nil && len(sources) == 0 {
		return nil, false
	}
	if actorID == nil {
		return findBelongs(messages, name, "/images/", "/api/images/", "/video/", "/api/video/", "/audio/", "/api/audio/", "/files/", "/api/files/", "/attachments/", "/api/attachments/"), true
	}
	belongs := make([]BelongItem, 0, len(sources))
	for _, source := range sources {
		belongs = append(belongs, belongItemFromAttachmentSource(source))
	}
	return belongs, true
}

func belongItemFromAttachmentSource(source services.AttachmentSource) BelongItem {
	snippet := source.Message.Content
	if source.Comment != nil {
		snippet = source.Comment.Content
	}
	if len(snippet) > 80 {
		snippet = snippet[:80]
	}
	label := fmt.Sprintf("笔记 #%d", source.MessageID)
	if source.SourceType == "comment" {
		label = fmt.Sprintf("评论 #%d", source.SourceID)
	}
	if source.SourceType == "reply" {
		label = fmt.Sprintf("回复 #%d", source.SourceID)
	}
	if source.SourceType == "guestbook" {
		label = fmt.Sprintf("留言 #%d", source.SourceID)
	}
	return BelongItem{
		ID: source.SourceID, CreatedAt: source.Message.CreatedAt, Snippet: snippet,
		Kind: source.SourceType, Label: label, SourceType: source.SourceType,
		SourceID: source.SourceID, MessageID: source.MessageID, CommentID: source.CommentID,
		ParentCommentID: source.ParentCommentID, OwnerUserID: source.OwnerUserID,
		Visibility: source.Visibility,
	}
}

func DeleteImageAttachment(c *gin.Context) {
	name := c.Param("name")
	base := filepath.Base(name)
	if !legacyAttachmentVisibleToActor(c, "image", base) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}

	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		decoded, err := url.PathUnescape(name)
		if err != nil {
			decoded = name
		}
		if err := deleteCloudAttachment(siteCfg, decoded); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
		return
	}

	imgDir := localImageDir()

	p := filepath.Join(imgDir, base)
	if _, err := os.Stat(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}
	if err := os.Remove(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
		return
	}
	if err := deleteLocalAttachmentGrants("image", base); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件已删除，但授权记录清理失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
}

func DeleteVideoAttachment(c *gin.Context) {
	name := c.Param("name")
	base := filepath.Base(name)
	if !legacyAttachmentVisibleToActor(c, "video", base) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}

	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		decoded, err := url.PathUnescape(name)
		if err != nil {
			decoded = name
		}
		if err := deleteCloudAttachment(siteCfg, decoded); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	vidDir := pickDir([]string{
		"./data/video",
		filepath.Join(wd, "data/video"),
		filepath.Join(exeDir, "data/video"),
		"/data/video",
		"/app/data/video",
	}, "./data/video")
	p := filepath.Join(vidDir, base)
	if _, err := os.Stat(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}
	if err := os.Remove(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
		return
	}
	if err := deleteLocalAttachmentGrants("video", base); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件已删除，但授权记录清理失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
}

func DeleteAudioAttachment(c *gin.Context) {
	name := c.Param("name")
	base := filepath.Base(name)
	if !legacyAttachmentVisibleToActor(c, "audio", base) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}

	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		decoded, err := url.PathUnescape(name)
		if err != nil {
			decoded = name
		}
		if err := deleteCloudAttachment(siteCfg, decoded); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	audioDir := pickDir([]string{
		"./data/audio",
		filepath.Join(wd, "data/audio"),
		filepath.Join(exeDir, "data/audio"),
		"/data/audio",
		"/app/data/audio",
	}, "./data/audio")
	p := filepath.Join(audioDir, base)
	if _, err := os.Stat(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}
	if err := os.Remove(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
		return
	}
	if err := deleteLocalAttachmentGrants("audio", base); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件已删除，但授权记录清理失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
}

func DeleteOtherAttachment(c *gin.Context) {
	name := c.Param("name")
	base := filepath.Base(name)
	if !legacyAttachmentVisibleToActor(c, "file", base) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}

	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		decoded, err := url.PathUnescape(name)
		if err != nil {
			decoded = name
		}
		if err := deleteCloudAttachment(siteCfg, decoded); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	attachmentDir := pickDir([]string{
		"./data/attachments",
		filepath.Join(wd, "data/attachments"),
		filepath.Join(exeDir, "data/attachments"),
		"/data/attachments",
		"/app/data/attachments",
	}, "./data/attachments")
	p := filepath.Join(attachmentDir, base)
	if _, err := os.Stat(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}
	if err := os.Remove(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
		return
	}
	if err := deleteLocalAttachmentGrants("file", base); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件已删除，但授权记录清理失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
}

func deleteLocalAttachmentGrants(kind string, name string) error {
	return database.DB.Where("kind = ? AND name = ?", kind, name).Delete(&models.LocalAttachmentGrant{}).Error
}

func DeleteAttachmentReference(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("id"))
	db, err := database.GetDB()
	if err != nil || publicID == "" {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "附件不存在"})
		return
	}
	registry := attachmentregistry.NewRegistry(db)
	resolved, err := registry.Resolve(publicID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "附件不存在"})
		return
	}
	if !attachmentReferenceVisibleToActor(c, resolved.Reference, resolved.Blob.StorageBackend) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "附件不存在"})
		return
	}
	store, err := attachmentBlobStore(db, resolved.Blob.StorageBackend)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	if err := registry.DeleteReference(c.Request.Context(), store, publicID); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
}

func attachmentReferenceVisibleToActor(c *gin.Context, reference models.AttachmentReference, backend string) bool {
	db, err := database.GetDB()
	if err != nil {
		return false
	}
	actorID, _ := currentMessageViewer(c)
	if actorID == nil {
		return true // direct controller tests/legacy callers; authenticated routes always set an actor
	}
	sources, err := services.VisibleAttachmentSources(db, actorID, reference, backend)
	if err != nil {
		return false
	}
	if len(sources) > 0 {
		return true
	}
	return actorID != nil && (*actorID == models.PrimaryAdminUserID || *actorID == reference.OwnerUserID)
}

func legacyAttachmentVisibleToActor(c *gin.Context, kind, name string) bool {
	actorID, _ := currentMessageViewer(c)
	if actorID == nil {
		return true
	}
	db, err := database.GetDB()
	if err != nil {
		return false
	}
	sources, err := services.VisibleLegacyAttachmentSources(db, actorID, kind, name)
	return err == nil && len(sources) > 0
}

func visibleReferenceIDsForActor(c *gin.Context, refs []models.AttachmentReference, backend string) []uint {
	ids := make([]uint, 0, len(refs))
	for _, ref := range refs {
		if attachmentReferenceVisibleToActor(c, ref, backend) {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}

// attachmentGroupID identifies the physical content behind a logical
// attachment without leaking the raw content hash to the browser. References
// sharing one blob share this id, which is what lets the admin UI fold
// duplicates into a single card instead of scattering them across the list.
func attachmentGroupID(blob models.AttachmentBlob) string {
	if strings.TrimSpace(blob.ContentHash) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("attachment-group\x00" + blob.StorageBackend + "\x00" + blob.ContentHash))
	return hex.EncodeToString(sum[:])[:32]
}

// attachmentBlobStore resolves the storage seam for a blob backend so the
// single, batch, and purge delete paths cannot drift apart.
func attachmentBlobStore(db *gorm.DB, backend string) (attachmentregistry.BlobStore, error) {
	switch backend {
	case "local":
		return attachmentregistry.NewLocalStore(attachmentregistry.DefaultLocalRoot()), nil
	case "cloud":
		var siteCfg models.SiteConfig
		if err := db.Table("site_configs").First(&siteCfg).Error; err != nil {
			return nil, errors.New("云存储配置不可用")
		}
		client, bucket, _, err := newAttachmentS3Client(siteCfg)
		if err != nil {
			return nil, errors.New("云存储配置不可用")
		}
		_, prefix := splitPublicBaseURL(siteCfg.AttachmentStoragePublicBaseURL)
		return attachmentregistry.NewS3Store(client, bucket, prefix), nil
	default:
		return nil, errors.New("不支持的附件存储类型")
	}
}

type AttachmentReferenceBatchRequest struct {
	LogicalIDs []string `json:"logical_ids"`
}

type AttachmentReferenceBatchResult struct {
	ReferencesDeleted int      `json:"references_deleted"`
	FilesPurged       int      `json:"files_purged"`
	Failed            int      `json:"failed"`
	Errors            []string `json:"errors,omitempty"`
}

func normalizeAttachmentLogicalIDs(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// DeleteAttachmentReferencesBatch removes several logical references in one
// request. Selections may mix references of the same file with references of
// entirely different files; each id is resolved independently so a partial
// failure never blocks the rest.
func DeleteAttachmentReferencesBatch(c *gin.Context) {
	var req AttachmentReferenceBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "参数错误"})
		return
	}
	logicalIDs := normalizeAttachmentLogicalIDs(req.LogicalIDs)
	if len(logicalIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "未选择附件"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "数据库不可用"})
		return
	}
	registry := attachmentregistry.NewRegistry(db)
	result := AttachmentReferenceBatchResult{}
	for _, publicID := range logicalIDs {
		resolved, err := registry.Resolve(publicID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, publicID+"：附件不存在")
			continue
		}
		if !attachmentReferenceVisibleToActor(c, resolved.Reference, resolved.Blob.StorageBackend) {
			result.Failed++
			result.Errors = append(result.Errors, publicID+"：附件不存在")
			continue
		}
		store, err := attachmentBlobStore(db, resolved.Blob.StorageBackend)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, publicID+"："+err.Error())
			continue
		}
		if err := registry.DeleteReference(c.Request.Context(), store, publicID); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, publicID+"：删除失败")
			continue
		}
		result.ReferencesDeleted++
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": result})
}

// PurgeAttachmentBlobsBatch deletes the physical files behind the selected
// logical references together with every remaining reference to them. Ids that
// resolve to the same blob collapse into one purge, so selecting any subset of
// a shared file is enough to clear it from disk.
func PurgeAttachmentBlobsBatch(c *gin.Context) {
	var req AttachmentReferenceBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "参数错误"})
		return
	}
	logicalIDs := normalizeAttachmentLogicalIDs(req.LogicalIDs)
	if len(logicalIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "未选择附件"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "数据库不可用"})
		return
	}
	registry := attachmentregistry.NewRegistry(db)
	result := AttachmentReferenceBatchResult{}
	purgedBlobs := make(map[uint]struct{}, len(logicalIDs))
	for _, publicID := range logicalIDs {
		resolved, err := registry.Resolve(publicID)
		if err != nil {
			// A sibling purge in this same request already removed it.
			continue
		}
		if _, done := purgedBlobs[resolved.Blob.ID]; done {
			continue
		}
		if !attachmentReferenceVisibleToActor(c, resolved.Reference, resolved.Blob.StorageBackend) {
			result.Failed++
			result.Errors = append(result.Errors, publicID+"：附件不存在")
			continue
		}
		store, err := attachmentBlobStore(db, resolved.Blob.StorageBackend)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, publicID+"："+err.Error())
			continue
		}
		var siblings []models.AttachmentReference
		if err := db.Where("blob_id = ?", resolved.Blob.ID).Find(&siblings).Error; err != nil {
			result.Failed++
			continue
		}
		visibleIDs := visibleReferenceIDsForActor(c, siblings, resolved.Blob.StorageBackend)
		removed, _, err := registry.PurgeBlobScoped(c.Request.Context(), store, publicID, visibleIDs)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, publicID+"：删除失败")
			continue
		}
		purgedBlobs[resolved.Blob.ID] = struct{}{}
		result.FilesPurged++
		result.ReferencesDeleted += removed
	}
	if result.FilesPurged == 0 && result.Failed == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "附件不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": result})
}

func DownloadAttachmentZip(c *gin.Context) {
	var req AttachmentZipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "参数错误"})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "请选择附件"})
		return
	}
	if len(req.Items) > 200 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "单次最多打包 200 个附件"})
		return
	}

	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error

	fileName := fmt.Sprintf("attachments_%s.zip", time.Now().Format("20060102150405"))
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	c.Header("Cache-Control", "no-cache")

	archive := zip.NewWriter(c.Writer)
	defer archive.Close()

	usedNames := map[string]int{}
	viewerID, _ := currentMessageViewer(c)
	for _, item := range req.Items {
		if err := addAttachmentToZip(archive, siteCfg, viewerID, item, usedNames); err != nil {
			continue
		}
	}
}

func pickDir(candidates []string, fallback string) string {
	for _, d := range candidates {
		if d == "" {
			continue
		}
		info, err := os.Stat(d)
		if err == nil && info.IsDir() {
			return d
		}
	}
	return fallback
}

func addAttachmentToZip(archive *zip.Writer, siteCfg models.SiteConfig, viewerID *uint, item AttachmentZipItem, usedNames map[string]int) error {
	if strings.TrimSpace(item.LogicalID) != "" {
		return addRegisteredAttachmentToZip(archive, siteCfg, viewerID, item, usedNames)
	}
	if siteCfg.AttachmentStorageEnabled {
		return addCloudAttachmentToZip(archive, siteCfg, item, usedNames)
	}
	return addLocalAttachmentToZip(archive, item, usedNames)
}

func addRegisteredAttachmentToZip(archive *zip.Writer, siteCfg models.SiteConfig, viewerID *uint, item AttachmentZipItem, usedNames map[string]int) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	resolved, err := attachmentregistry.NewRegistry(db).Resolve(strings.TrimSpace(item.LogicalID))
	if err != nil {
		return err
	}
	if viewerID != nil {
		sources, visibilityErr := services.VisibleAttachmentSources(db, viewerID, resolved.Reference, resolved.Blob.StorageBackend)
		if visibilityErr != nil {
			return visibilityErr
		}
		if len(sources) == 0 && *viewerID != models.PrimaryAdminUserID && *viewerID != resolved.Reference.OwnerUserID {
			return errors.New("attachment not found")
		}
	}
	wantedKind := strings.ToLower(strings.TrimSpace(item.Type))
	if wantedKind == "other" {
		wantedKind = "file"
	}
	if wantedKind != resolved.Reference.Kind {
		return errors.New("attachment type mismatch")
	}
	_, folder, _ := localAttachmentDirForType(item.Type)
	if folder == "" {
		folder = "attachments"
	}
	var reader io.ReadCloser
	if resolved.Blob.StorageBackend == "local" {
		file, _, err := attachmentregistry.NewLocalStore(attachmentregistry.DefaultLocalRoot()).Open(resolved.Blob.StorageKey)
		if err != nil {
			return err
		}
		reader = file
	} else if resolved.Blob.StorageBackend == "cloud" {
		client, bucket, _, err := newAttachmentS3Client(siteCfg)
		if err != nil {
			return err
		}
		obj, err := client.GetObject(context.Background(), &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(resolved.Blob.StorageKey)})
		if err != nil {
			return err
		}
		reader = obj.Body
	} else {
		return errors.New("unsupported attachment backend")
	}
	defer reader.Close()
	zipName := uniqueZipEntryName(filepath.ToSlash(filepath.Join(folder, safeZipEntryBaseName(resolved.Reference.OriginalName))), usedNames)
	writer, err := archive.Create(zipName)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, reader)
	return err
}

func addLocalAttachmentToZip(archive *zip.Writer, item AttachmentZipItem, usedNames map[string]int) error {
	dir, folder, ok := localAttachmentDirForType(item.Type)
	if !ok {
		return errors.New("unsupported attachment type")
	}
	base := filepath.Base(strings.TrimSpace(firstNonEmpty(item.Key, item.Name)))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return errors.New("invalid attachment name")
	}
	path := filepath.Join(dir, base)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zipName := uniqueZipEntryName(filepath.ToSlash(filepath.Join(folder, safeZipEntryBaseName(firstNonEmpty(item.Name, base)))), usedNames)
	writer, err := archive.Create(zipName)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, file)
	return err
}

func addCloudAttachmentToZip(archive *zip.Writer, siteCfg models.SiteConfig, item AttachmentZipItem, usedNames map[string]int) error {
	client, bucket, _, err := newAttachmentS3Client(siteCfg)
	if err != nil {
		return err
	}
	key := strings.TrimLeft(strings.TrimSpace(firstNonEmpty(item.Key, item.Name)), "/")
	if key == "" {
		return errors.New("invalid attachment key")
	}
	obj, err := client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	defer obj.Body.Close()

	_, folder, _ := localAttachmentDirForType(item.Type)
	if folder == "" {
		folder = "attachments"
	}
	zipName := uniqueZipEntryName(filepath.ToSlash(filepath.Join(folder, safeZipEntryBaseName(firstNonEmpty(item.Name, filepath.Base(key))))), usedNames)
	writer, err := archive.Create(zipName)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, obj.Body)
	return err
}

func localAttachmentDirForType(rawType string) (string, string, bool) {
	kind := strings.ToLower(strings.TrimSpace(rawType))
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	switch kind {
	case "image", "images":
		sp := strings.TrimRight(config.Config.Upload.SavePath, "/")
		return pickDir([]string{
			sp,
			"./" + sp,
			filepath.Join(wd, sp),
			filepath.Join(exeDir, sp),
			"./data/images",
			filepath.Join(wd, "data/images"),
			filepath.Join(exeDir, "data/images"),
			"/data/images",
			"/app/data/images",
		}, "./data/images"), "images", true
	case "video", "videos":
		return pickDir([]string{
			"./data/video",
			filepath.Join(wd, "data/video"),
			filepath.Join(exeDir, "data/video"),
			"/data/video",
			"/app/data/video",
		}, "./data/video"), "video", true
	case "audio", "audios":
		return pickDir([]string{
			"./data/audio",
			filepath.Join(wd, "data/audio"),
			filepath.Join(exeDir, "data/audio"),
			"/data/audio",
			"/app/data/audio",
		}, "./data/audio"), "audio", true
	case "other", "others":
		return pickDir([]string{
			"./data/attachments",
			filepath.Join(wd, "data/attachments"),
			filepath.Join(exeDir, "data/attachments"),
			"/data/attachments",
			"/app/data/attachments",
		}, "./data/attachments"), "other", true
	default:
		return "", "", false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func safeZipEntryBaseName(name string) string {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	base = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == 0 || r < 32 || r == 127 || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, base))
	if base == "" || base == "." {
		return "attachment"
	}
	return base
}

func uniqueZipEntryName(name string, used map[string]int) string {
	if name == "" {
		name = "attachment"
	}
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	used[name]++
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s(%d)%s", stem, used[name], ext)
}

func normalizePublicBaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.TrimRight(s, "/")
	if strings.HasPrefix(s, "//") {
		s = "https:" + s
	}
	parseStr := s
	if !strings.Contains(parseStr, "://") {
		parseStr = "https://" + strings.TrimLeft(parseStr, "/")
	}
	u, err := url.Parse(parseStr)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return s
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "/" {
		path = ""
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+path, "/")
}

func newAttachmentS3Client(cfg models.SiteConfig) (*s3.Client, string, string, error) {
	if strings.TrimSpace(cfg.AttachmentStorageBucket) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageAccessKey) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageSecretKey) == "" {
		return nil, "", "", errors.New("附件云存储配置不完整")
	}

	region := strings.TrimSpace(cfg.AttachmentStorageRegion)
	if cfg.AttachmentStorageProvider == "r2" {
		region = "auto"
	}
	if region == "" {
		region = "auto"
	}

	endpoint := strings.TrimSpace(cfg.AttachmentStorageEndpoint)
	if endpoint != "" {
		if u, err := url.Parse(endpoint); err == nil {
			base := u.Scheme + "://" + u.Host
			endpoint = strings.TrimRight(base, "/")
		}
	}

	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if endpoint == "" {
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		}
		return aws.Endpoint{
			URL:               endpoint,
			SigningRegion:     region,
			HostnameImmutable: true,
		}, nil
	})

	creds := credentials.NewStaticCredentialsProvider(cfg.AttachmentStorageAccessKey, cfg.AttachmentStorageSecretKey, "")
	awsConfig, err := awscfg.LoadDefaultConfig(context.Background(),
		awscfg.WithCredentialsProvider(creds),
		awscfg.WithEndpointResolverWithOptions(r2Resolver),
		awscfg.WithRegion(region),
	)
	if err != nil {
		return nil, "", "", err
	}

	client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		o.UsePathStyle = cfg.AttachmentStorageUsePathStyle
	})
	return client, cfg.AttachmentStorageBucket, normalizePublicBaseURL(cfg.AttachmentStoragePublicBaseURL), nil
}

func isImageExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func isVideoExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".webm", ".mov", ".avi":
		return true
	default:
		return false
	}
}

func isAudioExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".webm", ".ogg", ".mp3", ".m4a", ".wav", ".flac":
		return true
	default:
		return false
	}
}

func listCloudAttachments(siteCfg models.SiteConfig, actorID *uint, keep func(name string) bool) ([]AttachmentInfo, error) {
	cli, bucket, publicBaseURL, err := newAttachmentS3Client(siteCfg)
	if err != nil {
		return nil, err
	}
	origin, prefix := splitPublicBaseURL(publicBaseURL)

	var messages []models.Message
	database.DB.Select("id", "content", "image_url", "user_id", "created_at").Order("created_at DESC").Find(&messages)
	imageUsages := loadImageAttachmentUsages()

	var out []AttachmentInfo
	for _, kind := range []string{"image", "video", "audio", "file"} {
		registered, err := listRegisteredAttachmentsForViewer(kind, "cloud", actorID, messages, imageUsages)
		if err != nil {
			return nil, err
		}
		for _, item := range registered {
			if keep(item.Name) {
				out = append(out, item)
			}
		}
	}
	var registeredBlobs []models.AttachmentBlob
	if err := database.DB.Where("storage_backend = ?", "cloud").Find(&registeredBlobs).Error; err != nil {
		return nil, err
	}
	registeredKeys := make(map[string]struct{}, len(registeredBlobs))
	for _, blob := range registeredBlobs {
		registeredKeys[strings.TrimLeft(blob.StorageKey, "/")] = struct{}{}
	}
	var token *string
	for {
		resp, err := cli.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			ContinuationToken: token,
			MaxKeys:           aws.Int32(1000),
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range resp.Contents {
			key := aws.ToString(obj.Key)
			if key == "" {
				continue
			}
			cleanKey := strings.TrimLeft(key, "/")
			if _, registered := registeredKeys[cleanKey]; registered {
				continue
			}
			name := filepath.Base(cleanKey)
			if !keep(name) {
				continue
			}
			// 兼容历史对象：PublicBaseURL 可能带有 path 前缀（如 /note），但对象 key 未必包含该前缀。
			// 如果 key 不带 prefix，则在生成展示 URL 时补齐 prefix；但 Key 字段仍返回真实对象 key（用于删除）。
			record, err := ensureCloudAttachmentObject(cleanKey, name, "")
			if err != nil {
				return nil, err
			}
			visibleSources, sourceErr := services.VisibleLegacyAttachmentSources(database.DB, actorID, "cloud", record.PublicID)
			if sourceErr != nil {
				return nil, sourceErr
			}
			if actorID != nil && len(visibleSources) == 0 {
				continue
			}
			urlPath := "/api/cloud-attachments/" + record.PublicID + "/" + url.PathEscape(name)
			modAt := time.Time{}
			if obj.LastModified != nil {
				modAt = *obj.LastModified
			}
			belongs := findBelongsCloud(messages, cleanKey, origin, prefix, record.PublicID)
			if actorID != nil {
				belongs = make([]BelongItem, 0, len(visibleSources))
				for _, source := range visibleSources {
					belongs = append(belongs, belongItemFromAttachmentSource(source))
				}
			}
			belongs = appendImageUsageBelongs(belongs, imageUsages,
				"/api/cloud-attachments/"+record.PublicID+"/",
				origin+"/"+escapeObjectKeyForURL(cleanKey),
				"/"+cleanKey,
				"/"+url.PathEscape(cleanKey),
			)
			out = append(out, AttachmentInfo{
				Key:        cleanKey,
				Name:       name,
				URL:        urlPath,
				Size:       aws.ToInt64(obj.Size),
				ModifiedAt: modAt,
				Belongs:    belongs,
			})
		}
		if aws.ToBool(resp.IsTruncated) && resp.NextContinuationToken != nil && aws.ToString(resp.NextContinuationToken) != "" {
			token = resp.NextContinuationToken
			continue
		}
		break
	}
	return out, nil
}

func findBelongsCloud(messages []models.Message, key string, origin string, prefix string, publicID string) []BelongItem {
	var out []BelongItem
	cleanKey := strings.TrimLeft(key, "/")
	url1 := origin + "/" + escapeObjectKeyForURL(cleanKey)
	url2 := ""
	if prefix != "" && !strings.HasPrefix(cleanKey, prefix+"/") {
		url2 = origin + "/" + escapeObjectKeyForURL(prefix+"/"+cleanKey)
	}
	needle3 := "/" + cleanKey
	needle4 := "/" + url.PathEscape(cleanKey)
	proxyNeedle := "/api/cloud-attachments/" + publicID + "/"

	for _, m := range messages {
		has := false
		if strings.Contains(m.Content, proxyNeedle) || strings.Contains(m.Content, url1) || (url2 != "" && strings.Contains(m.Content, url2)) ||
			strings.Contains(m.Content, needle3) || strings.Contains(m.Content, needle4) {
			has = true
		}
		if !has {
			if strings.Contains(m.ImageURL, proxyNeedle) || strings.Contains(m.ImageURL, url1) || (url2 != "" && strings.Contains(m.ImageURL, url2)) ||
				strings.Contains(m.ImageURL, needle3) || strings.Contains(m.ImageURL, needle4) {
				has = true
			}
		}
		if has {
			snip := m.Content
			if len(snip) > 80 {
				snip = snip[:80]
			}
			out = append(out, BelongItem{ID: m.ID, CreatedAt: m.CreatedAt, Snippet: snip, Kind: "message", Label: fmt.Sprintf("笔记 #%d", m.ID), SourceType: "message", SourceID: m.ID, MessageID: m.ID, OwnerUserID: m.UserID, Visibility: services.StoredMessageVisibility(m)})
		}
	}
	return out
}

func ensureCloudAttachmentObject(key string, originalName string, contentType string) (models.CloudAttachmentObject, error) {
	var object models.CloudAttachmentObject
	if err := database.DB.Where("object_key = ?", key).First(&object).Error; err == nil && object.ID != 0 {
		return object, nil
	}
	object = models.CloudAttachmentObject{
		PublicID:     uuid.NewString(),
		ObjectKey:    key,
		OriginalName: originalName,
		ContentType:  contentType,
	}
	if err := database.DB.Create(&object).Error; err != nil {
		return models.CloudAttachmentObject{}, err
	}
	return object, nil
}

func deleteCloudAttachment(siteCfg models.SiteConfig, key string) error {
	cli, bucket, _, err := newAttachmentS3Client(siteCfg)
	if err != nil {
		return err
	}
	cleanKey := strings.TrimLeft(key, "/")
	var mappings []models.CloudAttachmentObject
	if err := database.DB.Where("object_key = ?", cleanKey).Find(&mappings).Error; err != nil {
		return err
	}
	_, err = cli.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(cleanKey),
	})
	if err == nil {
		err = database.DB.Transaction(func(tx *gorm.DB) error {
			publicIDs := make([]string, 0, len(mappings))
			for _, mapping := range mappings {
				publicIDs = append(publicIDs, mapping.PublicID)
			}
			if len(publicIDs) > 0 {
				if err := tx.Where("kind = ? AND name IN ?", "cloud", publicIDs).Delete(&models.LocalAttachmentGrant{}).Error; err != nil {
					return err
				}
			}
			return tx.Where("object_key = ?", cleanKey).Delete(&models.CloudAttachmentObject{}).Error
		})
	}
	return err
}
