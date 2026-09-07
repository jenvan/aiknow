package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/aiknow-org/aiknow/internal/model"
	"github.com/aiknow-org/aiknow/internal/repository"
	"github.com/aiknow-org/aiknow/internal/service/provider"
)

// SearchServiceImpl 检索服务实现
type SearchServiceImpl struct {
	searchRepo       repository.SearchRepository
	embeddingProvider provider.EmbeddingProvider
	rerankerProvider  provider.RerankerProvider
	chunkRepo         repository.ChunkRepository
	categoryRepo      repository.CategoryRepository
	tagRepo           repository.TagRepository
}

// NewSearchService 创建检索服务
func NewSearchService(
	searchRepo repository.SearchRepository,
	embeddingProvider provider.EmbeddingProvider,
	rerankerProvider provider.RerankerProvider,
	chunkRepo repository.ChunkRepository,
	categoryRepo repository.CategoryRepository,
	tagRepo repository.TagRepository,
) SearchService {
	return &SearchServiceImpl{
		searchRepo:        searchRepo,
		embeddingProvider: embeddingProvider,
		rerankerProvider:  rerankerProvider,
		chunkRepo:         chunkRepo,
		categoryRepo:      categoryRepo,
		tagRepo:           tagRepo,
	}
}

// Search 混合检索（向量 + BM25 + RRF + Rerank）
func (s *SearchServiceImpl) Search(ctx context.Context, query string, params model.SearchParams) (*model.SearchResult, error) {
	start := time.Now()

	// 默认参数
	if params.CandidateK <= 0 {
		params.CandidateK = 50
	}
	if params.RRFK <= 0 {
		params.RRFK = 60
	}
	if params.TopK <= 0 {
		params.TopK = 10
	}

	// 1. 生成 Query 向量
	embStart := time.Now()
	embResult, err := s.embeddingProvider.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}
	embeddingMs := time.Since(embStart).Milliseconds()

	// 2. 向量检索
	vectorStart := time.Now()
	vectorResults, err := s.searchRepo.VectorSearch(ctx, embResult.Embedding, params.CandidateK)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	vectorMs := time.Since(vectorStart).Milliseconds()

	// 3. BM25 检索
	bm25Start := time.Now()
	bm25Results, err := s.searchRepo.BM25Search(ctx, query, params.CandidateK)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}
	bm25Ms := time.Since(bm25Start).Milliseconds()

	// 4. RRF 融合
	rrfStart := time.Now()
	rrfResults := rrfFuse(vectorResults, bm25Results, params.RRFK)
	rrfMs := time.Since(rrfStart).Milliseconds()

	// 5. Rerank 精排
	rerankStart := time.Now()
	documents := make([]string, len(rrfResults))
	for i, r := range rrfResults {
		documents[i] = r.Content
	}

	rerankResult, err := s.rerankerProvider.Rerank(ctx, provider.RerankRequest{
		Query:     query,
		Documents: documents,
		TopK:      params.TopK,
	})
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}
	rerankMs := time.Since(rerankStart).Milliseconds()

	// 6. 补全结果
	finalResults := make([]model.SearchChunk, 0, len(rerankResult.Results))
	for _, item := range rerankResult.Results {
		for _, r := range rrfResults {
			if r.ID == fmt.Sprintf("%d", item.Index) {
				r.Score = item.Score
				finalResults = append(finalResults, r)
				break
			}
		}
	}

	totalMs := time.Since(start).Milliseconds()

	return &model.SearchResult{
		Query:   query,
		Results: finalResults,
		Timings: model.SearchTimings{
			EmbeddingMs: int(embeddingMs),
			VectorMs:    int(vectorMs),
			BM25Ms:      int(bm25Ms),
			RRFMs:       int(rrfMs),
			RerankMs:    int(rerankMs),
			TotalMs:     int(totalMs),
		},
	}, nil
}

// SearchKeywords 关键词检索
func (s *SearchServiceImpl) SearchKeywords(ctx context.Context, keywords []string, params model.KeywordSearchParams) (*model.KeywordSearchResult, error) {
	return nil, fmt.Errorf("keyword search not fully implemented")
}

