package services

import (
	"fmt"
	"net/http"
	"strings"
)

// Ech0 fetching owns its candidate endpoints and response mapping.
type feedFetchRequest struct {
	Method  string
	URL     string
	Body    interface{}
	Headers map[string]string
}

type sourceUserProfile struct {
	Username string
	Avatar   string
}

type sourcePublicProfile struct {
	Name   string
	Avatar string
}

func fetchEch0Source(client *http.Client, source InfoFeedSource, limit int) ([]InfoFeedItem, error) {
	bases := candidateSourceBases(source.URL, "/api/echo/query", "/echo/query", "/api/echo/page", "/echo/page")
	if len(bases) == 0 {
		return nil, fmt.Errorf("ech0 源地址无效: %s", source.URL)
	}
	base := ""
	var payload interface{}
	var err error
	for _, oneBase := range bases {
		if !isHTTPURL(oneBase) {
			continue
		}
		payload, err = fetchJSONFromCandidates(client, []feedFetchRequest{
			{
				Method: http.MethodPost,
				URL:    fmt.Sprintf("%s/api/echo/query", oneBase),
				Body: map[string]interface{}{
					"page":      1,
					"pageSize":  limit,
					"search":    "",
					"tagIds":    []string{},
					"sortBy":    "created_at",
					"sortOrder": "desc",
					"dateFrom":  0,
					"dateTo":    0,
				},
			},
			// 向后兼容旧接口
			{Method: http.MethodGet, URL: fmt.Sprintf("%s/api/echo/page?page=1&pageSize=%d", oneBase, limit)},
			{Method: http.MethodPost, URL: fmt.Sprintf("%s/api/echo/page", oneBase), Body: map[string]interface{}{"page": 1, "pageSize": limit}},
		}, "ech0")
		if err == nil {
			base = oneBase
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if base == "" {
		return nil, fmt.Errorf("ech0 源地址无效: %s", source.URL)
	}
	rows := extractRows(payload)
	sourceName := strings.TrimSpace(source.Name)
	if sourceName == "" {
		sourceName = "Ech0"
	}
	items := buildGenericFeedItems(rows, sourceName, base, platformParseOptions{
		ContentKeys:     []string{"content", "text", "summary", "description", "body", "markdown", "echo.content", "echo.text", "data.content", "message.content"},
		TitleKeys:       []string{"title", "name", "subject", "echo.title"},
		LinkKeys:        []string{"url", "link", "permalink", "path"},
		IDKeys:          []string{"id", "echo.id", "message.id", "uuid"},
		TimeKeys:        []string{"created_at", "createdAt", "create_time", "createTime", "publishedAt"},
		ImageKeys:       []string{"image_url", "imageURL", "imageUrl", "cover", "cover_url", "coverUrl", "thumbnail", "thumb", "echo.image_url"},
		ResourcesKeys:   []string{"echo_files", "echoFiles", "files", "resources", "attachments", "images"},
		AuthorKeys:      []string{"username", "user.username", "user.name", "user.nickname", "creator.username", "creator.name", "creator.nickname", "nickname", "author", "display_name", "echo.username"},
		AvatarKeys:      []string{"avatar", "avatar_url", "avatarUrl", "avatarURL", "user.avatar", "user.avatar_url", "user.avatarUrl", "creator.avatar", "creator.avatar_url", "creator.avatarUrl", "echo.avatar_url"},
		DefaultLinkPath: "/echo/%s",
		DefaultTitle:    "Ech0 动态",
		SourceType:      "ech0",
		Limit:           limit,
	})
	items = enrichItemsWithUserProfile(client, base, items)
	publicProfile := fetchSourcePublicProfile(client, base)
	for i := range items {
		if strings.TrimSpace(items[i].AvatarURL) == "" && strings.TrimSpace(publicProfile.Avatar) != "" {
			items[i].AvatarURL = publicProfile.Avatar
		}
		if strings.TrimSpace(items[i].Author) == "" && strings.TrimSpace(publicProfile.Name) != "" {
			items[i].Author = strings.TrimSpace(publicProfile.Name)
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("ech0 接口未返回可用内容")
	}
	return items, nil
}
