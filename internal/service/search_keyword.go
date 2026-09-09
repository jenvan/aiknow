package service

import (
	"context"
	"fmt"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
)

// KeywordSearchServiceImpl 关键词检索服务实现
type KeywordSearchServiceImpl struct {
	searchRepo   repository.SearchRepository
	categoryRepo repository.CategoryRepository
	tagRepo      repository.TagRepository
	// configSource 可选：L2 配置来源，未显式指定权重时用 L2 值
	configSource L2ConfigSource
}

// NewKeywordSearchService 创建关键词检索服务
func NewKeywordSearchService(
	searchRepo repository.SearchRepository,
	categoryRepo repository.CategoryRepository,
	tagRepo repository.TagRepository,
	configSources ...L2ConfigSource,
) KeywordSearchService {
	s := &KeywordSearchServiceImpl{
		searchRepo:   searchRepo,
		categoryRepo: categoryRepo,
		tagRepo:      tagRepo,
	}
	if len(configSources) > 0 && configSources[0] != nil {
		s.configSource = configSources[0]
	}
	return s
}

// applyDefaults 用 L2 配置（或内置默认）补齐未显式指定的权重
func (s *KeywordSearchServiceImpl) applyDefaults(params *model.KeywordSearchParams) {
	d := model.DefaultL2Config()
	if s.configSource != nil {
		d = s.configSource.Get()
	}
	if params.TopK <= 0 {
		params.TopK = 10
	}
	if params.TagWeight <= 0 {
		params.TagWeight = d.KeywordSearch.TagWeight
	}
	if params.CategoryWeight <= 0 {
		params.CategoryWeight = d.KeywordSearch.CategoryWeight
	}
	if params.TitleWeight <= 0 {
		params.TitleWeight = d.KeywordSearch.TitleWeight
	}
	if params.SummaryWeight <= 0 {
		params.SummaryWeight = d.KeywordSearch.SummaryWeight
	}
	if params.ContentWeight <= 0 {
		params.ContentWeight = d.KeywordSearch.ContentWeight
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

	s.applyDefaults(&params)

	results, err := s.searchRepo.KeywordSearch(ctx, keywords, params)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return &model.KeywordSearchResult{Total: 0, Articles: nil}, nil
	}
	return &results[0], nil
}
