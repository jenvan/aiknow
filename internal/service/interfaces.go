package service

import (
	"context"

	"github.com/aiknow-org/aiknow/internal/model"
)

// ArticleService 文章服务接口
type ArticleService interface {
	Create(ctx context.Context, input model.ArticleInput) (*model.Article, error)
	GetByID(ctx context.Context, id string) (*model.Article, error)
	List(ctx context.Context, query model.ArticleListQuery) (*model.ArticleListResult, error)
	Update(ctx context.Context, id string, input model.ArticleInput) (*model.Article, error)
	SoftDelete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
	GetStatus(ctx context.Context, id string) (*model.ArticleStatus, error)
	BulkUpdateStatus(ctx context.Context, ids []string, status string) error
	BulkSoftDelete(ctx context.Context, ids []string) error
	Reprocess(ctx context.Context, id string) error
}

// SearchService 检索服务接口
type SearchService interface {
	Search(ctx context.Context, query string, params model.SearchParams) (*model.SearchResult, error)
	SearchKeywords(ctx context.Context, keywords []string, params model.KeywordSearchParams) (*model.KeywordSearchResult, error)
	DebugSearch(ctx context.Context, query string, params model.SearchParams) (*model.SearchDebugResult, error)
}

// KeywordSearchService 关键词检索服务接口
type KeywordSearchService interface {
	SearchKeywords(ctx context.Context, keywords []string, params model.KeywordSearchParams) (*model.KeywordSearchResult, error)
}

// AuthService 认证服务接口
type AuthService interface {
	Login(ctx context.Context, username, password string) (string, error)
	Logout(ctx context.Context, sessionID string) error
	GetSession(ctx context.Context, sessionID string) (string, error)
	ValidateKey(ctx context.Context, key string) (*model.APIKey, error)
}

// RateLimiter 限流器接口
type RateLimiter interface {
	Allow(key string) bool
}
