package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// BAAI 兼容网关的 /v1/rerank 返回两种分数字段：
//   - siliconflow：relevance_score（且忽略请求里的 top_k，返回全部候选）
//   - jina/cohere：score
// 本测试锁住两件事：字段兼容、以及本地按得分降序并截断到 top_k。
func TestBAAIRerankerAcceptsRelevanceScoreAndTruncates(t *testing.T) {
	// 模拟 siliconflow：不理会 top_k，返回全部 4 条，字段名 relevance_score，
	// 分值乱序（index 0..3 → 0.1 / 0.9 / 0.3 / 0.7）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[
			{"index":0,"relevance_score":0.1},
			{"index":1,"relevance_score":0.9},
			{"index":2,"relevance_score":0.3},
			{"index":3,"relevance_score":0.7}
		]}`))
	}))
	t.Cleanup(srv.Close)

	p, err := NewBAAIRerankerProvider(srv.URL, "test-key", "BAAI/bge-reranker-v2-m3")
	if err != nil {
		t.Fatalf("NewBAAIRerankerProvider: %v", err)
	}

	docs := []string{"d0", "d1", "d2", "d3"}
	res, err := p.Rerank(context.Background(), RerankRequest{Query: "q", Documents: docs, TopK: 2})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}

	// 网关返回了 4 条，但 top_k=2 必须在本地截断到 2
	if len(res.Results) != 2 {
		t.Fatalf("expected top_k=2 truncation, got %d results", len(res.Results))
	}
	// 按得分降序：0.9（index1）应排第一
	if res.Results[0].Index != 1 || res.Results[0].Score < 0.8 {
		t.Errorf("first result should be index=1 score≈0.9, got index=%d score=%v", res.Results[0].Index, res.Results[0].Score)
	}
	// 得分非 0（relevance_score 被正确读取）
	if res.Results[0].Score == 0 || res.Results[1].Score == 0 {
		t.Errorf("scores must be non-zero from relevance_score, got %v", res.Results)
	}
	// 文档内容按 index 回取
	if res.Results[0].Document != "d1" {
		t.Errorf("document not mapped back by index, got %q", res.Results[0].Document)
	}
}

// jina/cohere 风格字段名为 score，同样要读得到。
func TestBAAIRerankerAcceptsScoreField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"index":0,"score":0.42}]}`))
	}))
	t.Cleanup(srv.Close)

	p, err := NewBAAIRerankerProvider(srv.URL, "test-key", "m")
	if err != nil {
		t.Fatalf("NewBAAIRerankerProvider: %v", err)
	}
	res, err := p.Rerank(context.Background(), RerankRequest{Query: "q", Documents: []string{"x"}, TopK: 1})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].Score != 0.42 {
		t.Fatalf("expected score=0.42 from 'score' field, got %+v", res.Results)
	}
}
