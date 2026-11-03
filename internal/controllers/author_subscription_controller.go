package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/services"
)

func GetAuthorSubscription(c *gin.Context) {
	authorSubscriptionResponse(c, nil)
}

func SubscribeToAuthor(c *gin.Context) {
	subscribed := true
	authorSubscriptionResponse(c, &subscribed)
}

func UnsubscribeFromAuthor(c *gin.Context) {
	subscribed := false
	authorSubscriptionResponse(c, &subscribed)
}

func authorSubscriptionResponse(c *gin.Context, subscribed *bool) {
	// Identity is supplied exclusively by the authentication middleware.
	subscriberID := c.GetUint("user_id")
	if subscriberID == 0 {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录或登录已过期"))
		return
	}
	parsed, err := strconv.ParseUint(c.Param("id"), 10, strconv.IntSize)
	if err != nil || parsed == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("作者 ID 无效"))
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("订阅服务暂不可用"))
		return
	}
	var state services.AuthorSubscriptionState
	if subscribed == nil {
		state, err = services.GetAuthorSubscriptionState(db, subscriberID, uint(parsed))
	} else {
		state, err = services.SetAuthorSubscriptionState(db, subscriberID, uint(parsed), *subscribed)
	}
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAuthorSubscriptionUnauthorized):
			c.JSON(http.StatusUnauthorized, dto.Fail[any](err.Error()))
		case errors.Is(err, services.ErrAuthorSubscriptionNotFound):
			c.JSON(http.StatusNotFound, dto.Fail[any](err.Error()))
		case errors.Is(err, services.ErrAuthorSubscriptionSelf):
			c.JSON(http.StatusBadRequest, dto.Fail[any](err.Error()))
		default:
			c.JSON(http.StatusInternalServerError, dto.Fail[any]("订阅操作失败，请稍后重试"))
		}
		return
	}
	c.JSON(http.StatusOK, dto.OK(state))
}
