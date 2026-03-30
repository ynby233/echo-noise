package pkg

import (
    "strings"
    "unicode/utf8"
)

// TruncateText 截取文本，保持完整的单词
func TruncateText(text string, maxLength int) string {
    if utf8.RuneCountInString(text) <= maxLength {
        return text
    }

    runes := []rune(text)
    truncated := string(runes[:maxLength])
    
    // 添加省略号
    return strings.TrimSpace(truncated) + "..."
}

// GetFirstLine 获取文本的第一行作为标题
func GetFirstLine(text string) string {
    lines := strings.Split(text, "\n")
    if len(lines) == 0 {
        return ""
    }

    // 清理第一行的 Markdown 标记
    title := strings.TrimSpace(lines[0])

    if strings.HasPrefix(title, "#") {
        j := 0
        for j < len(title) && title[j] == '#' {
            j++
        }
        if j > 0 && j < len(title) && title[j] == ' ' {
            title = strings.TrimSpace(title[j:])
        }
    }
    if strings.HasPrefix(title, "- ") {
        title = strings.TrimSpace(strings.TrimPrefix(title, "- "))
    }
    
    // 如果第一行太长，截断它
    if utf8.RuneCountInString(title) > 50 {
        return TruncateText(title, 50)
    }
    
    return title
}