package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jenvan/aiknow/internal/model"
)

// MCPServiceImpl MCP 服务实现
type MCPServiceImpl struct {
	searchService  SearchService
	keywordService KeywordSearchService
	articleService ArticleService
	authService    AuthService
	mcpLogService  MCPLogService
}

// NewMCPService 创建 MCP 服务
func NewMCPService(
	searchService SearchService,
	keywordService KeywordSearchService,
	articleService ArticleService,
	authService AuthService,
	mcpLogService MCPLogService,
) MCPService {
	return &MCPServiceImpl{
		searchService:  searchService,
		keywordService: keywordService,
		articleService: articleService,
		authService:    authService,
		mcpLogService:  mcpLogService,
	}
}

// Initialize 处理 MCP initialize 请求
func (s *MCPServiceImpl) Initialize(ctx context.Context, req model.MCPInitializeRequest) (*model.MCPInitializeResponse, error) {
	return &model.MCPInitializeResponse{
		Protocol: "2025-06-18",
		Capabilities: model.MCPServerCap{},
		ServerInfo: model.MCPServerInfo{
			Name:    "aiknow-server",
			Version: "0.1.0",
		},
	}, nil
}

// ListTools 列出可用工具
func (s *MCPServiceImpl) ListTools(ctx context.Context, scope string) ([]model.MCPTOOL, error) {
	tools := []model.MCPTOOL{
		{
			Name:        "search_knowledge",
			Description: "使用语义检索查找相关知识片段，返回 chunk 级证据块用于 RAG 证据供给",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string", "description": "检索查询"},
					"top_k": map[string]interface{}{"type": "integer", "description": "返回结果数", "default": 5},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "find_articles_by_keywords",
			Description: "使用关键词检索查找相关文章，返回文章卡片用于文章发现。与 search_knowledge 的区别：关键词检索返回整篇文章卡片，语义检索返回 chunk 片段",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"keywords": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "关键词列表"},
					"limit":    map[string]interface{}{"type": "integer", "description": "返回数量", "default": 5},
				},
				"required": []string{"keywords"},
			},
		},
		{
			Name:        "get_article_detail",
			Description: "获取文章全文详情",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"article_id": map[string]interface{}{"type": "string", "description": "文章 ID"},
				},
				"required": []string{"article_id"},
			},
		},
	}

	// 如果是 readwrite scope，添加写入工具
	if scope == "readwrite" {
		tools = append(tools, model.MCPTOOL{
			Name:        "add_article_by_url",
			Description: "通过 URL 采集并提交文章入库，用于将外部网页添加到知识库",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{"type": "string", "description": "文章 URL"},
				},
				"required": []string{"url"},
			},
		})
	}

	return tools, nil
}

// CallTool 调用工具
func (s *MCPServiceImpl) CallTool(ctx context.Context, name string, arguments map[string]interface{}, scope string) (*model.MCPTOOLResult, error) {
	switch name {
	case "search_knowledge":
		return s.searchKnowledge(ctx, arguments)
	case "find_articles_by_keywords":
		return s.findArticlesByKeywords(ctx, arguments)
	case "get_article_detail":
		return s.getArticleDetail(ctx, arguments)
	case "add_article_by_url":
		if scope != "readwrite" {
			return nil, fmt.Errorf("readwrite scope required")
		}
		return s.addArticleByURL(ctx, arguments)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// searchKnowledge 语义检索知识片段
func (s *MCPServiceImpl) searchKnowledge(ctx context.Context, args map[string]interface{}) (*model.MCPTOOLResult, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query is required")
	}

	topK := 5
	if k, ok := args["top_k"].(float64); ok {
		topK = int(k)
	}

	result, err := s.searchService.Search(ctx, query, model.SearchParams{TopK: topK})
	if err != nil {
		return nil, fmt.Errorf("search knowledge: %w", err)
	}

	items := make([]model.MCPContentItem, 0, len(result.Results))
	for _, r := range result.Results {
		items = append(items, model.MCPContentItem{
			Type: "text",
			Text: fmt.Sprintf("[chunk %d] (score=%.4f)\n%s", r.ChunkIndex, r.Score, r.Content),
		})
	}

	return &model.MCPTOOLResult{
		Content: items,
		IsError: false,
	}, nil
}

// findArticlesByKeywords 关键词检索文章
func (s *MCPServiceImpl) findArticlesByKeywords(ctx context.Context, args map[string]interface{}) (*model.MCPTOOLResult, error) {
	keywords, ok := args["keywords"].([]interface{})
	if !ok || len(keywords) == 0 {
		return nil, fmt.Errorf("keywords is required")
	}

	// 将 interface{} 切片转换为 string 切片
	kwList := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		if s, ok := kw.(string); ok {
			kwList = append(kwList, s)
		}
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	result, err := s.keywordService.SearchKeywords(ctx, kwList, model.KeywordSearchParams{TopK: limit})
	if err != nil {
		return nil, fmt.Errorf("search keywords: %w", err)
	}

	items := make([]model.MCPContentItem, 0, len(result.Articles))
	for _, a := range result.Articles {
		items = append(items, model.MCPContentItem{
			Type: "text",
			Text: fmt.Sprintf("ID: %s\n标题: %s\n摘要: %s\n", a.ID, a.Title, safeStr(a.Summary)),
		})
	}

	return &model.MCPTOOLResult{
		Content: items,
		IsError: false,
	}, nil
}

// getArticleDetail 获取文章全文详情
func (s *MCPServiceImpl) getArticleDetail(ctx context.Context, args map[string]interface{}) (*model.MCPTOOLResult, error) {
	articleID, ok := args["article_id"].(string)
	if !ok || articleID == "" {
		return nil, fmt.Errorf("article_id is required")
	}

	article, err := s.articleService.GetByID(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("get article: %w", err)
	}
	if article == nil {
		return nil, fmt.Errorf("article not found: %s", articleID)
	}

	return &model.MCPTOOLResult{
		Content: []model.MCPContentItem{
			{
				Type: "text",
				Text: fmt.Sprintf("标题: %s\n\n内容:\n%s", article.Title, article.Content),
			},
		},
		IsError: false,
	}, nil
}

// addArticleByURL 通过 URL 采集并提交文章入库
func (s *MCPServiceImpl) addArticleByURL(ctx context.Context, args map[string]interface{}) (*model.MCPTOOLResult, error) {
	url, ok := args["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("url is required")
	}

	// TODO: 实现 URL 采集和入库
	return &model.MCPTOOLResult{
		Content: []model.MCPContentItem{
			{Type: "text", Text: "URL 采集功能待实现"},
		},
		IsError: false,
	}, nil
}

// safeStr 安全地返回字符串，nil 时返回空字符串
func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// generateTraceID 生成唯一追踪 ID
func generateTraceID() string {
	return uuid.New().String()
}
