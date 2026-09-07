package repository

import (
	"context"
	"time"

	"github.com/aiknow-org/aiknow/internal/model"
)

// ArticleRepository 文章仓库接口
type ArticleRepository interface {
	Create(ctx context.Context, article *model.Article) error
	GetByID(ctx context.Context, id string) (*model.Article, error)
	List(ctx context.Context, query model.ArticleListQuery) (*model.ArticleListResult, error)
	Update(ctx context.Context, article *model.Article) error
	SoftDelete(ctx context.Context, id string, deletedBy *string) error
	Restore(ctx context.Context, id string) error
	CountByStatus(ctx context.Context) (map[string]int, error)
	FindByURL(ctx context.Context, url string) (*model.Article, error)
	FindByContentHash(ctx context.Context, hash string) (*model.Article, error)
	BulkUpdateStatus(ctx context.Context, ids []string, status string) error
	BulkSoftDelete(ctx context.Context, ids []string, deletedBy *string) error
	GetArticleStatus(ctx context.Context, id string) (*model.ArticleStatus, error)
}

// ChunkRepository Chunk 仓库接口
type ChunkRepository interface {
	Create(ctx context.Context, chunk *model.Chunk) error
	BulkCreate(ctx context.Context, chunks []*model.Chunk) error
	ListByArticle(ctx context.Context, articleID string) ([]model.Chunk, error)
	DeleteByArticle(ctx context.Context, articleID string) error
	UpdateEmbedding(ctx context.Context, id string, embedding []float32) error
	CountByArticle(ctx context.Context, articleID string) (int, error)
	GetTotalCount(ctx context.Context) (int, error)
}

// CategoryRepository 分类仓库接口
type CategoryRepository interface {
	Create(ctx context.Context, category *model.Category) (*model.Category, error)
	GetByID(ctx context.Context, id string) (*model.Category, error)
	GetByName(ctx context.Context, name string) (*model.Category, error)
	ListAll(ctx context.Context) ([]model.Category, error)
	GetOrCreateByName(ctx context.Context, name string) (*model.Category, error)
}

// TagRepository 标签仓库接口
type TagRepository interface {
	Create(ctx context.Context, tag *model.Tag) (*model.Tag, error)
	GetByID(ctx context.Context, id string) (*model.Tag, error)
	GetByName(ctx context.Context, name string) (*model.Tag, error)
	ListAll(ctx context.Context) ([]model.Tag, error)
	BulkCreate(ctx context.Context, tags []*model.Tag) ([]model.Tag, error)
	LinkTagsToArticle(ctx context.Context, articleID string, tagNames []string) error
}

// SettingRepository 配置仓库接口
type SettingRepository interface {
	GetByKey(ctx context.Context, key string) (*model.Setting, error)
	Upsert(ctx context.Context, setting *model.Setting) error
	ListAll(ctx context.Context) ([]model.Setting, error)
	DeleteByKey(ctx context.Context, key string) error
	GetHistory(ctx context.Context, settingID string, limit int) ([]model.SettingHistory, error)
}

// APIKeyRepository API Key 仓库接口
type APIKeyRepository interface {
	Create(ctx context.Context, key *model.APIKey) error
	GetByID(ctx context.Context, id string) (*model.APIKey, error)
	GetByHash(ctx context.Context, hash string) (*model.APIKey, error)
	ListAll(ctx context.Context) ([]model.APIKey, error)
	Update(ctx context.Context, key *model.APIKey) error
	Delete(ctx context.Context, id string) error
}

// MCPLogRepository MCP 日志仓库接口
type MCPLogRepository interface {
	Create(ctx context.Context, log *model.MCPRequestLog) error
	List(ctx context.Context, query model.MCPLogQuery) (*model.MCPRequestLog, error)
	GetByTraceID(ctx context.Context, traceID string) (*model.MCPRequestLog, error)
	DeleteExpired(ctx context.Context, before time.Time) (int, error)
}
