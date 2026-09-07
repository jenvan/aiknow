package provider

import (
	"context"
	"encoding/json"
	"fmt"
)

// LLMResult LLM 调用结果
type LLMResult struct {
	Content     string          `json:"content"`
	RawResponse json.RawMessage `json:"raw_response"`
	TokenCount  int             `json:"token_count"`
}

// LLMProvider LLM Provider 接口
type LLMProvider interface {
	// Generate 生成文本
	Generate(ctx context.Context, messages []Message, options GenerateOptions) (*LLMResult, error)

	// GenerateJSON 生成结构化 JSON 输出
	GenerateJSON(ctx context.Context, prompt string, schema string) (json.RawMessage, error)

	// Name 返回 provider 名称
	Name() string
}

// Message LLM 消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GenerateOptions 生成选项
type GenerateOptions struct {
	Model          string  `json:"model"`
	Temperature    float32 `json:"temperature"`
	MaxTokens      int     `json:"max_tokens"`
	ResponseFormat string  `json:"response_format"` // json_object | text
}

// MockLLM Mock LLM 实现（用于开发和测试）
type MockLLM struct {
	Response string
	Error    error
}

func (m *MockLLM) Name() string { return "mock" }

func (m *MockLLM) Generate(ctx context.Context, messages []Message, options GenerateOptions) (*LLMResult, error) {
	if m.Error != nil {
		return nil, m.Error
	}

	content := m.Response
	if content == "" {
		content = "这是 Mock 回复"
	}

	return &LLMResult{
		Content:     content,
		RawResponse: json.RawMessage(fmt.Sprintf(`{"content": %q}`, content)),
		TokenCount:  10,
	}, nil
}

func (m *MockLLM) GenerateJSON(ctx context.Context, prompt string, schema string) (json.RawMessage, error) {
	if m.Error != nil {
		return nil, m.Error
	}

	// 返回合法的 JSON
	result := map[string]interface{}{
		"summary":  "这是 Mock 摘要",
		"category": "技术",
		"tags":     []string{"mock", "test"},
	}

	return json.Marshal(result)
}
