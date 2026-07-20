package models

import (
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

type LocalAttachmentReference struct {
	Kind string
	Name string
}

var localAttachmentReferencePrefixes = []struct {
	kind   string
	prefix string
}{
	{kind: "image", prefix: "/api/images/"},
	{kind: "image", prefix: "/images/"},
	{kind: "video", prefix: "/api/video/"},
	{kind: "video", prefix: "/video/"},
	{kind: "audio", prefix: "/api/audio/"},
	{kind: "audio", prefix: "/audio/"},
	{kind: "file", prefix: "/api/files/"},
	{kind: "file", prefix: "/files/"},
	{kind: "file", prefix: "/api/attachments/"},
	{kind: "file", prefix: "/attachments/"},
}

func ExtractLocalAttachmentReferences(message Message) []LocalAttachmentReference {
	seen := make(map[string]struct{})
	references := make([]LocalAttachmentReference, 0)
	for _, source := range []string{message.Content, message.ImageURL} {
		for _, candidate := range localAttachmentReferencePrefixes {
			offset := 0
			for offset < len(source) {
				relative := strings.Index(source[offset:], candidate.prefix)
				if relative < 0 {
					break
				}
				start := offset + relative + len(candidate.prefix)
				end := start
				for end < len(source) && !isLocalAttachmentURLDelimiter(rune(source[end])) {
					end++
				}
				rawName := source[start:end]
				name, err := url.PathUnescape(rawName)
				if err == nil && name != "" && name == filepath.Base(name) && !strings.ContainsAny(name, `/\`) {
					key := candidate.kind + "\x00" + name
					if _, exists := seen[key]; !exists {
						seen[key] = struct{}{}
						references = append(references, LocalAttachmentReference{Kind: candidate.kind, Name: name})
					}
				}
				offset = end
				if offset <= start {
					offset = start + 1
				}
			}
		}
	}
	return references
}

func ExtractCloudAttachmentPublicIDs(message Message) []string {
	const prefix = "/api/cloud-attachments/"
	seen := make(map[string]struct{})
	publicIDs := make([]string, 0)
	for _, source := range []string{message.Content, message.ImageURL} {
		offset := 0
		for offset < len(source) {
			relative := strings.Index(source[offset:], prefix)
			if relative < 0 {
				break
			}
			start := offset + relative + len(prefix)
			end := strings.IndexByte(source[start:], '/')
			if end < 0 {
				break
			}
			end += start
			publicID := strings.TrimSpace(source[start:end])
			if publicID != "" && !strings.ContainsAny(publicID, `/\?#`) {
				if _, exists := seen[publicID]; !exists {
					seen[publicID] = struct{}{}
					publicIDs = append(publicIDs, publicID)
				}
			}
			offset = end + 1
		}
	}
	return publicIDs
}

func isLocalAttachmentURLDelimiter(value rune) bool {
	return unicode.IsSpace(value) || strings.ContainsRune(`)]}"'<>`+"`"+`?#`, value)
}

func StoredLocalAttachmentVisibility(message Message) string {
	visibility := strings.ToLower(strings.TrimSpace(message.Visibility))
	switch visibility {
	case "public", "users", "contacts", "private":
		return visibility
	case "":
		if message.Private {
			return "private"
		}
		return "public"
	default:
		return "private"
	}
}

func SyncLocalAttachmentGrants(tx *gorm.DB, message *Message) error {
	if tx == nil || message == nil || message.ID == 0 {
		return nil
	}
	visibility := StoredLocalAttachmentVisibility(*message)
	for _, reference := range ExtractLocalAttachmentReferences(*message) {
		if err := syncAttachmentGrant(tx, message, reference.Kind, reference.Name, visibility); err != nil {
			return err
		}
	}
	for _, publicID := range ExtractCloudAttachmentPublicIDs(*message) {
		if err := syncAttachmentGrant(tx, message, "cloud", publicID, visibility); err != nil {
			return err
		}
	}
	return nil
}

func syncAttachmentGrant(tx *gorm.DB, message *Message, kind string, name string, visibility string) error {
	var grant LocalAttachmentGrant
	return tx.Where("kind = ? AND name = ? AND message_id = ?", kind, name, message.ID).
		Assign(LocalAttachmentGrant{
			OwnerUserID: message.UserID,
			Visibility:  visibility,
		}).
		FirstOrCreate(&grant, LocalAttachmentGrant{
			Kind:      kind,
			Name:      name,
			MessageID: message.ID,
		}).Error
}
