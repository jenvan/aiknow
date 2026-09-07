package service

import (
	"context"
	"fmt"

	"github.com/aiknow-org/aiknow/internal/model"
)

// searchServiceImpl 检索服务实现（占位）
type searchServiceImpl struct{}

// NewSearchService 创建检索服务
func NewSearchService() SearchService {
	return &searchServiceImpl{}
}

// Search 混合检索
func (s *searchServiceImpl) Search(ctx context.Context, query string, params model.SearchParams) (*model.SearchResult, error) {
	return nil, fmt.Errorf("search service not implemented")
}

// SearchKeywords 关键词检索
func (s *searchServiceImpl) SearchKeywords(ctx context.Context, keywords []string, params model.KeywordSearchParams) (*model.KeywordSearchResult, error) {
	return nil, fmt.Errorf("keywords search not implemented")
}

// DebugSearch 检索调试
func (s *searchServiceImpl) DebugSearch(ctx context.Context, query string, params model.SearchParams) (*model.SearchDebugResult, error) {
	return nil, fmt.Errorf("debug search not implemented")
}
