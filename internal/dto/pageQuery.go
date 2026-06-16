package dto

import (
	"github.com/rcy1314/echo-noise/internal/models"
)

type PageQueryDto struct {
	Page      int     `json:"page"`
	PageSize  int     `json:"pageSize"`
	AuthorID  *uint   `json:"authorId,omitempty"`
	Username  *string `json:"username,omitempty"`
	Date      string  `json:"date,omitempty"`
	Keyword   string  `json:"keyword,omitempty"`
	Tag       string  `json:"tag,omitempty"`
	ExcludeID *uint   `json:"excludeId,omitempty"`
}

type PageQueryResult struct {
	Total int64            `json:"total"`
	Items []models.Message `json:"items"`
}

type MessagePageLocateDto struct {
	MessageID uint    `json:"messageId"`
	PageSize  int     `json:"pageSize"`
	AuthorID  *uint   `json:"authorId,omitempty"`
	Username  *string `json:"username,omitempty"`
	Date      string  `json:"date,omitempty"`
	Keyword   string  `json:"keyword,omitempty"`
	Tag       string  `json:"tag,omitempty"`
	ExcludeID *uint   `json:"excludeId,omitempty"`
}

type MessagePageLocateResult struct {
	MessageID uint  `json:"messageId"`
	Page      int   `json:"page"`
	PageSize  int   `json:"pageSize"`
	Total     int64 `json:"total"`
}
