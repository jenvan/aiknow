package provider

import "context"

// RerankRequest Reranker 请求
type RerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopK      int      `json:"top_k"`
}

// RerankResult Reranker 结果
type RerankResult struct {
	Results []RerankItem `json:"results"`
}

// RerankItem Reranker 单项结果
type RerankItem struct {
	Index    int     `json:"index"`
	Score    float64 `json:"score"`
	Document string  `json:"document"`
}

// RerankerProvider Reranker Provider 接口
type RerankerProvider interface {
	// Rerank 重新排序
	Rerank(ctx context.Context, req RerankRequest) (*RerankResult, error)

	// Name 返回 provider 名称
	Name() string
}

// MockReranker Mock Reranker 实现
type MockReranker struct{}

func (m *MockReranker) Name() string { return "mock" }

func (m *MockReranker) Rerank(ctx context.Context, req RerankRequest) (*RerankResult, error) {
	results := make([]RerankItem, len(req.Documents))
	for i, doc := range req.Documents {
		// 按输入顺序返回，得分递减
		results[i] = RerankItem{
			Index:    i,
			Score:    float64(len(req.Documents)-i) / float64(len(req.Documents)),
			Document: doc,
		}
	}

	// 如果指定了 top_k，截断
	if req.TopK > 0 && req.TopK < len(results) {
		results = results[:req.TopK]
	}

	return &RerankResult{Results: results}, nil
}