// DebugSearch 检索调试
func (s *SearchServiceImpl) DebugSearch(ctx context.Context, query string, params model.SearchParams) (*model.SearchDebugResult, error) {
	// TODO: 实现调试接口
	return nil, fmt.Errorf("debug search not implemented")
}

// rrfFuse RRF 倒数排名融合
func rrfFuse(vectorResults, bm25Results []model.SearchChunk, k int) []model.SearchChunk {
	scores := make(map[string]float64)
	ranks := make(map[string]int)

	for i, r := range vectorResults {
		scores[r.ID] += 1.0 / float64(k+i)
		ranks[r.ID] = i + 1
	}

	for i, r := range bm25Results {
		scores[r.ID] += 1.0 / float64(k+i)
		ranks[r.ID] = i + 1
	}

	// 去重并保留原始 chunk 数据
	seen := make(map[string]bool)
	var ranked []model.SearchChunk
	for _, r := range vectorResults {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		r.RRFScore = scores[r.ID]
		r.VectorRank = ranks[r.ID]
		r.BM25Rank = ranks[r.ID]
		ranked = append(ranked, r)
	}
	for _, r := range bm25Results {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		r.RRFScore = scores[r.ID]
		r.VectorRank = ranks[r.ID]
		r.BM25Rank = ranks[r.ID]
		ranked = append(ranked, r)
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].RRFScore > ranked[j].RRFScore
	})

	return ranked
}

// SearchWithConcurrent 并发执行向量与 BM25 检索（辅助方法）
func SearchWithConcurrent(ctx context.Context, query string, params model.SearchParams,
	sr repository.SearchRepository, embProvider provider.EmbeddingProvider, rerankProvider provider.RerankerProvider,
) (*model.SearchResult, error) {
	start := time.Now()

	if params.CandidateK <= 0 {
		params.CandidateK = 50
	}
	if params.RRFK <= 0 {
		params.RRFK = 60
	}
	if params.TopK <= 0 {
		params.TopK = 10
	}

	// 1. 生成向量
	embResult, err := embProvider.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}

	// 2. 并发执行向量检索与 BM25
	var wg sync.WaitGroup
	var vectorResults []model.SearchChunk
	var bm25Results []model.SearchChunk
	var vectorErr, bm25Err error

	wg.Add(2)
	go func() {
		defer wg.Done()
		vectorResults, vectorErr = sr.VectorSearch(ctx, embResult.Embedding, params.CandidateK)
	}()
	go func() {
		defer wg.Done()
		bm25Results, bm25Err = sr.BM25Search(ctx, query, params.CandidateK)
	}()
	wg.Wait()

	if vectorErr != nil {
		return nil, fmt.Errorf("vector search: %w", vectorErr)
	}
	if bm25Err != nil {
		return nil, fmt.Errorf("bm25 search: %w", bm25Err)
	}

	// 3. RRF 融合
	rrfResults := rrfFuse(vectorResults, bm25Results, params.RRFK)

	// 4. Rerank 精排
	documents := make([]string, len(rrfResults))
	for i, r := range rrfResults {
		documents[i] = r.Content
	}

	rerankResult, err := rerankProvider.Rerank(ctx, provider.RerankRequest{
		Query:     query,
		Documents: documents,
		TopK:      params.TopK,
	})
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}

	totalMs := time.Since(start).Milliseconds()

	// 补全结果
	finalResults := make([]model.SearchChunk, 0, len(rerankResult.Results))
	for _, item := range rerankResult.Results {
		for _, r := range rrfResults {
			if r.ID == fmt.Sprintf("%d", item.Index) {
				r.Score = item.Score
				finalResults = append(finalResults, r)
				break
			}
		}
	}

	return &model.SearchResult{
		Query:   query,
		Results: finalResults,
		Timings: model.SearchTimings{
			TotalMs: int(totalMs),
		},
	}, nil
}
