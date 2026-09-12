package service

import (
	"context"
	"time"

	"github.com/jenvan/aiknow/internal/model"
)

// L2ConfigSource L2 业务配置来源（由 config.ConfigCache 实现）。
//
// 各服务在每次调用时读取最新的 L2 配置（后台可改且热生效），
// 使管理后台设置的值优先于 L1 启动配置与代码内置默认。
type L2ConfigSource interface {
	Get() model.L2Config
}

// ChunkService Chunk 服务接口
type ChunkService interface {
	GetByArticle(ctx context.Context, articleID string) ([]model.Chunk, error)
	DeleteByArticle(ctx context.Context, articleID string) error
	CountByArticle(ctx context.Context, articleID string) (int, error)
	GetTotalCount(ctx context.Context) (int, error)
	SplitContent(content string, chunkSize, overlap int) ([]model.Chunk, error)
	CreateChunks(ctx context.Context, articleID string, content string, chunkSize, overlap int) ([]model.Chunk, error)
}

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
	CSRFToken(ctx context.Context, sessionID string) (string, error)
	ValidateCSRF(ctx context.Context, sessionID, token string) error
	ValidateKey(ctx context.Context, key string) (*model.APIKey, error)
	CreateKey(ctx context.Context, input model.APIKeyCreateInput, createdBy string) (*model.APIKeyPlaintext, error)
	ListKeys(ctx context.Context) ([]model.APIKey, error)
	SetKeyActive(ctx context.Context, id string, active bool) error
	DeleteKey(ctx context.Context, id string) error
}

// MCPService MCP 服务接口
type MCPService interface {
	Initialize(ctx context.Context, req model.MCPInitializeRequest) (*model.MCPInitializeResponse, error)
	ListTools(ctx context.Context, scope string) ([]model.MCPTOOL, error)
	CallTool(ctx context.Context, name string, arguments map[string]interface{}, scope string) (*model.MCPTOOLResult, error)
}

// MCPLogService MCP 日志服务接口
type MCPLogService interface {
	CreateLog(ctx context.Context, log *model.MCPRequestLog) error
	GetByTraceID(ctx context.Context, traceID string) (*model.MCPRequestLog, error)
	ListLogs(ctx context.Context, query model.MCPLogQuery) ([]model.MCPRequestLog, error)
	DeleteExpired(ctx context.Context, before time.Time) (int, error)
}

// RateLimiter 限流器接口
type RateLimiter interface {
	Allow(key string) bool
}
