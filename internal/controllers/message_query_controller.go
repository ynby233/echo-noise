package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

// GetMessages 处理 GET /messages 请求，返回所有留言
func GetMessages(c *gin.Context) {
	currentUserID, _ := currentMessageViewer(c)
	messages, err := services.GetAllMessagesForViewer(currentUserID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.GetAllMessagesFailMessage))
		return
	}
	c.JSON(http.StatusOK, dto.OK(messages, models.GetAllMessagesSuccess))
}

// GetMessage 处理 GET /messages/:id 请求，获取留言详情
func GetMessage(c *gin.Context) {
	// 从 URL 参数获取留言 ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}

	currentUserID, _ := currentMessageViewer(c)
	message, err := services.GetMessageByIDForViewer(uint(id), currentUserID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.GetMessageByIDFailMessage))
		return
	}

	if message == nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.MessageNotFoundMessage))
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, dto.OK(message, models.GetMessageByIDSuccess))
}

func LocateMessagePage(c *gin.Context) {
	var request dto.MessagePageLocateDto
	_ = c.ShouldBindJSON(&request)

	if request.MessageID == 0 {
		idStr := strings.TrimSpace(c.Query("messageId"))
		if idStr == "" {
			idStr = strings.TrimSpace(c.Query("id"))
		}
		if id, err := strconv.ParseUint(idStr, 10, 64); err == nil {
			request.MessageID = uint(id)
		}
	}
	if request.MessageID == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	if request.PageSize == 0 {
		if sizeStr := c.Query("pageSize"); sizeStr != "" {
			if size, err := strconv.Atoi(sizeStr); err == nil {
				request.PageSize = size
			}
		}
	}
	if request.AuthorID == nil {
		if aid := c.Query("authorId"); aid != "" {
			if v, err := strconv.ParseUint(aid, 10, 64); err == nil {
				vv := uint(v)
				request.AuthorID = &vv
			}
		}
	}
	if request.ExcludeID == nil {
		if eid := c.Query("excludeId"); eid != "" {
			if v, err := strconv.ParseUint(eid, 10, 64); err == nil {
				vv := uint(v)
				request.ExcludeID = &vv
			}
		}
	}
	if request.Username == nil {
		if un := strings.TrimSpace(c.Query("username")); un != "" {
			request.Username = &un
		}
	}
	if queryDate := strings.TrimSpace(c.Query("date")); queryDate != "" {
		request.Date = queryDate
	} else {
		request.Date = strings.TrimSpace(request.Date)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		request.Keyword = keyword
	} else {
		request.Keyword = strings.TrimSpace(request.Keyword)
	}
	if tag := strings.TrimSpace(c.Query("tag")); tag != "" {
		request.Tag = strings.TrimPrefix(tag, "#")
	} else {
		request.Tag = strings.TrimPrefix(strings.TrimSpace(request.Tag), "#")
	}
	if pinScope := strings.TrimSpace(c.Query("pinScope")); pinScope != "" {
		request.PinScope = pinScope
	} else {
		request.PinScope = strings.TrimSpace(request.PinScope)
	}

	currentUserID, _ := currentMessageViewer(c)
	if strings.EqualFold(request.PinScope, services.MessagePinScopePersonal) && (currentUserID == nil || request.AuthorID == nil || *currentUserID != *request.AuthorID) {
		c.JSON(http.StatusForbidden, dto.Fail[string]("个人作用域仅允许查询当前用户的笔记"))
		return
	}
	location, err := services.LocateMessagePage(request.MessageID, request.PageSize, currentUserID, request.AuthorID, request.Username, &request.Date, &request.Keyword, &request.Tag, request.PinScope, request.ExcludeID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK(location, models.GetMessagesByPageSuccess))
}

func GetMessagesByPage(c *gin.Context) {
	var page, pageSize int = 1, 10

	// 尝试从 POST JSON 数据获取分页参数
	var pageRequest dto.PageQueryDto
	if err := c.ShouldBindJSON(&pageRequest); err == nil {
		page = pageRequest.Page
		pageSize = pageRequest.PageSize
	} else {
		// 如果不是 POST JSON，则尝试从 URL 查询参数获取
		if pageStr := c.Query("page"); pageStr != "" {
			if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
				page = p
			}
		}
		if sizeStr := c.Query("pageSize"); sizeStr != "" {
			if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
				pageSize = s
			}
		}
	}

	// 验证分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// Session、Bearer 与 Token 路由统一通过同一个实时身份解析器，
	// 避免过期 Session 抢占有效 Bearer 身份。
	currentUserID, _ := currentMessageViewer(c)

	// 作者筛选（可选）
	var authorID *uint
	if aid := c.Query("authorId"); aid != "" {
		if v, err := strconv.ParseUint(aid, 10, 64); err == nil {
			vv := uint(v)
			authorID = &vv
		}
	}
	if authorID == nil && pageRequest.AuthorID != nil {
		authorID = pageRequest.AuthorID
	}
	if pageRequest.ExcludeID == nil {
		if eid := c.Query("excludeId"); eid != "" {
			if v, err := strconv.ParseUint(eid, 10, 64); err == nil {
				vv := uint(v)
				pageRequest.ExcludeID = &vv
			}
		}
	}
	var username *string
	if un := c.Query("username"); strings.TrimSpace(un) != "" {
		u := strings.TrimSpace(un)
		username = &u
	}
	if username == nil && pageRequest.Username != nil && strings.TrimSpace(*pageRequest.Username) != "" {
		u := strings.TrimSpace(*pageRequest.Username)
		username = &u
	}
	if queryDate := strings.TrimSpace(c.Query("date")); queryDate != "" {
		pageRequest.Date = queryDate
	} else {
		pageRequest.Date = strings.TrimSpace(pageRequest.Date)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		pageRequest.Keyword = keyword
	} else {
		pageRequest.Keyword = strings.TrimSpace(pageRequest.Keyword)
	}
	if tag := strings.TrimSpace(c.Query("tag")); tag != "" {
		pageRequest.Tag = strings.TrimPrefix(tag, "#")
	} else {
		pageRequest.Tag = strings.TrimPrefix(strings.TrimSpace(pageRequest.Tag), "#")
	}
	if pinScope := strings.TrimSpace(c.Query("pinScope")); pinScope != "" {
		pageRequest.PinScope = pinScope
	} else {
		pageRequest.PinScope = strings.TrimSpace(pageRequest.PinScope)
	}
	if strings.EqualFold(pageRequest.PinScope, services.MessagePinScopePersonal) && (currentUserID == nil || authorID == nil || *currentUserID != *authorID) {
		c.JSON(http.StatusForbidden, dto.Fail[string]("个人作用域仅允许查询当前用户的笔记"))
		return
	}

	pageQueryResult, err := services.GetMessagesByPage(page, pageSize, currentUserID, authorID, username, &pageRequest.Date, &pageRequest.Keyword, &pageRequest.Tag, pageRequest.PinScope, pageRequest.ExcludeID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK(pageQueryResult, models.GetMessagesByPageSuccess))
}
