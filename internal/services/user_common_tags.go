package services

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeCommonTags keeps the order used by the account's composer shortcuts.
func NormalizeCommonTags(values []string) ([]string, error) {
	tags := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		tag := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "#"))
		if tag == "" {
			continue
		}
		if !utf8.ValidString(tag) || utf8.RuneCountInString(tag) > 64 {
			return nil, errors.New("每个常用标签最多 64 个字符")
		}
		for _, r := range tag {
			if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
				return nil, errors.New("常用标签不能包含空格、标点或符号")
			}
		}
		if !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
		if len(tags) > 50 {
			return nil, errors.New("最多保存 50 个常用标签")
		}
	}
	return tags, nil
}

// UserCommonTags also supports accounts created before this column existed.
func UserCommonTags(stored string) []string {
	var values []string
	if json.Unmarshal([]byte(stored), &values) != nil {
		return []string{}
	}
	tags, err := NormalizeCommonTags(values)
	if err != nil {
		return []string{}
	}
	return tags
}
