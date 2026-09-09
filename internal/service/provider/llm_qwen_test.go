package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQwenProviderUsesConfiguredModel(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		captured = body.Model
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"completion_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)

	p, err := NewQwenProvider(srv.URL, "test-key", "qwen-max")
	if err != nil {
		t.Fatalf("NewQwenProvider: %v", err)
	}

	// 未显式指定模型时回落启动配置的模型名
	if _, err := p.Generate(context.Background(), []Message{{Role: "user", Content: "hi"}}, GenerateOptions{}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if captured != "qwen-max" {
		t.Errorf("未显式指定模型时应使用启动配置模型，实际 %q", captured)
	}

	// 显式指定模型时优先使用传入值
	if _, err := p.Generate(context.Background(), []Message{{Role: "user", Content: "hi"}}, GenerateOptions{Model: "qwen-turbo"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if captured != "qwen-turbo" {
		t.Errorf("显式模型应优先，实际 %q", captured)
	}
}

func TestNewQwenProviderDefaultsModel(t *testing.T) {
	p, err := NewQwenProvider("", "test-key", "")
	if err != nil {
		t.Fatalf("NewQwenProvider: %v", err)
	}
	if p.model != "qwen-plus" {
		t.Errorf("空模型应回落 qwen-plus，实际 %q", p.model)
	}
}

// L2 运行时模型源优先于启动配置（后台 llm.model → 覆盖 QWEN_MODEL）
func TestQwenProviderModelSourceOverridesStartupConfig(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured = body.Model
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"completion_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)

	// 启动配置为 qwen-max，运行时源返回 qwen-turbo
	p, err := NewQwenProvider(srv.URL, "test-key", "qwen-max")
	if err != nil {
		t.Fatalf("NewQwenProvider: %v", err)
	}
	p.SetModelSource(modelSourceFunc(func() string { return "qwen-turbo" }))

	if _, err := p.Generate(context.Background(), []Message{{Role: "user", Content: "hi"}}, GenerateOptions{}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if captured != "qwen-turbo" {
		t.Errorf("运行时模型源应覆盖启动配置，实际 %q", captured)
	}

	// 运行时源返回空串时回落启动配置
	p.SetModelSource(modelSourceFunc(func() string { return "" }))
	if _, err := p.Generate(context.Background(), []Message{{Role: "user", Content: "hi"}}, GenerateOptions{}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if captured != "qwen-max" {
		t.Errorf("空运行时源应回落启动配置，实际 %q", captured)
	}
}

type modelSourceFunc func() string

func (f modelSourceFunc) LLMModel() string { return f() }
