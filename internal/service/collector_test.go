package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jenvan/aiknow/internal/model"
)

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
