package service

import (
	"testing"

	"github.com/jenvan/aiknow/internal/model"
)

// fakeL2Source 实现 L2ConfigSource 的最小桩
type fakeL2Source struct {
	cfg model.L2Config
}

func (f fakeL2Source) Get() model.L2Config { return f.cfg }

// 检索参数在未显式指定时应取 L2 值（后台配置优先于内置默认）
func TestSearchServiceDefaultsFromL2(t *testing.T) {
	l2 := model.DefaultL2Config()
	l2.Search.CandidateK = 77
	l2.Search.RRFK = 45
	l2.Search.TopK = 9
	l2.Search.VectorWeight = 0.6
	l2.Search.BM25Weight = 0.4
	l2.Search.EFSearch = 150

	s := NewSearchService(nil, nil, nil, nil, nil, nil, fakeL2Source{cfg: l2}).(*SearchServiceImpl)

	p := model.SearchParams{}
	s.applyDefaults(&p)
	if p.CandidateK != 77 || p.RRFK != 45 || p.TopK != 9 ||
		p.VectorWeight != 0.6 || p.BM25Weight != 0.4 || p.EFSearch != 150 {
		t.Errorf("L2 检索参数未生效: %+v", p)
	}

	// 显式指定的参数不应被 L2 覆盖
	p2 := model.SearchParams{TopK: 3, CandidateK: 10}
	s.applyDefaults(&p2)
	if p2.TopK != 3 || p2.CandidateK != 10 {
		t.Errorf("显式参数被覆盖: %+v", p2)
	}

	// 无 L2 来源时回落内置默认
	s2 := NewSearchService(nil, nil, nil, nil, nil, nil).(*SearchServiceImpl)
	p3 := model.SearchParams{}
	s2.applyDefaults(&p3)
	if p3.TopK != model.DefaultL2Config().Search.TopK {
		t.Errorf("无来源时应用内置默认，实际 %d", p3.TopK)
	}
}

// 关键词权重在未显式指定时应取 L2 值
func TestKeywordSearchDefaultsFromL2(t *testing.T) {
	l2 := model.DefaultL2Config()
	l2.KeywordSearch.TagWeight = 12.5
	l2.KeywordSearch.CategoryWeight = 9.5
	l2.KeywordSearch.ContentWeight = 2.2

	s := NewKeywordSearchService(nil, nil, nil, fakeL2Source{cfg: l2}).(*KeywordSearchServiceImpl)

	p := model.KeywordSearchParams{}
	s.applyDefaults(&p)
	if p.TagWeight != 12.5 || p.CategoryWeight != 9.5 || p.ContentWeight != 2.2 {
		t.Errorf("L2 权重未生效: %+v", p)
	}

	// 显式指定的权重不被覆盖
	p2 := model.KeywordSearchParams{TagWeight: 1.0}
	s.applyDefaults(&p2)
	if p2.TagWeight != 1.0 {
		t.Errorf("显式权重被覆盖: %+v", p2)
	}
}

// 送入精排的候选数上限取自 L2 reranker.top_k
func TestSearchServiceRerankCandidateLimit(t *testing.T) {
	l2 := model.DefaultL2Config()
	l2.Reranker.TopK = 12

	s := NewSearchService(nil, nil, nil, nil, nil, nil, fakeL2Source{cfg: l2}).(*SearchServiceImpl)
	if got := s.rerankCandidateLimit(); got != 12 {
		t.Errorf("rerankCandidateLimit = %d，期望 12", got)
	}

	// 无来源时不限制
	s2 := NewSearchService(nil, nil, nil, nil, nil, nil).(*SearchServiceImpl)
	if got := s2.rerankCandidateLimit(); got != 0 {
		t.Errorf("无来源时应为 0（不限制），实际 %d", got)
	}
}

// AIPipeline 的 LLM 参数（model/temperature/max_tokens）应取 L2 值
func TestAIPipelineGenerateOptionsFromL2(t *testing.T) {
	l2 := model.DefaultL2Config()
	l2.LLM.Model = "qwen-max"
	l2.LLM.Temperature = 0.9
	l2.LLM.MaxTokens = 4000

	p := NewAIPipeline(nil, nil, nil, nil, nil, nil)
	def := model.DefaultL2Config()
	if opts := p.generateOptions(); opts.Model != "" || opts.Temperature != def.LLM.Temperature || opts.MaxTokens != def.LLM.MaxTokens {
		t.Errorf("未注入时应用内置默认，实际 %+v", opts)
	}

	p.SetConfigSource(fakeL2Source{cfg: l2})
	opts := p.generateOptions()
	if opts.Model != "qwen-max" || opts.Temperature != 0.9 || opts.MaxTokens != 4000 {
		t.Errorf("L2 LLM 参数未生效: %+v", opts)
	}
}
