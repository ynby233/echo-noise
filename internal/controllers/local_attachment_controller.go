package controllers

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	attachmentregistry "github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

// ServeLocalAttachment serves uploaded files while enforcing the visibility of
// every message that references the file. Unreferenced files remain available
// for upload previews, avatars, backgrounds, and other pre-existing site assets.
func ServeLocalAttachment(kind string, root string) gin.HandlerFunc {
	return serveLocalAttachment(kind, root, attachmentregistry.DefaultLocalRoot())
}

func serveLocalAttachment(kind string, root string, blobRoot string) gin.HandlerFunc {
	rootAbs, _ := filepath.Abs(root)
	return func(c *gin.Context) {
		rawName := strings.TrimPrefix(c.Param("name"), "/")
		if strings.HasPrefix(rawName, "refs/") {
			serveLocalAttachmentReference(c, kind, blobRoot, rawName)
			return
		}
		name, err := url.PathUnescape(rawName)
		if err != nil || name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
			c.Status(http.StatusNotFound)
			return
		}
		filePath := filepath.Join(rootAbs, name)
		fileAbs, err := filepath.Abs(filePath)
		if err != nil || filepath.Dir(fileAbs) != rootAbs {
			c.Status(http.StatusNotFound)
			return
		}

		file, err := os.Open(fileAbs)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}

		messages, err := messagesReferencingLocalAttachment(kind, name)
		if err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		grants, err := localAttachmentGrants(kind, name)
		if err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		viewerID, isAdmin := currentMessageViewer(c)
		allowed := len(messages) == 0 && len(grants) == 0
		publiclyReferenced := false
		liveMessageIDs := make(map[uint]struct{}, len(messages))
		for _, message := range messages {
			liveMessageIDs[message.ID] = struct{}{}
			if services.StoredMessageVisibility(message) == services.MessageVisibilityPublic {
				publiclyReferenced = true
			}
			if services.CanViewMessage(message, viewerID, isAdmin) {
				allowed = true
			}
		}
		for _, grant := range grants {
			if _, isLiveReference := liveMessageIDs[grant.MessageID]; isLiveReference {
				continue
			}
			snapshot := models.Message{
				ID:         grant.MessageID,
				UserID:     grant.OwnerUserID,
				Visibility: grant.Visibility,
				Private:    grant.Visibility != services.MessageVisibilityPublic,
			}
			if services.StoredMessageVisibility(snapshot) == services.MessageVisibilityPublic {
				publiclyReferenced = true
			}
			if services.CanViewMessage(snapshot, viewerID, isAdmin) {
				allowed = true
			}
		}
		if !allowed {
			c.Header("Cache-Control", "private, no-store")
			c.Header("Vary", "Cookie, Authorization")
			c.Status(http.StatusNotFound)
			return
		}
		if (len(messages) > 0 || len(grants) > 0) && !publiclyReferenced {
			c.Header("Cache-Control", "private, no-store")
			c.Header("Vary", "Cookie, Authorization")
		} else {
			c.Header("Cache-Control", "public, max-age=3600")
		}
		http.ServeContent(c.Writer, c.Request, name, info.ModTime(), file)
	}
}

func localAttachmentGrants(kind string, name string) ([]models.LocalAttachmentGrant, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var grants []models.LocalAttachmentGrant
	if err := db.Where("kind = ? AND name = ?", kind, name).Find(&grants).Error; err != nil {
		return nil, err
	}
	return grants, nil
}

func messagesReferencingLocalAttachment(kind string, name string) ([]models.Message, error) {
	encodedName := url.PathEscape(name)
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var candidates []models.Message
	if err := db.Select("id", "content", "image_url", "user_id", "private", "visibility").
		Where("content LIKE ? OR image_url LIKE ? OR content LIKE ? OR image_url LIKE ?", "%"+name+"%", "%"+name+"%", "%"+encodedName+"%", "%"+encodedName+"%").
		Find(&candidates).Error; err != nil {
		return nil, err
	}
	matched := make([]models.Message, 0, len(candidates))
	for _, message := range candidates {
		for _, reference := range models.ExtractLocalAttachmentReferences(message) {
			if reference.Kind == kind && reference.Name == name {
				matched = append(matched, message)
				break
			}
		}
	}
	return matched, nil
}
