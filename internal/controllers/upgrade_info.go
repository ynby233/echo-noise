package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

func computeUpgradeInfo() (bool, string, string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	type tagInfo struct{ Name, LastUpdated string }
	latest := tagInfo{}
	get := func(url string, v any) error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req = req.WithContext(ctx)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return json.NewDecoder(resp.Body).Decode(v)
	}
	type result struct {
		ok   bool
		info tagInfo
	}
	ch := make(chan result, 3)
	go func() {
		var v struct {
			Name        string `json:"name"`
			LastUpdated string `json:"last_updated"`
		}
		if get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags/latest", &v) == nil && strings.TrimSpace(v.LastUpdated) != "" {
			ch <- result{true, tagInfo{v.Name, v.LastUpdated}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	go func() {
		var v struct {
			Results []struct {
				Name        string `json:"name"`
				LastUpdated string `json:"last_updated"`
			} `json:"results"`
		}
		if get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags?page_size=1&ordering=last_updated", &v) == nil && len(v.Results) > 0 && strings.TrimSpace(v.Results[0].LastUpdated) != "" {
			r := v.Results[0]
			ch <- result{true, tagInfo{r.Name, r.LastUpdated}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	go func() {
		var v struct {
			TagName     string `json:"tag_name"`
			PublishedAt string `json:"published_at"`
		}
		if get("https://api.github.com/repos/noise233/echo-noise/releases/latest", &v) == nil && strings.TrimSpace(v.PublishedAt) != "" {
			ch <- result{true, tagInfo{v.TagName, v.PublishedAt}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	for i := 0; i < 3; i++ {
		r := <-ch
		if r.ok {
			latest = r.info
			break
		}
	}
	if strings.TrimSpace(latest.LastUpdated) == "" {
		cur := strings.TrimSpace(os.Getenv("ECHO_NOISE_VERSION"))
		if cur == "" {
			cur = strings.TrimSpace(os.Getenv("APP_VERSION"))
		}
		if cur == "" {
			cur = strings.TrimSpace(os.Getenv("IMAGE_TAG"))
		}
		if cur == "" {
			cur = "latest"
		}
		return false, time.Now().Format(time.RFC3339), cur, nil
	}
	cur := strings.TrimSpace(os.Getenv("ECHO_NOISE_VERSION"))
	if cur == "" {
		cur = strings.TrimSpace(os.Getenv("APP_VERSION"))
	}
	if cur == "" {
		cur = strings.TrimSpace(os.Getenv("IMAGE_TAG"))
	}
	if cur == "" {
		cur = "latest"
	}
	var curUpdated string
	if strings.ToLower(cur) == "latest" {
		curUpdated = strings.TrimSpace(latest.LastUpdated)
	} else {
		if resp, err := client.Get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags/" + cur); err == nil {
			defer resp.Body.Close()
			var curTag struct {
				Name        string `json:"name"`
				LastUpdated string `json:"last_updated"`
			}
			if json.NewDecoder(resp.Body).Decode(&curTag) == nil {
				curUpdated = strings.TrimSpace(curTag.LastUpdated)
			}
		}
		if strings.TrimSpace(curUpdated) == "" {
			if resp, err := client.Get("https://api.github.com/repos/noise233/echo-noise/releases/tags/" + cur); err == nil {
				defer resp.Body.Close()
				var rel struct {
					PublishedAt string `json:"published_at"`
				}
				if json.NewDecoder(resp.Body).Decode(&rel) == nil {
					curUpdated = strings.TrimSpace(rel.PublishedAt)
				}
			}
		}
	}
	latestTime, err := time.Parse(time.RFC3339, latest.LastUpdated)
	if err != nil {
		return false, "", cur, err
	}
	var hasUpdate bool
	if curUpdated != "" {
		curTime, err := time.Parse(time.RFC3339, curUpdated)
		if err != nil {
			return false, "", cur, err
		}
		hasUpdate = latestTime.After(curTime)
	} else {
		hasUpdate = true
	}
	return hasUpdate, latest.LastUpdated, cur, nil
}
