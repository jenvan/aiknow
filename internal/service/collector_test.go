package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jenvan/aiknow/internal/model"
)

// TestExtractTitle 锁住「og:title 有值、<title> 为空」的微信场景及优先级。
func TestExtractTitle(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "微信：title 为空，og:title 有值",
			html: `<html><head><title></title>` +
				`<meta property="og:title" content="一篇讲透Agent自进化飞轮"></head>` +
				`<body><h1 class="rich_media_title"></h1></body></html>`,
			want: "一篇讲透Agent自进化飞轮",
		},
		{
			name: "twitter:title 次级兜底",
			html: `<html><head><meta name="twitter:title" content="T2">` +
				`<title></title></head></html>`,
			want: "T2",
		},
		{
			name: "og:title 优先于 twitter:title",
			html: `<html><head><meta name="twitter:title" content="T2">` +
				`<meta property="og:title" content="T1"></head></html>`,
			want: "T1",
		},
		{
			name: "都无 meta，回落 <title>",
			html: `<html><head><title>FromTitle</title></head></html>`,
			want: "FromTitle",
		},
		{
			name: "微信 #activity-name",
			html: `<html><body><h1 id="activity-name">微信标题</h1></body></html>`,
			want: "微信标题",
		},
		{
			name: "全空",
			html: `<html><head><title></title></head><body></body></html>`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractTitle(tc.html); got != tc.want {
				t.Errorf("extractTitle = %q，期望 %q", got, tc.want)
			}
		})
	}
}

// fakeSettingRepo 实现 SettingRepository 的最小桩，用于 User-Agent 覆盖测试
type fakeSettingRepo struct {
	val *model.Setting
}

func (f *fakeSettingRepo) GetByKey(_ context.Context, _ string) (*model.Setting, error) {
	return f.val, nil
}
func (f *fakeSettingRepo) Upsert(context.Context, *model.Setting) error     { return nil }
func (f *fakeSettingRepo) ListAll(context.Context) ([]model.Setting, error) { return nil, nil }
func (f *fakeSettingRepo) DeleteByKey(context.Context, string) error        { return nil }
func (f *fakeSettingRepo) GetHistory(context.Context, string, int) ([]model.SettingHistory, error) {
	return nil, nil
}

func settingStr(s string) *model.Setting {
	v, _ := json.Marshal(s)
	return &model.Setting{Key: "collector.user_agent", Value: v}
}

func settingInt(i int) *model.Setting {
	v, _ := json.Marshal(i)
	return &model.Setting{Key: "collector.max_content_length", Value: v}
}

func TestCollectorEffectiveUserAgent(t *testing.T) {
	cfg := DefaultCollectorConfig()
	cfg.UserAgent = "L1-Baseline"

	t.Run("无后台覆盖时用启动配置", func(t *testing.T) {
		c := NewCollector(cfg)
		if got := c.effectiveUserAgent(context.Background()); got != "L1-Baseline" {
			t.Errorf("effectiveUserAgent = %q，期望启动配置值", got)
		}
	})

	t.Run("后台覆盖优先", func(t *testing.T) {
		c := NewCollector(cfg)
		c.SetUserAgentSource(&fakeSettingRepo{val: settingStr("Backend-Override")})
		if got := c.effectiveUserAgent(context.Background()); got != "Backend-Override" {
			t.Errorf("effectiveUserAgent = %q，期望后台覆盖值", got)
		}
	})

	t.Run("后台覆盖为空回落启动配置", func(t *testing.T) {
		c := NewCollector(cfg)
		c.SetUserAgentSource(&fakeSettingRepo{val: settingStr("")})
		if got := c.effectiveUserAgent(context.Background()); got != "L1-Baseline" {
			t.Errorf("effectiveUserAgent = %q，空覆盖应回落启动配置", got)
		}
	})
}

func TestCollectorEffectiveMaxContentLength(t *testing.T) {
	cfg := DefaultCollectorConfig() // MaxContentLength = 2MB（L1 默认）
	base := cfg.MaxContentLength

	t.Run("无后台覆盖时用启动配置", func(t *testing.T) {
		c := NewCollector(cfg)
		if got := c.effectiveMaxContentLength(context.Background()); got != base {
			t.Errorf("effectiveMaxContentLength = %d，期望启动配置 %d", got, base)
		}
	})

	t.Run("后台覆盖优先（>0 才生效）", func(t *testing.T) {
		c := NewCollector(cfg)
		c.SetUserAgentSource(&fakeSettingRepo{val: settingInt(5 * 1024 * 1024)})
		if got := c.effectiveMaxContentLength(context.Background()); got != 5*1024*1024 {
			t.Errorf("effectiveMaxContentLength = %d，期望后台覆盖 5MB", got)
		}
	})

	t.Run("后台覆盖为 0 回落启动配置", func(t *testing.T) {
		c := NewCollector(cfg)
		c.SetUserAgentSource(&fakeSettingRepo{val: settingInt(0)})
		if got := c.effectiveMaxContentLength(context.Background()); got != base {
			t.Errorf("effectiveMaxContentLength = %d，0 覆盖应回落启动配置 %d", got, base)
		}
	})
}

// ---- 提取路由（纯函数，不碰网络）----

