package controllers

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
)

type imageBackingKind int

const (
	imageBackingUnknown imageBackingKind = iota
	imageBackingLocalFile
	imageBackingReference
	imageBackingCloudObject
)

type imageBacking struct {
	kind imageBackingKind
	id   string
}

// localImageDir 解析本地图片目录，兼容源码运行、容器与自定义 savepath。
func localImageDir() string {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
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
	}, "./data/images")
}

// classifyImageBacking 判断图片 URL 背后的存储对象类型。无法判定的一律视为
// unknown，由调用方按“保留”处理，避免误删外链或未知形态的图片。
func classifyImageBacking(rawURL string) imageBacking {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return imageBacking{}
	}
	path := trimmed
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Path != "" {
		path = parsed.Path
	}
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		path = path[:idx]
	}
	if path == "" || !strings.HasPrefix(path, "/") {
		return imageBacking{}
	}
	if path == "/api" {
		return imageBacking{}
	}
	path = strings.TrimPrefix(path, "/api")
	if !strings.HasPrefix(path, "/") {
		return imageBacking{}
	}

	if rest, ok := trimRoutePrefix(path, "/cloud-attachments/"); ok {
		id, _, found := strings.Cut(rest, "/")
		if !found || strings.TrimSpace(id) == "" {
			return imageBacking{}
		}
		return imageBacking{kind: imageBackingCloudObject, id: id}
	}

	rest, ok := trimRoutePrefix(path, "/images/")
	if !ok {
		return imageBacking{}
	}
	if refRest, isRef := trimRoutePrefix(rest, "refs/"); isRef {
		id, _, found := strings.Cut(refRest, "/")
		if !found || strings.TrimSpace(id) == "" {
			return imageBacking{}
		}
		return imageBacking{kind: imageBackingReference, id: id}
	}
	name, err := url.PathUnescape(rest)
	if err != nil || name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return imageBacking{}
	}
	return imageBacking{kind: imageBackingLocalFile, id: name}
}

func trimRoutePrefix(path string, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return "", false
	}
	return rest, true
}

// imageAvailability 批量判定一组图片 URL 的存储对象是否仍然存在。
type imageAvailability struct {
	localNames map[string]struct{}
	references map[string]struct{}
	cloudIDs   map[string]struct{}
	skipLocal  bool
	skipRefs   bool
	skipCloud  bool
}

func newImageAvailability(urls []string) *imageAvailability {
	availability := &imageAvailability{
		localNames: map[string]struct{}{},
		references: map[string]struct{}{},
		cloudIDs:   map[string]struct{}{},
	}
	wantedRefs := map[string]struct{}{}
	wantedCloud := map[string]struct{}{}
	needLocal := false
	for _, rawURL := range urls {
		switch backing := classifyImageBacking(rawURL); backing.kind {
		case imageBackingLocalFile:
			needLocal = true
		case imageBackingReference:
			wantedRefs[backing.id] = struct{}{}
		case imageBackingCloudObject:
			wantedCloud[backing.id] = struct{}{}
		}
	}

	if needLocal {
		entries, err := os.ReadDir(localImageDir())
		if err != nil {
			availability.skipLocal = true
		} else {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				availability.localNames[entry.Name()] = struct{}{}
			}
		}
	}

	if len(wantedRefs) > 0 || len(wantedCloud) > 0 {
		db, err := database.GetDB()
		if err != nil {
			availability.skipRefs = true
			availability.skipCloud = true
			return availability
		}
		if len(wantedRefs) > 0 {
			ids := setKeys(wantedRefs)
			var found []string
			if err := db.Model(&models.AttachmentReference{}).Where("public_id IN ?", ids).Pluck("public_id", &found).Error; err != nil {
				availability.skipRefs = true
			} else {
				for _, id := range found {
					availability.references[id] = struct{}{}
				}
			}
		}
		if len(wantedCloud) > 0 {
			ids := setKeys(wantedCloud)
			var refFound []string
			if err := db.Model(&models.AttachmentReference{}).Where("public_id IN ?", ids).Pluck("public_id", &refFound).Error; err != nil {
				availability.skipCloud = true
			} else {
				for _, id := range refFound {
					availability.cloudIDs[id] = struct{}{}
				}
				var cloudFound []string
				if err := db.Model(&models.CloudAttachmentObject{}).Where("public_id IN ?", ids).Pluck("public_id", &cloudFound).Error; err != nil {
					availability.skipCloud = true
				} else {
					for _, id := range cloudFound {
						availability.cloudIDs[id] = struct{}{}
					}
				}
			}
		}
	}

	return availability
}

// Has 仅在能够确认存储对象已缺失时返回 false。
func (a *imageAvailability) Has(rawURL string) bool {
	if a == nil {
		return true
	}
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	backing := classifyImageBacking(rawURL)
	switch backing.kind {
	case imageBackingLocalFile:
		if a.skipLocal {
			return true
		}
		_, ok := a.localNames[backing.id]
		return ok
	case imageBackingReference:
		if a.skipRefs {
			return true
		}
		_, ok := a.references[backing.id]
		return ok
	case imageBackingCloudObject:
		if a.skipCloud {
			return true
		}
		_, ok := a.cloudIDs[backing.id]
		return ok
	default:
		return true
	}
}

func setKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
