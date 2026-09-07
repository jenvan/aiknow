package service

import (
	"context"
	"fmt"
	"strings"
)

// PromptService 提示词服务
type PromptService struct{}

// NewPromptService 创建提示词服务
func NewPromptService() *PromptService {
	return &PromptService{}
}

// GetPrompt 获取提示词模板
func (s *PromptService) GetPrompt(ctx context.Context, key string) (string, error) {
	// TODO: 从数据库读取
	defaultPrompts := map[string]string{
		"analyze": "你是一个知识库内容分析师。请阅读以下文章...",
	}
	if prompt, ok := defaultPrompts[key]; ok {
		return prompt, nil
	}
	return "", fmt.Errorf("prompt not found: %s", key)
}

// UpdatePrompt 更新提示词模板
func (s *PromptService) UpdatePrompt(ctx context.Context, key, prompt string) error {
	// 校验提示词模板
	if !strings.Contains(prompt, "{{.Content}}") {
		return fmt.Errorf("prompt must contain {{.Content}}")
	}
	if !strings.Contains(strings.ToLower(prompt), "json") {
		return fmt.Errorf("prompt must contain 'json' for LLM JSON mode")
	}
	// TODO: 保存到数据库
	return nil
}

// PromptResult 提示词测试结果
type PromptResult struct {
	RenderedPrompt    string `json:"rendered_prompt"`
	LLMResponse       string `json:"llm_response,omitempty"`
	ValidationError   string `json:"validation_error,omitempty"`
}

// TestPrompt 测试提示词渲染和 LLM 调用
func (s *PromptService) TestPrompt(ctx context.Context, promptTemplate, articleID string) (*PromptResult, error) {
	// TODO: 实现
	return &PromptResult{RenderedPrompt: promptTemplate}, nil
}
