package service

import (
	"context"
	"encoding/json"
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
