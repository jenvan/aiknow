package model

import (
	"time"
)

// Article 文章模型
type Article struct {
	ID               string     `json:"id" db:"id"`
	Title            string     `json:"title" db:"title"`
	Content          string     `json:"content" db:"content"`
	SourceType       string     `json:"source_type" db:"source_type"` // manual | url
	SourceURL        *string    `json:"source_url,omitempty" db:"source_url"`
	ProcessingStatus string     `json:"processing_status" db:"processing_status"` // pending | processing | completed | failed
	Summary          *string    `json:"summary,omitempty" db:"summary"`
	CategoryID       *string    `json:"category_id,omitempty" db:"category_id"`
	CategoryName     string     `json:"category_name" db:"category_name"`
	Tags             []Tag      `json:"tags" db:"-"`
	Published        bool       `json:"published" db:"published"`
	ViewCount        int        `json:"view_count" db:"view_count"`
	EmbeddingModel   *string    `json:"embedding_model,omitempty" db:"embedding_model"`
	EmbeddingDim     *int       `json:"embedding_dim,omitempty" db:"embedding_dim"`
	IndexedAt        *time.Time `json:"indexed_at,omitempty" db:"indexed_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	DeletedBy        *string    `json:"deleted_by,omitempty" db:"deleted_by"`
	LastError        *string    `json:"last_error,omitempty" db:"last_error"`
	RetryCount       int        `json:"retry_count" db:"retry_count"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

// ArticleInput 文章录入输入
type ArticleInput struct {
	Title        string   `json:"title" validate:"required,max=500"`
	Content      string   `json:"content" validate:"required"`
	SourceType   string   `json:"source_type"` // manual | url
	SourceURL    *string  `json:"source_url,omitempty"`
	CategoryID   *string  `json:"category_id,omitempty"`
	CategoryName *string  `json:"category_name,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// ArticleListQuery 文章列表查询参数
type ArticleListQuery struct {
	Page       int        `form:"page" default:"1"`
	PageSize   int        `form:"page_size" default:"20"`
	CategoryID *string    `form:"category_id,omitempty"`
	Category   string     `form:"category,omitempty"`
	Tags       string     `form:"tags,omitempty"`
	TagIDs     []string   `form:"tag_id,omitempty"`
	Status     *string    `form:"status,omitempty"`
	Search     *string    `form:"search,omitempty"`
	SortBy     string     `form:"sort_by" default:"created_at"`
	SortOrder  string     `form:"sort_order" default:"desc"`
	DateFrom   string     `form:"date_from,omitempty"` // yyyy-mm-dd
	DateTo     string     `form:"date_to,omitempty"`   // yyyy-mm-dd
}

// ArticleListResult 文章列表结果
type ArticleListResult struct {
	Total    int       `json:"total"`
	Pages    int       `json:"pages"`
	Articles []Article `json:"articles"`
}

// ArticleStatus 文章状态查询
type ArticleStatus struct {
	ID               string     `json:"id"`
	ProcessingStatus string     `json:"processing_status"`
	RetryCount       int        `json:"retry_count"`
	LastError        *string    `json:"last_error,omitempty"`
	IndexedAt        *time.Time `json:"indexed_at,omitempty"`
}
