package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jenvan/aiknow/internal/model"
)

type fakeSettingRepo struct {
	stored *model.Setting
}

func (f *fakeSettingRepo) GetByKey(_ context.Context, _ string) (*model.Setting, error) {
	return f.stored, nil
}
func (f *fakeSettingRepo) Upsert(_ context.Context, s *model.Setting) error { f.stored = s; return nil }
func (f *fakeSettingRepo) ListAll(context.Context) ([]model.Setting, error) {
	if f.stored == nil {
		return nil, nil
	}
	return []model.Setting{*f.stored}, nil
}
func (f *fakeSettingRepo) DeleteByKey(context.Context, string) error { f.stored = nil; return nil }
func (f *fakeSettingRepo) GetHistory(context.Context, string, int) ([]model.SettingHistory, error) {
	return nil, nil
}

// 回归：UpdateSetting 此前把整个请求体 {"value": ...} 存进设置，存储结果变成
// 外层对象，消费方读不到实际值。现在应只存储内层 value。
func TestUpdateSettingStoresInnerValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeSettingRepo{}
	r := gin.New()
	h := NewSettingHandler(nil, repo)
	r.PUT("/settings/:key", h.UpdateSetting)

	req := httptest.NewRequest(http.MethodPut, "/settings/collector.user_agent",
		bytes.NewBufferString(`{"value":"Mozilla/5.0 (X11; Linux x86_64)"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200（响应: %s）", w.Code, w.Body.String())
	}
	if repo.stored == nil {
		t.Fatal("设置未被保存")
	}

	var got string
	if err := json.Unmarshal(repo.stored.Value, &got); err != nil {
		t.Fatalf("存储值应能被解析为字符串: %v（存储内容: %s）", err, repo.stored.Value)
	}
	if got != "Mozilla/5.0 (X11; Linux x86_64)" {
		t.Errorf("存储值 = %q，期望内层 value", got)
	}
}