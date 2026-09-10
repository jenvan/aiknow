package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BAAIEmbeddingProvider BAAI Embedding Provider 实现
type BAAIEmbeddingProvider struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
	dim     int
}

// NewBAAIEmbeddingProvider 创建 BAAI Embedding Provider
func NewBAAIEmbeddingProvider(baseURL, apiKey, model string) (*BAAIEmbeddingProvider, error) {
	if model == "" {
		model = "BAAI/bge-large-zh-v1.5"
	}
	if baseURL == "" {
		// 默认 deepinfra，不带 /v1（provider 内部拼 /v1/embeddings）
		baseURL = "https://api.deepinfra.com"
	}
	if apiKey == "" {
		return nil, fmt.Errorf("BAAI API key is required")
	}

	return &BAAIEmbeddingProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		dim: 1024,
	}, nil
}

func (b *BAAIEmbeddingProvider) Name() string { return "baai" }

func (b *BAAIEmbeddingProvider) Dimension() int { return b.dim }

// embeddingRequest Embedding 请求
type embeddingRequest struct {
	Model        string `json:"model"`
	Input        string `json:"input"`
	EncodingType string `json:"encoding_type,omitempty"`
}

// embeddingResponse Embedding 响应
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (b *BAAIEmbeddingProvider) Embed(ctx context.Context, text string) (*EmbeddingResult, error) {
	reqBody := embeddingRequest{
		Model:        b.model,
		Input:        text,
		EncodingType: "float",
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/v1/embeddings", bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var embResp embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	return &EmbeddingResult{
		Embedding:  embResp.Data[0].Embedding,
		Model:      b.model,
		TokenCount: 10,
	}, nil
}

func (b *BAAIEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([]*EmbeddingResult, error) {
	results := make([]*EmbeddingResult, len(texts))
	for i, text := range texts {
		r, err := b.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed batch item %d: %w", i, err)
		}
		results[i] = r
	}
	return results, nil
}