// TestFetchHTML_RenderFirst 渲染优先：渲染层启用且渲染成功时走渲染路径；
// 渲染失败时降级为轻量 http。（用 httptest 模拟 browserless 与目标页。）
func TestFetchHTML_RenderFirst(t *testing.T) {
	// 目标页（http 降级路径用）
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<html><body><article><p>`+
			strings.Repeat("SSR 正文足够长。", 50)+`</p></article></body></html>`)
	}))
	defer target.Close()

	t.Run("渲染成功走渲染路径", func(t *testing.T) {
		// 模拟 browserless /function：返回渲染后 HTML + base64 截图
		bw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"data":{"html":"<html><body><article><p>`+
				strings.Repeat("渲染正文。", 50)+`</p></article></body></html>","screenshot":"iVBORw0KGgo="}}`)
		}))
		defer bw.Close()

		cfg := DefaultCollectorConfig()
		cfg.RenderEnabled = true
		cfg.BrowserEndpoint = bw.URL
		c := NewCollector(cfg)

		html, shot, rendered, _, err := c.fetchHTML(context.Background(), target.URL, true)
		if err != nil {
			t.Fatalf("fetchHTML: %v", err)
		}
		if !rendered {
			t.Error("渲染成功时 rendered 应为 true")
		}
		if !strings.Contains(html, "渲染正文") {
			t.Errorf("应返回渲染后 HTML，实际: %q", html[:min(60, len(html))])
		}
		if len(shot) == 0 {
			t.Error("应带回截图字节")
		}
	})

	t.Run("渲染失败降级 http", func(t *testing.T) {
		bw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer bw.Close()

		cfg := DefaultCollectorConfig()
		cfg.RenderEnabled = true
		cfg.BrowserEndpoint = bw.URL
		c := NewCollector(cfg)

		html, shot, rendered, _, err := c.fetchHTML(context.Background(), target.URL, true)
		if err != nil {
			t.Fatalf("fetchHTML: %v", err)
		}
		if rendered {
			t.Error("渲染失败时 rendered 应为 false")
		}
		if !strings.Contains(html, "SSR 正文") {
			t.Error("渲染失败应降级为 http 抓取源码")
		}
		if shot != nil {
			t.Error("降级路径不应有截图")
		}
	})

	t.Run("渲染层关闭直接 http", func(t *testing.T) {
		c := NewCollector(DefaultCollectorConfig())
		html, shot, rendered, _, err := c.fetchHTML(context.Background(), target.URL, true)
		if err != nil {
			t.Fatalf("fetchHTML: %v", err)
		}
		if rendered || shot != nil {
			t.Error("渲染层关闭时不应渲染、不应有截图")
		}
		if !strings.Contains(html, "SSR 正文") {
			t.Error("应直接 http 抓源码")
		}
	})

	t.Run("渲染与 http 都失败返回错误", func(t *testing.T) {
		// 渲染端点与目标页都 500：应把 http 的错误向上传，而不是退化成「内容过短」
		bw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer bw.Close()
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer bad.Close()

		cfg := DefaultCollectorConfig()
		cfg.RenderEnabled = true
		cfg.BrowserEndpoint = bw.URL
		c := NewCollector(cfg)

		_, _, _, _, err := c.fetchHTML(context.Background(), bad.URL, true)
		if err == nil {
			t.Error("渲染与 http 都失败时应返回错误")
		}
	})

	t.Run("http 截断标志透传", func(t *testing.T) {
		c := NewCollector(DefaultCollectorConfig())
		c.config.MaxContentLength = 8 // 远小于目标页正文
		_, _, _, truncated, err := c.fetchHTML(context.Background(), target.URL, true)
		if err != nil {
			t.Fatalf("fetchHTML: %v", err)
		}
		if !truncated {
			t.Error("正文超过上限时应标记 truncated")
		}
	})
}

func TestExtractHostRouting(t *testing.T) {
	c := NewCollector(DefaultCollectorConfig())

	// 微信 host → htmlclean
	wechat := `<html><head><meta property="og:title" content="微信文"></head>` +
		`<body><div id="js_content"><p>微信正文内容一，足够长以通过最小长度校验的一段话。</p>` +
		strings.Repeat("微信正文。", 30) + `</div></body></html>`
	got := c.extract("https://mp.weixin.qq.com/s/abc", wechat)
	if got.Text == "" {
		t.Error("微信页经 htmlclean 应能提取到正文")
	}
	if got.Title != "微信文" {
		t.Errorf("微信页标题 = %q，期望 og:title", got.Title)
	}

	// 非微信 host 且非文章 → 回退 htmlclean（不报错）
	nonArticle := `<html><body><div id="root">You need JS</div></body></html>`
	got2 := c.extract("https://example.com/", nonArticle)
	if got2.Text == "" && got2.Markdown == "" {
		t.Error("非文章页回退后仍应有内容（htmlclean 兜底）")
	}
}

func TestHostOf(t *testing.T) {
	if got := hostOf("https://MP.Weixin.QQ.COM/s/x"); got != "mp.weixin.qq.com" {
		t.Errorf("hostOf 小写 host = %q，期望 mp.weixin.qq.com", got)
	}
	if got := hostOf("not a url"); got != "" {
		t.Errorf("非法 URL hostOf 应返回空串，实际 %q", got)
	}
}

func TestIsWechatHost(t *testing.T) {
	cases := map[string]bool{
		"mp.weixin.qq.com":  true,
		"servicewechat.com": false,
		"example.com":       false,
	}
	for host, want := range cases {
		if got := isWechatHost(host); got != want {
			t.Errorf("isWechatHost(%q) = %v，期望 %v", host, got, want)
		}
	}
}
