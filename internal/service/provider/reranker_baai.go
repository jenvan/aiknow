package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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

// rerankResponse Reranker 响应。
// 不同 BAAI 兼容网关字段名不一致：siliconflow 用 relevance_score，
// jina/cohere 风格用 score。用指针区分“字段缺省”与“值为 0”，
// 缺失的那一侧不会误覆盖有值的一侧。
type rerankResponse struct {
	Results []struct {
		Index          int      `json:"index"`
		Score          *float64 `json:"score"`
		RelevanceScore *float64 `json:"relevance_score"`
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

	// 兼容两种分数字段：siliconflow 给 relevance_score，jina/cohere 风格给 score。
	// 用指针区分“缺省”与“为 0”，只有一侧有值时取有值的一侧。
	results := make([]RerankItem, 0, len(rerankResp.Results))
	for _, r := range rerankResp.Results {
		score := 0.0
		switch {
		case r.RelevanceScore != nil:
			score = *r.RelevanceScore
		case r.Score != nil:
			score = *r.Score
		}
		doc := ""
		if r.Index >= 0 && r.Index < len(req.Documents) {
			doc = req.Documents[r.Index]
		}
		results = append(results, RerankItem{
			Index:    r.Index,
			Score:    score,
			Document: doc,
		})
	}

	// BAAI 兼容网关会忽略请求里的 top_k，返回全部候选（已按相关性降序）。
	// 这里按得分显式排序并本地截断，保证 TopK 语义与下游 applyRerank 一致。
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if req.TopK > 0 && len(results) > req.TopK {
		results = results[:req.TopK]
	}

	return &RerankResult{Results: results}, nil
}
