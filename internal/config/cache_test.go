package config

import (
	"testing"

	"github.com/jenvan/aiknow/internal/model"
)

// 回归：settings 表的 value 是扁平标量（数字、字符串），此前 Reload 用
// map[string]interface{} 解析必然失败，导致 L2 配置从未生效。现在按 key
// 类型直接解码扁平标量。
func TestApplySettingFlatScalar(t *testing.T) {
	c := &ConfigCache{}

	t.Run("字符串标量", func(t *testing.T) {
		cfg := model.DefaultL2Config()
		if err := c.applySetting(&cfg, "llm.model", []byte(`"qwen-max"`)); err != nil {
			t.Fatalf("applySetting: %v", err)
		}
		if cfg.LLM.Model != "qwen-max" {
			t.Errorf("LLM.Model = %q，期望 qwen-max", cfg.LLM.Model)
		}

		if err := c.applySetting(&cfg, "collector.user_agent", []byte(`"Mozilla/5.0"`)); err != nil {
			t.Fatalf("applySetting: %v", err)
		}
		if cfg.Collector.UserAgent != "Mozilla/5.0" {
			t.Errorf("Collector.UserAgent = %q，期望 Mozilla/5.0", cfg.Collector.UserAgent)
		}
	})

	t.Run("数字标量", func(t *testing.T) {
		cfg := model.DefaultL2Config()
		apply := func(key string, raw string) {
			t.Helper()
			if err := c.applySetting(&cfg, key, []byte(raw)); err != nil {
				t.Fatalf("applySetting(%s): %v", key, err)
			}
		}

		apply("llm.temperature", `0.7`)
		if cfg.LLM.Temperature != 0.7 {
			t.Errorf("LLM.Temperature = %v，期望 0.7", cfg.LLM.Temperature)
		}

		apply("llm.max_tokens", `3000`)
		if cfg.LLM.MaxTokens != 3000 {
			t.Errorf("LLM.MaxTokens = %d，期望 3000", cfg.LLM.MaxTokens)
		}

		apply("search.vector_weight", `0.6`)
		if cfg.Search.VectorWeight != 0.6 {
			t.Errorf("Search.VectorWeight = %v，期望 0.6", cfg.Search.VectorWeight)
		}

		apply("chunk.overlap", `80`)
		if cfg.Chunk.Overlap != 80 {
			t.Errorf("Chunk.Overlap = %d，期望 80", cfg.Chunk.Overlap)
		}
	})
}

// 未被识别的 key 应被静默忽略，不影响整批加载
func TestApplySettingUnknownKeyIgnored(t *testing.T) {
	c := &ConfigCache{}
	cfg := model.DefaultL2Config()
	if err := c.applySetting(&cfg, "unknown.key", []byte(`1`)); err != nil {
		t.Fatalf("未知 key 应被忽略，实际报错: %v", err)
	}
	if cfg.LLM.Temperature != model.DefaultL2Config().LLM.Temperature {
		t.Error("未知 key 不应改变已有配置")
	}
}
