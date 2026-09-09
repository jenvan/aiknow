package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// QwenProvider Qwen LLM Provider 实现
type QwenProvider struct {
	baseURL string
	apiKey  string
	model   string // 启动配置（QWEN_MODEL）
	// modelFn 可选：运行时模型名来源（L2 配置），非空时优先于启动配置
	modelFn func() string
	client  *http.Client
}

// NewQwenProvider 创建 Qwen Provider
func NewQwenProvider(baseURL, apiKey, model string) (*QwenProvider, error) {
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	if apiKey == "" {
		return nil, fmt.Errorf("Qwen API key is required")
	}
	if model == "" {
		model = "qwen-plus"
	}

	return &QwenProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// SetModelSource 注入运行时模型名来源（实现 ModelSource 接口）。
//
// 传入后每次调用从该来源读取当前模型名：非空则覆盖启动配置，可为 nil 还原。
func (q *QwenProvider) SetModelSource(src ModelSource) {
	if src == nil {
		q.modelFn = nil
		return
	}
	q.modelFn = src.LLMModel
}

func (q *QwenProvider) Name() string { return "qwen" }

// chatRequest ChatGPT 兼容格式请求
type chatRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	Temperature    float32   `json:"temperature,omitempty"`
	MaxTokens      int       `json:"max_tokens,omitempty"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

// chatResponse ChatGPT 兼容格式响应
type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (q *QwenProvider) Generate(ctx context.Context, messages []Message, options GenerateOptions) (*LLMResult, error) {
	// 模型名优先级：调用方显式指定 > 运行时来源（L2） > 启动配置（QWEN_MODEL）
	if options.Model == "" {
		options.Model = q.model
		if q.modelFn != nil {
			if m := q.modelFn(); m != "" {
				options.Model = m
			}
		}
	}

	reqBody := chatRequest{
		Model:       options.Model,
		Messages:    messages,
		Temperature: options.Temperature,
		MaxTokens:   options.MaxTokens,
	}

	if options.ResponseFormat == "json_object" {
		reqBody.ResponseFormat = &struct {
			Type string `json:"type"`
		}{Type: "json_object"}
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.baseURL+"/chat/completions", bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+q.apiKey)

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	return &LLMResult{
		Content:     chatResp.Choices[0].Message.Content,
		RawResponse: reqBytes,
		TokenCount:  chatResp.Usage.CompletionTokens,
	}, nil
}

func (q *QwenProvider) GenerateJSON(ctx context.Context, prompt string, schema string, opts GenerateOptions) (json.RawMessage, error) {
	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant. Always respond in JSON format."},
		{Role: "user", Content: prompt},
	}

	// 结构化输出固定 json_object；model / temperature / max_tokens 沿用调用方
	// 传入值，未设时由 q.Generate 回落到启动配置默认。
	opts.ResponseFormat = "json_object"

	result, err := q.Generate(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	var jsonResult json.RawMessage
	if err := json.Unmarshal([]byte(result.Content), &jsonResult); err != nil {
		return nil, fmt.Errorf("parse JSON response: %w", err)
	}

	return jsonResult, nil
}
