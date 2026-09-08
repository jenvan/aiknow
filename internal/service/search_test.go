package service

import (
	"math"
	"testing"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/service/provider"
)

func chunks(ids ...string) []model.SearchChunk {
	out := make([]model.SearchChunk, len(ids))
	for i, id := range ids {
		out[i] = model.SearchChunk{ID: id, Content: "内容-" + id}
	}
	return out
}

func findByID(list []model.SearchChunk, id string) (model.SearchChunk, bool) {
	for _, c := range list {
		if c.ID == id {
			return c, true
		}
	}
	return model.SearchChunk{}, false
}

// 两路都命中的文档得分应为两路之和，且排在单路命中之前
func TestRRFFuseCombinesBothLists(t *testing.T) {
	vector := chunks("a", "b", "c")
	bm25 := chunks("c", "d")

	got := rrfFuse(vector, bm25, 60, 1.0, 1.0)

	if len(got) != 4 {
		t.Fatalf("期望 4 条去重结果，实际 %d", len(got))
	}

	// c 在向量第 3 名、BM25 第 1 名：1/63 + 1/61
	c, ok := findByID(got, "c")
	if !ok {
		t.Fatal("结果中缺少 c")
	}
	want := 1.0/63.0 + 1.0/61.0
	if math.Abs(c.RRFScore-want) > 1e-9 {
		t.Errorf("c 的 RRF 得分 = %v，期望 %v", c.RRFScore, want)
	}
	if got[0].ID != "c" {
		t.Errorf("两路命中的 c 应排首位，实际首位为 %s", got[0].ID)
	}
}

// 回归：两路名次此前被同一个 map 覆盖，导致 VectorRank 与 BM25Rank 恒相等
func TestRRFFuseRanksTrackedSeparately(t *testing.T) {
	vector := chunks("a", "b", "c")
	bm25 := chunks("c", "b", "a")

	got := rrfFuse(vector, bm25, 60, 1.0, 1.0)

	a, _ := findByID(got, "a")
	if a.VectorRank != 1 || a.BM25Rank != 3 {
		t.Errorf("a 的名次 = (向量 %d, BM25 %d)，期望 (1, 3)", a.VectorRank, a.BM25Rank)
	}

	c, _ := findByID(got, "c")
	if c.VectorRank != 3 || c.BM25Rank != 1 {
		t.Errorf("c 的名次 = (向量 %d, BM25 %d)，期望 (3, 1)", c.VectorRank, c.BM25Rank)
	}
}

// 仅单路命中时，另一路名次应为 0 而非被误填
func TestRRFFuseSingleListRankIsZero(t *testing.T) {
	got := rrfFuse(chunks("a"), nil, 60, 1.0, 1.0)

	a, _ := findByID(got, "a")
	if a.VectorRank != 1 {
		t.Errorf("VectorRank = %d，期望 1", a.VectorRank)
	}
	if a.BM25Rank != 0 {
		t.Errorf("未被 BM25 召回，BM25Rank 应为 0，实际 %d", a.BM25Rank)
	}
}

// 名次从 1 起算：首位得分应为 weight/(k+1)
func TestRRFFuseRankStartsAtOne(t *testing.T) {
	got := rrfFuse(chunks("a"), nil, 60, 1.0, 1.0)

	want := 1.0 / 61.0
	if math.Abs(got[0].RRFScore-want) > 1e-9 {
		t.Errorf("首位得分 = %v，期望 %v（k+1，而非 k+0）", got[0].RRFScore, want)
	}
}

func TestRRFFuseWeightsApplied(t *testing.T) {
	// BM25 权重为 0 视作未设置，回落到 1.0；此处用差异明显的权重验证加权生效
	got := rrfFuse(chunks("a"), chunks("b"), 60, 3.0, 1.0)

	a, _ := findByID(got, "a")
	b, _ := findByID(got, "b")

	if math.Abs(a.RRFScore-3.0/61.0) > 1e-9 {
		t.Errorf("向量路加权未生效：a = %v", a.RRFScore)
	}
	if math.Abs(b.RRFScore-1.0/61.0) > 1e-9 {
		t.Errorf("BM25 路加权未生效：b = %v", b.RRFScore)
	}
	if got[0].ID != "a" {
		t.Errorf("高权重的 a 应排在前，实际首位 %s", got[0].ID)
	}
}

func TestRRFFuseDefaultsOnZeroK(t *testing.T) {
	got := rrfFuse(chunks("a"), nil, 0, 0, 0)
	if len(got) != 1 {
		t.Fatalf("期望 1 条结果，实际 %d", len(got))
	}
	// k 回落到 60、权重回落到 1.0
	if math.Abs(got[0].RRFScore-1.0/61.0) > 1e-9 {
		t.Errorf("默认值未生效：得分 = %v", got[0].RRFScore)
	}
}

func TestRRFFuseEmptyInput(t *testing.T) {
	if got := rrfFuse(nil, nil, 60, 1.0, 1.0); len(got) != 0 {
		t.Errorf("空输入应返回空结果，实际 %d 条", len(got))
	}
}

// 回归：此前用 chunk 的 UUID 与 Rerank 返回的数组下标比较，结果恒为空
func TestApplyRerankMapsByIndex(t *testing.T) {
	candidates := chunks("uuid-a", "uuid-b", "uuid-c")
	results := []provider.RerankItem{
		{Index: 2, Score: 0.9},
		{Index: 0, Score: 0.5},
	}

	got := applyRerank(candidates, results)

	if len(got) != 2 {
		t.Fatalf("期望 2 条结果，实际 %d（下标映射失效会返回 0 条）", len(got))
	}
	if got[0].ID != "uuid-c" || got[0].Score != 0.9 {
		t.Errorf("首条 = (%s, %v)，期望 (uuid-c, 0.9)", got[0].ID, got[0].Score)
	}
	if got[1].ID != "uuid-a" || got[1].Score != 0.5 {
		t.Errorf("次条 = (%s, %v)，期望 (uuid-a, 0.5)", got[1].ID, got[1].Score)
	}
}

func TestApplyRerankSkipsOutOfRangeIndex(t *testing.T) {
	candidates := chunks("a", "b")
	results := []provider.RerankItem{
		{Index: 5, Score: 0.9},  // 越界
		{Index: -1, Score: 0.8}, // 负数
		{Index: 1, Score: 0.7},  // 合法
	}

	got := applyRerank(candidates, results)

	if len(got) != 1 {
		t.Fatalf("越界下标应被跳过，期望 1 条，实际 %d", len(got))
	}
	if got[0].ID != "b" {
		t.Errorf("结果 = %s，期望 b", got[0].ID)
	}
}

func TestApplyRerankEmpty(t *testing.T) {
	if got := applyRerank(chunks("a"), nil); len(got) != 0 {
		t.Errorf("空 Rerank 结果应返回空，实际 %d 条", len(got))
	}
}
