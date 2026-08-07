package models

import "strings"

// CanonicalGuestbookContent is the body used for newly created system
// guestbooks. The marker is intentionally strict so an ordinary note that
// merely mentions "留言板" cannot become the system guestbook.
const CanonicalGuestbookContent = "留言板\n\n此条用于承载全站留言，不会参与普通内容展示。\n\n#留言 #guestbook"

// IsCanonicalGuestbookContent recognizes the historical system marker while
// keeping the recognition narrow enough for migration and compatibility.
func IsCanonicalGuestbookContent(content string) bool {
	content = strings.ReplaceAll(strings.TrimSpace(content), "\r\n", "\n")
	if content == CanonicalGuestbookContent {
		return true
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "留言板" {
		return false
	}
	hasGuestbookTag := false
	for _, line := range lines {
		switch strings.TrimSpace(strings.ToLower(line)) {
		case "#guestbook":
			hasGuestbookTag = true
		case "#留言 #guestbook", "#guestbook #留言":
			hasGuestbookTag = true
		}
	}
	// Older installations used only the #guestbook line; the exact title
	// plus that standalone system tag is still a strict, migratable marker.
	return hasGuestbookTag
}
