package provider

import (
	"context"
	"fmt"
)

// ProviderFactory 工厂模式创建 Provider
type ProviderFactory struct {
	config L1ProviderConfig
}

// L1ProviderConfig L1 Provider 配置
type L1ProviderConfig struct {
	LLMProvider       string // qwen | mock
	LLMBaseURL        string
	LLMAPIKey         string
	LLMModel          string
	EmbeddingProvider string // baai | mock
	EmbeddingBaseURL  string
	EmbeddingAPIKey   string
	EmbeddingModel    string
	RerankerProvider  string // baai | mock
	RerankerBaseURL   string
	RerankerAPIKey    string
	RerankerModel     string
}

// NewProviderFactory 创建 Provider 工厂
func NewProviderFactory(config L1ProviderConfig) *ProviderFactory {
	return &ProviderFactory{config: config}
}

// CreateLLM 创建 LLM Provider
func (f *ProviderFactory) CreateLLM(ctx context.Context) (LLMProvider, error) {
	switch f.config.LLMProvider {
	case "qwen":
		return NewQwenProvider(f.config.LLMBaseURL, f.config.LLMAPIKey, f.config.LLMModel)
	case "mock", "":
		return &MockLLM{}, nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", f.config.LLMProvider)
	}
}

// CreateEmbedding 创建 Embedding Provider
func (f *ProviderFactory) CreateEmbedding(ctx context.Context) (EmbeddingProvider, error) {
	switch f.config.EmbeddingProvider {
	case "baai":
		return NewBAAIEmbeddingProvider(f.config.EmbeddingBaseURL, f.config.EmbeddingAPIKey, f.config.EmbeddingModel)
	case "mock", "":
		return NewMockEmbedding(1024), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", f.config.EmbeddingProvider)
	}
}

// CreateReranker 创建 Reranker Provider
func (f *ProviderFactory) CreateReranker(ctx context.Context) (RerankerProvider, error) {
	switch f.config.RerankerProvider {
	case "baai":
		return NewBAAIRerankerProvider(f.config.RerankerBaseURL, f.config.RerankerAPIKey, f.config.RerankerModel)
	case "mock", "":
		return &MockReranker{}, nil
	default:
		return nil, fmt.Errorf("unknown reranker provider: %s", f.config.RerankerProvider)
	}
}
