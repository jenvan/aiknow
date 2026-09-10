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

// BAAIRerankerProvider BAAI Reranker Provider 实现
type BAAIRerankerProvider struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

// NewBAAIRerankerProvider 创建 BAAI Reranker Provider
func NewBAAIRerankerProvider(baseURL, apiKey, model string) (*BAAIRerankerProvider, error) {
	if model == "" {
		model = "BAAI/bge-reranker-v2-m3"
	}
	if baseURL == "" {
		// 默认 deepinfra，不带 /v1（provider 内部拼 /v1/rerank）
		baseURL = "https://api.deepinfra.com"
	}
	if apiKey == "" {
		return nil, fmt.Errorf("BAAI Reranker API key is required")
	}

	return &BAAIRerankerProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (b *BAAIRerankerProvider) Name() string { return "baai" }

// rerankRequest Reranker 请求
type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopK      int      `json:"top_k"`
}

// rerankResponse Reranker 响应
type rerankResponse struct {
	Results []struct {
		Index int     `json:"index"`
		Score float64 `json:"score"`
	} `json:"results"`
}

func (b *BAAIRerankerProvider) Rerank(ctx context.Context, req RerankRequest) (*RerankResult, error) {
	reqBody := rerankRequest{
		Model:     b.model,
		Query:     req.Query,
		Documents: req.Documents,
		TopK:      req.TopK,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/v1/rerank", bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+b.apiKey)

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var rerankResp rerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	results := make([]RerankItem, len(rerankResp.Results))
	for i, r := range rerankResp.Results {
		doc := ""
		if r.Index < len(req.Documents) {
			doc = req.Documents[r.Index]
		}
		results[i] = RerankItem{
			Index:    r.Index,
			Score:    r.Score,
			Document: doc,
		}
	}

	return &RerankResult{Results: results}, nil
}
