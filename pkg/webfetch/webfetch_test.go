package webfetch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRender_HtmlResponse 模拟 browserless /content 直接返回渲染后 HTML。
func TestRender_HtmlResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s，期望 POST", r.Method)
		}
		if r.URL.Path != "/content" {
			t.Errorf("path = %s，期望 /content", r.URL.Path)
		}
		if got := r.Header.Get("x-access-token"); got != "secret" {
			t.Errorf("token = %q，期望 secret", got)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "https://example.com") {
			t.Errorf("请求体未包含 url: %s", string(body))
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><body><div id="root">Rendered</div></body></html>`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Token: "secret", WaitUntil: ""})
	if !c.Enabled() {
		t.Fatal("配置了 BaseURL，Enabled 应为 true")
	}
	res, err := c.Render(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Render 失败: %v", err)
	}
	if !strings.Contains(res.HTML, "Rendered") {
		t.Errorf("渲染结果不含正文: %q", res.HTML)
	}
	if res.FinalURL != "https://example.com" {
		t.Errorf("FinalURL = %q", res.FinalURL)
	}
}

// TestRender_JsonWrapped 模拟 browserless /content 返回 JSON 包装形态。
func TestRender_JsonWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":"<body>ok</body>","page":{"url":"https://final.example.com/"}}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	res, err := c.Render(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Render 失败: %v", err)
	}
	if res.FinalURL != "https://final.example.com/" {
		t.Errorf("JSON 包装下 FinalURL = %q，期望 page.url", res.FinalURL)
	}
}

// TestRender_Disabled 未配置端点时报错。
func TestRender_Disabled(t *testing.T) {
	c := New(Config{})
	if c.Enabled() {
		t.Fatal("未配置 BaseURL，Enabled 应为 false")
	}
	if _, err := c.Render(context.Background(), "https://example.com"); err == nil {
		t.Error("未配置端点时 Render 应报错")
	}
}

// TestRender_HttpError 5xx 时返回可读错误。
func TestRender_HttpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "queue full", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	if _, err := c.Render(context.Background(), "https://example.com"); err == nil {
		t.Error("5xx 应报错")
	}
}

// TestRenderAndScreenshot 验证 /function 端点一次返回 HTML + base64 截图。
func TestRenderAndScreenshot(t *testing.T) {
	// 1x1 PNG 的 base64
	pngB64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/function" {
			t.Errorf("path = %s，期望 /function", r.URL.Path)
		}
		// /function 从查询参数取令牌
		if got := r.URL.Query().Get("token"); got != "tk" {
			t.Errorf("token = %q，期望 tk", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"html":"<body>ok</body>","screenshot":"`+pngB64+`"}}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Token: "tk"})
	res, err := c.RenderAndScreenshot(context.Background(), "https://example.com", "MyUA/1.0")
	if err != nil {
		t.Fatalf("RenderAndScreenshot 失败: %v", err)
	}
	if !strings.Contains(res.HTML, "ok") {
		t.Errorf("HTML = %q", res.HTML)
	}
	if len(res.Screenshot) == 0 {
		t.Error("未解出截图字节")
	}
	// 校验 PNG 魔数
	if len(res.Screenshot) >= 4 && string(res.Screenshot[1:4]) != "PNG" {
		t.Errorf("截图非 PNG，前 4 字节 %v", res.Screenshot[:4])
	}
}

// TestRenderAndScreenshot_ForwardsUserAgent 校验 UA 被传进 /function 的 context。
// 反爬站点（微信）依赖此 UA，缺了会拿到验证页。
func TestRenderAndScreenshot_ForwardsUserAgent(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"html":"<body>ok</body>","screenshot":""}}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	if _, err := c.RenderAndScreenshot(context.Background(), "https://mp.weixin.qq.com/s/x", "iPhone-UA"); err != nil {
		t.Fatalf("RenderAndScreenshot 失败: %v", err)
	}
	if !strings.Contains(gotBody, `"userAgent":"iPhone-UA"`) {
		t.Errorf("请求体未携带 userAgent，实际: %s", gotBody)
	}
	if !strings.Contains(gotBody, "setUserAgent") {
		t.Error("function 代码应调用 setUserAgent")
	}
}

// TestRenderAndScreenshot_ScreenshotDecodeFail 截图 base64 损坏时应仍返回 HTML。
func TestRenderAndScreenshot_ScreenshotDecodeFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"html":"<body>ok</body>","screenshot":"!!!not-base64!!!"}}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	res, err := c.RenderAndScreenshot(context.Background(), "https://example.com", "")
	if err == nil {
		t.Error("截图解码失败应报错")
	}
	if res == nil || res.HTML == "" {
		t.Error("截图解码失败时仍应返回 HTML")
	}
}

// TestIdleTimeoutLinear 锁定子超时随总超时线性变化（T/3，不封顶）。
// 之前封顶 12s 会让调大 COLLECTOR_BROWSER_TIMEOUT 后等待时间不再增长。
func TestIdleTimeoutLinear(t *testing.T) {
	cases := []struct {
		total time.Duration
		want  time.Duration
	}{
		{30 * time.Second, 10 * time.Second},
		{60 * time.Second, 20 * time.Second},
		{90 * time.Second, 30 * time.Second},
		{6 * time.Second, 3 * time.Second}, // 下限 3s：6/3=2 → 提到 3s
		{3 * time.Second, 3 * time.Second}, // 下限
	}
	for _, c := range cases {
		if got := idleTimeout(c.total); got != c.want {
			t.Errorf("idleTimeout(%s) = %s，期望 %s", c.total, got, c.want)
		}
	}
}
