package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
)

// KeywordSearchServiceImpl 关键词检索服务实现
type KeywordSearchServiceImpl struct {
	searchRepo     repository.SearchRepository
	categoryRepo   repository.CategoryRepository
	tagRepo        repository.TagRepository
}

// NewKeywordSearchService 创建关键词检索服务
func NewKeywordSearchService(
	searchRepo repository.SearchRepository,
	categoryRepo repository.CategoryRepository,
	tagRepo repository.TagRepository,
) KeywordSearchService {
	return &KeywordSearchServiceImpl{
		searchRepo:     searchRepo,
		categoryRepo:   categoryRepo,
		tagRepo:        tagRepo,
	}
}

// SearchKeywords 关键词检索
func (s *KeywordSearchServiceImpl) SearchKeywords(ctx context.Context, keywords []string, params model.KeywordSearchParams) (*model.KeywordSearchResult, error) {
	if len(keywords) == 0 {
		return nil, fmt.Errorf("empty keywords")
	}

	// TODO: 实现关键词加权检索
	// 1. 对每个关键词进行 BM25 检索
	// 2. 按权重融合（标签权重 > 分类权重 > 标题权重 > 内容权重）
	// 3. 返回 TopK 结果
	return nil, fmt.Errorf("keyword search not fully implemented")
}

// getQueryTokens 获取查询分词结果（用于调试）
func (s *KeywordSearchServiceImpl) getQueryTokens(query string) []string {
	// TODO: 调用 zhparser 分词
	return strings.Fields(query)
}
