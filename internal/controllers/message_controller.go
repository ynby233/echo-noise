package controllers

import (
    "net/http"
    "time"
    "strconv"  // 添加 strconv 包
    "github.com/gin-gonic/gin"
    "github.com/rcy1314/echo-noise/internal/models"
    "github.com/rcy1314/echo-noise/internal/database"
    "github.com/rcy1314/echo-noise/internal/services"  // 添加 services 包
    "regexp"
)
// GetMessagesByTag 获取指定标签的消息
// GetMessagesByTag 获取指定标签的消息
func GetMessagesByTag(c *gin.Context) {
    tag := c.Param("tag")
    if tag == "" {
        c.JSON(http.StatusOK, gin.H{"code": 1, "data": []models.Message{}})
        return
    }

    db, err := database.GetDB()
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"code": 1, "data": []models.Message{}})
        return
    }

    var messages []models.Message
    // 使用 LIKE 进行初步筛选
    tagPattern := "%#" + tag + "%"
    q := db.Where("content LIKE ?", tagPattern)
    // 作者筛选（可选）
    if aid := c.Query("authorId"); aid != "" {
        if v, err := strconv.ParseUint(aid, 10, 64); err == nil {
            q = q.Where("user_id = ?", uint(v))
        }
    }
    if un := c.Query("username"); un != "" {
        q = q.Where("username = ?", un)
    }
    // 仅公开内容（小组件场景）
    q = q.Where("private = ?", false)
    if err := q.Order("created_at DESC").Find(&messages).Error; err != nil {
        c.JSON(http.StatusOK, gin.H{"code": 1, "data": []models.Message{}})
        return
    }

    // 使用正则表达式进行精确匹配
    var filteredMessages []models.Message
    tagRegex := regexp.MustCompile(`#` + regexp.QuoteMeta(tag) + `(?:[\s,.!?]|$)`)
    for _, msg := range messages {
        if tagRegex.MatchString(msg.Content) {
            filteredMessages = append(filteredMessages, msg)
        }
    }

    c.JSON(http.StatusOK, gin.H{"code": 1, "data": filteredMessages})
}

// GetAllTags 获取所有标签列表
func GetAllTags(c *gin.Context) {
    db, err := database.GetDB()
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"code": 1, "data": []map[string]interface{}{}})
        return
    }

    // 添加缓存控制头
    c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
    c.Header("Pragma", "no-cache")
    c.Header("Expires", "0")

    var messages []models.Message
    // 修改查询，按创建时间倒序排列并限制数量
    if err := db.Select("content").Order("created_at DESC").Find(&messages).Error; err != nil {
        c.JSON(http.StatusOK, gin.H{"code": 1, "data": []map[string]interface{}{}})
        return
    }

    // 提取并统计标签
    tagMap := make(map[string]int)
    invalidTagPattern := regexp.MustCompile(`[/?=&]|^(song|video|playlist)\?id=\d+$`)
    
    for _, msg := range messages {
        tags := regexp.MustCompile(`#([^\s#]+)`).FindAllStringSubmatch(msg.Content, -1)
        for _, tag := range tags {
            if len(tag) > 1 && !invalidTagPattern.MatchString(tag[1]) {
                tagMap[tag[1]]++
            }
        }
    }

    // 转换为数组格式并按计数倒序排序
    var tagList []map[string]interface{}
    for tag, count := range tagMap {
        tagList = append(tagList, map[string]interface{}{
            "name":  tag,
            "count": count,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "code": 1, 
        "data": tagList,
        "timestamp": time.Now().Unix(), // 添加时间戳
    })
}
// GetAllImages 获取所有图片列表.
func GetAllImages(c *gin.Context) {
    db, err := database.GetDB()
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"code": 1, "data": []map[string]interface{}{}})
        return
    }

    var messages []models.Message
    if err := db.Select("id", "content", "image_url", "created_at").Order("created_at DESC").Find(&messages).Error; err != nil {
        c.JSON(http.StatusOK, gin.H{"code": 1, "data": []map[string]interface{}{}})
        return
    }

    type ImageInfo struct {
        ID        uint      `json:"id"`
        ImageURL  string    `json:"image_url"`
        CreatedAt time.Time `json:"created_at"`
    }

    var allImages []ImageInfo
    
    // 正则表达式匹配 Markdown 格式的图片
    imageRegex := regexp.MustCompile(`!\[.*?\]\((.*?)\)`)

    for _, msg := range messages {
        // 添加 image_url 字段的图片
        if msg.ImageURL != "" {
            allImages = append(allImages, ImageInfo{
                ID:        msg.ID,
                ImageURL:  msg.ImageURL,
                CreatedAt: msg.CreatedAt,
            })
        }

        // 提取内容中的 Markdown 格式图片
        matches := imageRegex.FindAllStringSubmatch(msg.Content, -1)
        for _, match := range matches {
            if len(match) > 1 {
                allImages = append(allImages, ImageInfo{
                    ID:        msg.ID,
                    ImageURL:  match[1],
                    CreatedAt: msg.CreatedAt,
                })
            }
        }
    }

    c.JSON(http.StatusOK, gin.H{"code": 1, "data": allImages})
}

// GetMessagePage 处理消息详情页请求
func GetMessagePage(c *gin.Context) {
    id := c.Param("id")
    messageID, err := strconv.ParseUint(id, 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
        return
    }
    
    message, err := services.GetMessagePage(uint(messageID))
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "code": 1,
        "data": message,
    })
}
