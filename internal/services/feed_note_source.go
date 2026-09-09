package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Note-source fetching handles the native echo-noise JSON contract.
func fetchNoteSource(client *http.Client, source InfoFeedSource, limit int) ([]InfoFeedItem, error) {
	bases := candidateSourceBases(source.URL, "/api/messages", "#/messages")
	if len(bases) == 0 {
		return nil, fmt.Errorf("信息流 note 源为空")
	}
	base := ""
	var payload map[string]interface{}
	var lastErr error
	for _, oneBase := range bases {
		if !isHTTPURL(oneBase) {
			continue
		}
		apiURL := fmt.Sprintf("%s/api/messages/page?page=1&pageSize=%d", oneBase, limit)
		req, _ := http.NewRequest(http.MethodGet, apiURL, nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Echo-Noise-InfoFlow/1.0")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("note 请求失败: %d", resp.StatusCode)
			continue
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			lastErr = err
			continue
		}
		if len(extractRows(parsed)) == 0 {
			lastErr = fmt.Errorf("note 接口返回为空")
			continue
		}
		base = oneBase
		payload = parsed
		break
	}
	if base == "" {
		if lastErr == nil {
			lastErr = fmt.Errorf("note 源地址无效: %s", source.URL)
		}
		return nil, lastErr
	}
	rows := extractRows(payload)
	sourceName := strings.TrimSpace(source.Name)
	if sourceName == "" {
		sourceName = "同部署项目"
	}
	type noteProfileInfo struct {
		Username string
		Avatar   string
	}
	profileCache := map[string]noteProfileInfo{}
	loadUserProfile := func(username string) noteProfileInfo {
		name := strings.TrimSpace(username)
		if name == "" {
			return noteProfileInfo{}
		}
		key := strings.ToLower(name)
		if cached, ok := profileCache[key]; ok {
			return cached
		}
		profileURL := fmt.Sprintf("%s/api/users/profile?username=%s", base, url.QueryEscape(name))
		req, _ := http.NewRequest(http.MethodGet, profileURL, nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Echo-Noise-InfoFlow/1.0")
		resp, err := client.Do(req)
		if err != nil {
			profileCache[key] = noteProfileInfo{}
			return noteProfileInfo{}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			profileCache[key] = noteProfileInfo{}
			return noteProfileInfo{}
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		if err != nil {
			profileCache[key] = noteProfileInfo{}
			return noteProfileInfo{}
		}
		var profilePayload map[string]interface{}
		if err := json.Unmarshal(body, &profilePayload); err != nil {
			profileCache[key] = noteProfileInfo{}
			return noteProfileInfo{}
		}
		profileObj := readMapByKey(profilePayload, "data")
		if profileObj == nil {
			profileObj = profilePayload
		}
		info := noteProfileInfo{
			Username: cleanText(firstNonEmpty(
				readStringByKeys(profileObj, "username", "name", "displayName", "display_name"),
				name,
			)),
			Avatar: resolveRelativeURL(base, firstNonEmpty(
				readStringByKeys(profileObj, "avatar_url", "avatarUrl", "avatar"),
				readStringByKeys(profileObj, "profile.avatar_url", "profile.avatarUrl", "profile.avatar"),
			)),
		}
		profileCache[key] = info
		return info
	}
	out := make([]InfoFeedItem, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		id := toInt(readAnyByPath(m, "id"), 0)
		if id <= 0 {
			id = toInt(readAnyByPath(m, "ID"), 0)
		}
		rawContent := firstNonEmpty(
			readFlexibleTextByKeys(m, "content", "text", "summary", "body", "markdown"),
			fmt.Sprintf("%v", m["content"]),
		)
		content := normalizeRawContent(rawContent)
		if content == "" {
			content = cleanText(rawContent)
		}
		if content == "" {
			continue
		}
		title := limitText(firstLine(cleanContentText(content)), 80)
		if title == "" {
			title = limitText(firstLine(content), 80)
		}
		if title == "" {
			title = "未命名内容"
		}
		link := strings.TrimSpace(firstNonEmpty(
			readStringByKeys(m, "url", "link", "permalink"),
			readStringByKeys(m, "message_url", "messageUrl"),
		))
		if source.Local && id > 0 {
			link = fmt.Sprintf("/#/messages/%d", id)
		} else if link == "" && id > 0 {
			link = strings.TrimSpace(fmt.Sprintf("%s/#/messages/%d", base, id))
		}
		if !source.Local {
			link = resolveRelativeURL(base, link)
		}
		if link == "" {
			continue
		}
		pubRaw := firstNonEmpty(
			readStringByKeys(m, "created_at", "createdAt", "createTime", "created", "publishedAt", "display_time", "displayTime"),
			strings.TrimSpace(fmt.Sprintf("%v", m["created_at"])),
		)
		img := resolveRelativeURL(base, firstNonEmpty(
			readStringByKeys(m, "image_url", "imageURL", "imageUrl", "cover", "cover_url", "coverUrl", "thumbnail"),
			extractImageFromResources(base, m, []string{"attachments", "resources", "files", "echo_files"}),
		))
		tm := parseAnyTime(pubRaw)
		author := cleanText(firstNonEmpty(
			readStringByKeys(m, "username"),
			readStringByKeys(m, "userName"),
			readStringByKeys(m, "user.username"),
			readStringByKeys(m, "user.name"),
			readStringByKeys(m, "creator.username"),
			readStringByKeys(m, "creator.name"),
			readStringByKeys(m, "nickname"),
			readStringByKeys(m, "author"),
			readStringByKeys(m, "display_name"),
		))
		avatarURL := resolveRelativeURL(base, firstNonEmpty(
			readStringByKeys(m, "avatar"),
			readStringByKeys(m, "avatar_url"),
			readStringByKeys(m, "avatarUrl"),
			readStringByKeys(m, "avatarURL"),
			readStringByKeys(m, "user.avatar"),
			readStringByKeys(m, "user.avatar_url"),
			readStringByKeys(m, "user.avatarUrl"),
			readStringByKeys(m, "user.avatarURL"),
			readStringByKeys(m, "creator.avatar"),
			readStringByKeys(m, "creator.avatar_url"),
			readStringByKeys(m, "creator.avatarUrl"),
			readStringByKeys(m, "creator.avatarURL"),
		))
		if author != "" && avatarURL == "" {
			profile := loadUserProfile(author)
			if strings.TrimSpace(profile.Avatar) != "" {
				avatarURL = profile.Avatar
			}
			if strings.TrimSpace(profile.Username) != "" {
				author = profile.Username
			}
		}
		out = append(out, InfoFeedItem{
			Title:       title,
			Link:        link,
			Content:     content,
			Summary:     content,
			ImageURL:    img,
			Source:      sourceName,
			Type:        "note",
			Author:      author,
			AvatarURL:   avatarURL,
			PublishedAt: formatTime(tm, pubRaw),
			Timestamp:   tm.Unix(),
		})
	}
	return out, nil
}
