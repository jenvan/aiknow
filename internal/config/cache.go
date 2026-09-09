package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jenvan/aiknow/internal/model"
)

// ConfigCache L2 配置缓存（热重载）
type ConfigCache struct {
	mu             sync.RWMutex
	config         model.L2Config
	lastReloadTime atomic.Value // time.Time
	db             *pgxpool.Pool
	notifyChan     chan string
}

// NewConfigCache 创建配置缓存
func NewConfigCache(db *pgxpool.Pool) *ConfigCache {
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

	defaultCfg := model.DefaultL2Config()
	cfg := defaultCfg

	// 从 settings 表读取配置
	rows, err := c.db.Query(ctx, `
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

		var value map[string]interface{}
		if err := json.Unmarshal(valueBytes, &value); err != nil {
			return fmt.Errorf("unmarshal setting %s: %w", key, err)
		}

		// 应用到配置
		c.applySetting(&cfg, key, value)
	}

	c.config = cfg
	c.lastReloadTime.Store(time.Now())
	fmt.Printf("Config reloaded at %s\n", time.Now().Format(time.RFC3339))
	return nil
}

// applySetting 应用单个配置项
func (c *ConfigCache) applySetting(cfg *model.L2Config, key string, value map[string]interface{}) {
	switch key {
	case "llm.temperature":
		if v, ok := value["temperature"].(float64); ok {
			cfg.LLM.Temperature = float32(v)
		}
	case "llm.max_tokens":
		if v, ok := value["max_tokens"].(float64); ok {
			cfg.LLM.MaxTokens = int(v)
		}
	case "search.top_k":
		if v, ok := value["top_k"].(float64); ok {
			cfg.Search.TopK = int(v)
		}
	case "search.candidate_k":
		if v, ok := value["candidate_k"].(float64); ok {
			cfg.Search.CandidateK = int(v)
		}
	case "search.rrf_k":
		if v, ok := value["rrf_k"].(float64); ok {
			cfg.Search.RRFK = int(v)
		}
	case "chunk.size":
		if v, ok := value["size"].(float64); ok {
			cfg.Chunk.Size = int(v)
		}
	case "chunk.overlap":
		if v, ok := value["overlap"].(float64); ok {
			cfg.Chunk.Overlap = int(v)
		}
	case "keyword_search.tag_weight":
		if v, ok := value["tag_weight"].(float64); ok {
			cfg.KeywordSearch.TagWeight = v
		}
	case "keyword_search.category_weight":
		if v, ok := value["category_weight"].(float64); ok {
			cfg.KeywordSearch.CategoryWeight = v
		}
	case "keyword_search.title_weight":
		if v, ok := value["title_weight"].(float64); ok {
			cfg.KeywordSearch.TitleWeight = v
		}
	case "keyword_search.summary_weight":
		if v, ok := value["summary_weight"].(float64); ok {
			cfg.KeywordSearch.SummaryWeight = v
		}
	case "keyword_search.content_weight":
		if v, ok := value["content_weight"].(float64); ok {
			cfg.KeywordSearch.ContentWeight = v
		}
	case "collector.user_agent":
		if v, ok := value["user_agent"].(string); ok {
			cfg.Collector.UserAgent = v
		}
	}
}

// LastReloadTime 获取最后重载时间
func (c *ConfigCache) LastReloadTime() time.Time {
	return c.lastReloadTime.Load().(time.Time)
}
