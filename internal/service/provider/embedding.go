package provider

import "context"

// EmbeddingResult Embedding 结果
type EmbeddingResult struct {
	Embedding  []float32 `json:"embedding"`
	Model      string    `json:"model"`
	TokenCount int       `json:"token_count"`
}

// EmbeddingProvider Embedding Provider 接口
type EmbeddingProvider interface {
	// Embed 生成向量
	Embed(ctx context.Context, text string) (*EmbeddingResult, error)

	// EmbedBatch 批量生成向量
	EmbedBatch(ctx context.Context, texts []string) ([]*EmbeddingResult, error)

	// Name 返回 provider 名称
	Name() string

	// Dimension 返回向量维度
	Dimension() int
}

// MockEmbedding Mock Embedding 实现
type MockEmbedding struct {
	dim int
}

func NewMockEmbedding(dim int) *MockEmbedding {
	if dim == 0 {
		dim = 1024
	}
	return &MockEmbedding{dim: dim}
}

func (m *MockEmbedding) Name() string { return "mock" }

func (m *MockEmbedding) Dimension() int { return m.dim }

func (m *MockEmbedding) Embed(ctx context.Context, text string) (*EmbeddingResult, error) {
	// 生成随机向量
	embedding := make([]float32, m.dim)
	for i := range embedding {
		embedding[i] = float32(i) / float32(m.dim)
	}

	return &EmbeddingResult{
		Embedding:  embedding,
		Model:      "mock-bge-large-zh-v1.5",
		TokenCount: 10,
	}, nil
}

func (m *MockEmbedding) EmbedBatch(ctx context.Context, texts []string) ([]*EmbeddingResult, error) {
	results := make([]*EmbeddingResult, len(texts))
	for i, text := range texts {
		r, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = r
	}
	return results, nil
}
