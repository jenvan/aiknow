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
