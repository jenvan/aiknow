package model

import (
	"encoding/json"
	"time"
)

// Setting 系统配置
type Setting struct {
	ID  string `json:"id" db:"id"`
	Key string `json:"key" db:"key"`
	// Value 为任意 JSON 值，不限于对象。
	//
	// 迁移 005 播种的配置项是扁平标量（'llm.temperature' → 0.3，
	// 'llm.max_tokens' → 2000）。此前该字段声明为 map[string]interface{}，
	// 使 ListAll/GetByKey 对每一条播种数据都报
	// "cannot unmarshal number into Go value of type map[string]interface{}"，
	// 系统配置接口完全不可用。改用 RawMessage 以容纳标量、数组与对象。
	Value       json.RawMessage `json:"value" db:"value"`
	Description *string         `json:"description,omitempty" db:"description"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// SettingHistory 配置变更记录
type SettingHistory struct {
	ID           string          `json:"id" db:"id"`
	SettingID    string          `json:"setting_id" db:"setting_id"`
	OldValue     json.RawMessage `json:"old_value" db:"old_value"`
	NewValue     json.RawMessage `json:"new_value" db:"new_value"`
	ChangedBy    *string         `json:"changed_by,omitempty" db:"changed_by"`
	ChangeReason *string         `json:"change_reason,omitempty" db:"change_reason"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

// L2Config L2 业务配置（内存缓存）
type L2Config struct {
	LLM struct {
		Temperature float32 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
		RetryCount  int     `json:"retry_count"`
		TimeoutMS   int     `json:"timeout_ms"`
	} `json:"llm"`
	Embedding struct {
		Dimension int `json:"dimension"`
		BatchSize int `json:"batch_size"`
	} `json:"embedding"`
	Reranker struct {
		TopK int `json:"top_k"`
	} `json:"reranker"`
	Search struct {
		TopK         int     `json:"top_k"`
		CandidateK   int     `json:"candidate_k"`
		RRFK         int     `json:"rrf_k"`
		VectorWeight float64 `json:"vector_weight"`
		BM25Weight   float64 `json:"bm25_weight"`
		EFSearch     int     `json:"ef_search"`
	} `json:"search"`
	Chunk struct {
		Size    int `json:"size"`
		Overlap int `json:"overlap"`
	} `json:"chunk"`
	MCPLog struct {
		RetentionDays int `json:"retention_days"`
	} `json:"mcp_log"`
	KeywordSearch struct {
		TagWeight      float64 `json:"tag_weight"`
		CategoryWeight float64 `json:"category_weight"`
		TitleWeight    float64 `json:"title_weight"`
		SummaryWeight  float64 `json:"summary_weight"`
		ContentWeight  float64 `json:"content_weight"`
	} `json:"keyword_search"`
	Collector struct {
		TimeoutMS        int `json:"timeout_ms"`
		MaxRedirects     int `json:"max_redirects"`
		MinContentLength int `json:"min_content_length"`
	} `json:"collector"`
	Prompt struct {
		Analyze string `json:"analyze"`
	} `json:"prompt"`
}

// DefaultL2Config 返回内置默认配置
func DefaultL2Config() L2Config {
	return L2Config{
		LLM: struct {
			Temperature float32 `json:"temperature"`
			MaxTokens   int     `json:"max_tokens"`
			RetryCount  int     `json:"retry_count"`
			TimeoutMS   int     `json:"timeout_ms"`
		}{
			Temperature: 0.3,
			MaxTokens:   2000,
			RetryCount:  3,
			TimeoutMS:   30000,
		},
		Embedding: struct {
			Dimension int `json:"dimension"`
			BatchSize int `json:"batch_size"`
		}{
			Dimension: 1024,
			BatchSize: 10,
		},
		Reranker: struct {
			TopK int `json:"top_k"`
		}{
			TopK: 20,
		},
		Search: struct {
			TopK         int     `json:"top_k"`
			CandidateK   int     `json:"candidate_k"`
			RRFK         int     `json:"rrf_k"`
			VectorWeight float64 `json:"vector_weight"`
			BM25Weight   float64 `json:"bm25_weight"`
			EFSearch     int     `json:"ef_search"`
		}{
			TopK:         5,
			CandidateK:   20,
			RRFK:         60,
			VectorWeight: 0.5,
			BM25Weight:   0.5,
			EFSearch:     100,
		},
		Chunk: struct {
			Size    int `json:"size"`
			Overlap int `json:"overlap"`
		}{
			Size:    400,
			Overlap: 50,
		},
		MCPLog: struct {
			RetentionDays int `json:"retention_days"`
		}{
			RetentionDays: 30,
		},
		KeywordSearch: struct {
			TagWeight      float64 `json:"tag_weight"`
			CategoryWeight float64 `json:"category_weight"`
			TitleWeight    float64 `json:"title_weight"`
			SummaryWeight  float64 `json:"summary_weight"`
			ContentWeight  float64 `json:"content_weight"`
		}{
			TagWeight:      10.0,
			CategoryWeight: 8.0,
			TitleWeight:    3.0,
			SummaryWeight:  1.5,
			ContentWeight:  1.0,
		},
		Collector: struct {
			TimeoutMS        int `json:"timeout_ms"`
			MaxRedirects     int `json:"max_redirects"`
			MinContentLength int `json:"min_content_length"`
		}{
			TimeoutMS:        10000,
			MaxRedirects:     3,
			MinContentLength: 200,
		},
		Prompt: struct {
			Analyze string `json:"analyze"`
		}{
			Analyze: `你是一个知识库内容分析师。请阅读以下文章，并提取关键信息。

# 文章标题
{{.Title}}

# 文章正文
{{.Content}}

# 现有分类列表
{{.Categories}}

请以 JSON 格式返回分析结果，必须包含以下字段：
- summary: 100-300 字的摘要
- category: 从上述分类列表中选择一个最合适的分类
- tags: 3-10 个关键词标签，每个标签不超过 20 字

JSON 格式示例：
{
  "summary": "...",
  "category": "...",
  "tags": ["tag1", "tag2", "tag3"]
}

注意：返回必须是合法的 JSON，不要包含任何其他文字。`,
		},
	}
}
