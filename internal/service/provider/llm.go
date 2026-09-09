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

	// GenerateJSON 生成结构化 JSON 输出。
	// opts 传入调用方准备好的 LLM 参数（model / temperature / max_tokens），
	// 空值由各 Provider 用自身默认。
	GenerateJSON(ctx context.Context, prompt string, schema string, opts GenerateOptions) (json.RawMessage, error)

	// Name 返回 provider 名称
	Name() string
}

// ModelSource 运行时模型名来源（L2 配置）。
//
// 实现方返回当前生效的模型名；返回空串时回落启动配置（QWEN_MODEL）。
// QwenProvider 通过 SetModelSource 注入，实现后台在线切换模型。
type ModelSource interface {
	LLMModel() string
}

// ModelSourceSetter 支持注入运行时模型名来源的 Provider。
//
// 通过类型断言识别：main.go 把 L2 ConfigCache 注入 QwenProvider。
type ModelSourceSetter interface {
	SetModelSource(src ModelSource)
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

func (m *MockLLM) GenerateJSON(ctx context.Context, prompt string, schema string, opts GenerateOptions) (json.RawMessage, error) {
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
