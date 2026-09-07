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

// MCPService MCP 服务接口
type MCPService interface {
	Initialize(ctx context.Context, req model.MCPInitializeRequest) (*model.MCPInitializeResponse, error)
	ListTools(ctx context.Context, scope string) ([]model.MCPTOOL, error)
	CallTool(ctx context.Context, name string, arguments map[string]interface{}, scope string) (*model.MCPTOOLResult, error)
}
