package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/internal/service/provider"
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
	rrfResults := rrfFuse(vectorResults, bm25Results, params.RRFK, params.VectorWeight, params.BM25Weight)
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

	// 6. 按 Rerank 名次回取候选
	finalResults := applyRerank(rrfResults, rerankResult.Results)

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

// SearchKeywords 关键词加权检索（委托仓库层实现）
func (s *SearchServiceImpl) SearchKeywords(ctx context.Context, keywords []string, params model.KeywordSearchParams) (*model.KeywordSearchResult, error) {
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

// DebugSearch 检索调试：逐阶段返回中间结果，用于排查召回与排序问题。
//
// 与 Search 的差别只在于把各阶段结果分别保留下来，检索逻辑完全一致——
// 若两者行为出现分歧，调试页就失去了参考价值。
func (s *SearchServiceImpl) DebugSearch(ctx context.Context, query string, params model.SearchParams) (*model.SearchDebugResult, error) {
	if query == "" {
		return nil, fmt.Errorf("检索词为空")
	}

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

	// 1. 向量检索
	embStart := time.Now()
	embResult, err := s.embeddingProvider.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}
	embeddingMs := time.Since(embStart).Milliseconds()

	vectorStart := time.Now()
	vectorResults, err := s.searchRepo.VectorSearch(ctx, embResult.Embedding, params.CandidateK)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	vectorMs := time.Since(vectorStart).Milliseconds()

	// 2. BM25 检索
	bm25Start := time.Now()
	bm25Results, err := s.searchRepo.BM25Search(ctx, query, params.CandidateK)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}
	bm25Ms := time.Since(bm25Start).Milliseconds()

	// 3. RRF 融合
	rrfStart := time.Now()
	rrfResults := rrfFuse(vectorResults, bm25Results, params.RRFK, params.VectorWeight, params.BM25Weight)
	rrfMs := time.Since(rrfStart).Milliseconds()

	// 4. Rerank 精排
	rerankStart := time.Now()
	documents := make([]string, len(rrfResults))
	for i, r := range rrfResults {
		documents[i] = r.Content
	}

	var finalResults []model.SearchChunk
	rerankResult, err := s.rerankerProvider.Rerank(ctx, provider.RerankRequest{
		Query:     query,
		Documents: documents,
		TopK:      params.TopK,
	})
	if err != nil {
		// 精排失败不中断调试：前两路召回本身就是要观察的对象，
		// 直接返回融合结果并在该阶段留空更有助于定位问题
		finalResults = rrfResults
	} else {
		finalResults = applyRerank(rrfResults, rerankResult.Results)
	}
	rerankMs := time.Since(rerankStart).Milliseconds()

	totalMs := time.Since(start).Milliseconds()

	timings := model.SearchTimings{
		EmbeddingMs: int(embeddingMs),
		VectorMs:    int(vectorMs),
		BM25Ms:      int(bm25Ms),
		RRFMs:       int(rrfMs),
		RerankMs:    int(rerankMs),
		TotalMs:     int(totalMs),
	}

	return &model.SearchDebugResult{
		SearchParams: params,
		VectorResult: &model.SearchResult{Query: query, Results: vectorResults, Timings: timings},
		BM25Result:   &model.SearchResult{Query: query, Results: bm25Results, Timings: timings},
		RerankResult: &model.SearchResult{Query: query, Results: finalResults, Timings: timings},
	}, nil
}

// rrfFuse RRF 倒数排名融合
//
// 对每路召回按名次计 weight/(k+rank)，同一 chunk 的多路得分相加。
// 向量与 BM25 的名次分别记录，便于调试时观察各路贡献。
func rrfFuse(vectorResults, bm25Results []model.SearchChunk, k int, vectorWeight, bm25Weight float64) []model.SearchChunk {
	if k <= 0 {
		k = 60
	}
	if vectorWeight <= 0 {
		vectorWeight = 1.0
	}
	if bm25Weight <= 0 {
		bm25Weight = 1.0
	}

	scores := make(map[string]float64)
	vectorRanks := make(map[string]int)
	bm25Ranks := make(map[string]int)

	for i, r := range vectorResults {
		rank := i + 1
		scores[r.ID] += vectorWeight / float64(k+rank)
		vectorRanks[r.ID] = rank
	}

	for i, r := range bm25Results {
		rank := i + 1
		scores[r.ID] += bm25Weight / float64(k+rank)
		bm25Ranks[r.ID] = rank
	}

	// 去重并回填各路名次（两路名次分别记录，不再互相覆盖）
	seen := make(map[string]bool)
	ranked := make([]model.SearchChunk, 0, len(scores))
	appendUnique := func(list []model.SearchChunk) {
		for _, r := range list {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			r.RRFScore = scores[r.ID]
			r.VectorRank = vectorRanks[r.ID]
			r.BM25Rank = bm25Ranks[r.ID]
			ranked = append(ranked, r)
		}
	}
	appendUnique(vectorResults)
	appendUnique(bm25Results)

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].RRFScore != ranked[j].RRFScore {
			return ranked[i].RRFScore > ranked[j].RRFScore
		}
		// 得分相同时按 ID 排序，保证结果稳定
		return ranked[i].ID < ranked[j].ID
	})

	return ranked
}

// applyRerank 按 Rerank 返回的下标重排候选集
//
// Rerank 结果中的 Index 是送入文档数组的下标，需据此回取原 chunk；
// 此前误将该下标与 chunk 的 UUID 直接比较，导致结果恒为空、精排失效。
func applyRerank(candidates []model.SearchChunk, results []provider.RerankItem) []model.SearchChunk {
	final := make([]model.SearchChunk, 0, len(results))
	for _, item := range results {
		if item.Index < 0 || item.Index >= len(candidates) {
			continue
		}
		c := candidates[item.Index]
		c.Score = item.Score
		final = append(final, c)
	}
	return final
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
	rrfResults := rrfFuse(vectorResults, bm25Results, params.RRFK, params.VectorWeight, params.BM25Weight)

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

	finalResults := applyRerank(rrfResults, rerankResult.Results)

	return &model.SearchResult{
		Query:   query,
		Results: finalResults,
		Timings: model.SearchTimings{
			TotalMs: int(totalMs),
		},
	}, nil
}
