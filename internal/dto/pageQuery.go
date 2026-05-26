package dto

import (
	"github.com/rcy1314/echo-noise/internal/models"
)

type PageQueryDto struct {
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	AuthorID *uint   `json:"authorId,omitempty"`
	Username *string `json:"username,omitempty"`
}

type PageQueryResult struct {
	Total int64            `json:"total"`
	Items []models.Message `json:"items"`
}
