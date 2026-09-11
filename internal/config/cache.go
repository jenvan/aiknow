package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jenvan/aiknow/internal/model"
)

// ConfigCache L2 配置缓存（热重载）
//
// settings 表的 value 是扁平标量：数字（0.3、2000）、字符串（"qwen-plus"，JSON
// 编码带引号）或文本。Reload 时按各 key 的已知类型直接解码，不再套一层
// map[string]interface{}（后者对扁平标量必然解码失败，这也是此前 L2 配置
// 从未真正生效的原因）。
type ConfigCache struct {
	mu             sync.RWMutex
	config         model.L2Config
	lastReloadTime atomic.Value // time.Time
	db             *sql.DB
	notifyChan     chan string
}

// NewConfigCache 创建配置缓存
func NewConfigCache(db *sql.DB) *ConfigCache {
	c := &ConfigCache{
		db:         db,
		notifyChan: make(chan string, 10),
	}
	// 加载默认配置
	c.config = model.DefaultL2Config()
	c.lastReloadTime.Store(time.Now())
	return c
}

// Get 获取配置
func (c *ConfigCache) Get() model.L2Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// Reload 从数据库重新加载配置
func (c *ConfigCache) Reload(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cfg := model.DefaultL2Config()

	// 从 settings 表读取配置
	rows, err := c.db.QueryContext(ctx, `
		SELECT "key", value FROM settings
	`)
	if err != nil {
		return fmt.Errorf("query settings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var valueBytes []byte
		if err := rows.Scan(&key, &valueBytes); err != nil {
			return fmt.Errorf("scan setting: %w", err)
		}
		if err := c.applySetting(&cfg, key, valueBytes); err != nil {
			return fmt.Errorf("apply setting %s: %w", key, err)
		}
	}

	c.config = cfg
	c.lastReloadTime.Store(time.Now())
	fmt.Printf("Config reloaded at %s\n", time.Now().Format(time.RFC3339))
	return nil
}

// num JSON 数字 → float64；解析失败返回零值
func num(raw []byte) float64 {
	var v float64
	_ = json.Unmarshal(raw, &v)
	return v
}

// str JSON 字符串 → string；解析失败返回空串
func str(raw []byte) string {
	var v string
	_ = json.Unmarshal(raw, &v)
	return v
}

// applySetting 应用单个配置项（value 为原始 JSON 标量字节）
func (c *ConfigCache) applySetting(cfg *model.L2Config, key string, raw []byte) error {
	switch key {
	case "llm.model":
		cfg.LLM.Model = str(raw)
	case "llm.temperature":
		cfg.LLM.Temperature = float32(num(raw))
	case "llm.max_tokens":
		cfg.LLM.MaxTokens = int(num(raw))
	case "llm.retry_count":
		cfg.LLM.RetryCount = int(num(raw))
	case "llm.timeout_ms":
		cfg.LLM.TimeoutMS = int(num(raw))
	case "embedding.batch_size":
		cfg.Embedding.BatchSize = int(num(raw))
	case "reranker.top_k":
		cfg.Reranker.TopK = int(num(raw))
	case "search.top_k":
		cfg.Search.TopK = int(num(raw))
	case "search.candidate_k":
		cfg.Search.CandidateK = int(num(raw))
	case "search.rrf_k":
		cfg.Search.RRFK = int(num(raw))
	case "search.vector_weight":
		cfg.Search.VectorWeight = num(raw)
	case "search.bm25_weight":
		cfg.Search.BM25Weight = num(raw)
	case "search.ef_search":
		cfg.Search.EFSearch = int(num(raw))
	case "chunk.strategy":
		cfg.Chunk.Strategy = str(raw)
	case "chunk.size":
		cfg.Chunk.Size = int(num(raw))
	case "chunk.overlap":
		cfg.Chunk.Overlap = int(num(raw))
	case "chunk.min_size":
		cfg.Chunk.MinSize = int(num(raw))
	case "mcp_log.retention_days":
		cfg.MCPLog.RetentionDays = int(num(raw))
	case "keyword_search.tag_weight":
		cfg.KeywordSearch.TagWeight = num(raw)
	case "keyword_search.category_weight":
		cfg.KeywordSearch.CategoryWeight = num(raw)
	case "keyword_search.title_weight":
		cfg.KeywordSearch.TitleWeight = num(raw)
	case "keyword_search.summary_weight":
		cfg.KeywordSearch.SummaryWeight = num(raw)
	case "keyword_search.content_weight":
		cfg.KeywordSearch.ContentWeight = num(raw)
	case "collector.user_agent":
		cfg.Collector.UserAgent = str(raw)
	case "collector.max_content_length":
		cfg.Collector.MaxContentLength = int(num(raw))
	}
	return nil
}

// LastReloadTime 获取最后重载时间
func (c *ConfigCache) LastReloadTime() time.Time {
	return c.lastReloadTime.Load().(time.Time)
}

// LLMModel 返回当前生效的 LLM 模型名（L2 覆盖值，空串表示使用 L1 QWEN_MODEL）
func (c *ConfigCache) LLMModel() string {
	return c.Get().LLM.Model
}
