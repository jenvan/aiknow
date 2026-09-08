package service

import (
	"context"
	"fmt"

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

// SearchKeywords 关键词加权检索
//
// 加权逻辑在仓库层以单条 SQL 完成（标签 > 分类 > 标题 > 摘要 > 正文），
// 中文分词由 pg_search 的 jieba 在数据库侧完成，无需应用层预分词。
func (s *KeywordSearchServiceImpl) SearchKeywords(ctx context.Context, keywords []string, params model.KeywordSearchParams) (*model.KeywordSearchResult, error) {
	if len(keywords) == 0 {
		return nil, fmt.Errorf("关键词为空")
	}

	results, err := s.searchRepo.KeywordSearch(ctx, keywords, params)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return &model.KeywordSearchResult{Total: 0, Articles: nil}, nil
	}
	return &results[0], nil
}
