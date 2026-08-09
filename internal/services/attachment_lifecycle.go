package services

import (
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

// RemoveUnreferencedMessageAttachmentReferences removes only logical registry
// references which are no longer mentioned by any surviving message or
// comment. Physical blobs are deliberately retained here; the registry's
// reference-counted cleanup may reclaim them in a separate safe operation.
func RemoveUnreferencedMessageAttachmentReferences(tx *gorm.DB, messageID uint, message models.Message, comments []models.Comment) error {
	if tx == nil || messageID == 0 {
		return nil
	}
	var refs []models.AttachmentReference
	if err := tx.Find(&refs).Error; err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	content := message.Content + "\n" + message.ImageURL
	for _, comment := range comments {
		content += "\n" + comment.Content
	}
	used := make([]models.AttachmentReference, 0, len(refs))
	for _, ref := range refs {
		if registryReferenceMentioned(content, ref) {
			used = append(used, ref)
		}
	}
	if len(used) == 0 {
		return nil
	}
	var survivors []models.Message
	if err := tx.Where("id <> ?", messageID).Find(&survivors).Error; err != nil {
		return err
	}
	var survivorComments []models.Comment
	if err := tx.Where("message_id <> ?", messageID).Find(&survivorComments).Error; err != nil {
		return err
	}
	survivingContent := strings.Builder{}
	for _, candidate := range survivors {
		survivingContent.WriteString(candidate.Content)
		survivingContent.WriteByte('\n')
		survivingContent.WriteString(candidate.ImageURL)
		survivingContent.WriteByte('\n')
	}
	for _, comment := range survivorComments {
		survivingContent.WriteString(comment.Content)
		survivingContent.WriteByte('\n')
	}
	for _, ref := range used {
		if registryReferenceMentioned(survivingContent.String(), ref) {
			continue
		}
		if err := tx.Delete(&models.AttachmentReference{}, ref.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

func registryReferenceMentioned(content string, ref models.AttachmentReference) bool {
	id := strings.TrimSpace(ref.PublicID)
	if id == "" || !strings.Contains(content, id) {
		return false
	}
	for _, prefix := range []string{"/api/files/refs/", "/api/images/refs/", "/api/video/refs/", "/api/audio/refs/", "/api/cloud-attachments/"} {
		if strings.Contains(content, prefix+id) {
			return true
		}
	}
	return false
}
