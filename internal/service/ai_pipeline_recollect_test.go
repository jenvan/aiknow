package service

import (
	"context"
	"testing"

	"github.com/jenvan/aiknow/internal/model"
)

// fakeURLCollector 假采集器：返回预设结果或错误。
type fakeURLCollector struct {
	result *CollectorResult
	err    error
	called bool
	gotURL string
}

func (f *fakeURLCollector) Fetch(_ context.Context, url string) (*CollectorResult, error) {
	f.called = true
	f.gotURL = url
	return f.result, f.err
}

// fakeScreenshotStore 假截图存储。
type fakeScreenshotStore struct {
	enabled bool
	called  bool
	gotID   string
	gotData []byte
	retURL  string
	err     error
}

func (f *fakeScreenshotStore) Enabled() bool { return f.enabled }

func (f *fakeScreenshotStore) UploadScreenshot(_ context.Context, id string, data []byte) (string, error) {
	f.called = true
	f.gotID = id
	f.gotData = data
	return f.retURL, f.err
}

func strp(s string) *string { return &s }

func TestRecollectIfURL(t *testing.T) {
	t.Run("URL 文章：Markdown 覆盖正文 + 截图上传", func(t *testing.T) {
		coll := &fakeURLCollector{result: &CollectorResult{
			Content:    "纯文本正文",
			Markdown:   "## 标题\n\n正文段落",
			Screenshot: []byte("PNGDATA"),
		}}
		store := &fakeScreenshotStore{enabled: true, retURL: "https://cos.example.com/a.png"}

		p := &AIPipeline{}
		p.SetCollector(coll)
		p.SetScreenshotStore(store)

		article := &model.Article{ID: "art-1", SourceType: "url", SourceURL: strp("https://x.com/a")}
		p.recollectIfURL(context.Background(), article)

		if !coll.called {
			t.Fatal("URL 文章应触发重采集")
		}
		if article.Content != "## 标题\n\n正文段落" {
			t.Errorf("正文应被 Markdown 覆盖，实际 %q", article.Content)
		}
		if !store.called {
			t.Fatal("有截图且存储启用时应上传")
		}
		if store.gotID != "art-1" {
			t.Errorf("上传 ID = %q", store.gotID)
		}
		if article.ScreenshotURL == nil || *article.ScreenshotURL != "https://cos.example.com/a.png" {
			t.Errorf("ScreenshotURL 未写回: %v", article.ScreenshotURL)
		}
	})

	t.Run("手动文章：不重采集", func(t *testing.T) {
		coll := &fakeURLCollector{result: &CollectorResult{Markdown: "x"}}
		p := &AIPipeline{}
		p.SetCollector(coll)

		article := &model.Article{ID: "art-2", SourceType: "manual"}
		p.recollectIfURL(context.Background(), article)

		if coll.called {
			t.Error("手动文章不应触发重采集")
		}
	})

	t.Run("采集失败：保留原正文，不阻断", func(t *testing.T) {
		coll := &fakeURLCollector{err: context.DeadlineExceeded}
		p := &AIPipeline{}
		p.SetCollector(coll)

		article := &model.Article{ID: "art-3", SourceType: "url", SourceURL: strp("https://x.com/b"), Content: "原正文"}
		p.recollectIfURL(context.Background(), article)

		if article.Content != "原正文" {
			t.Errorf("采集失败应保留原正文，实际 %q", article.Content)
		}
	})

	t.Run("无 Markdown 时退回纯文本", func(t *testing.T) {
		coll := &fakeURLCollector{result: &CollectorResult{Content: "仅纯文本"}}
		p := &AIPipeline{}
		p.SetCollector(coll)

		article := &model.Article{ID: "art-4", SourceType: "url", SourceURL: strp("https://x.com/c")}
		p.recollectIfURL(context.Background(), article)

		if article.Content != "仅纯文本" {
			t.Errorf("无 Markdown 应退回纯文本，实际 %q", article.Content)
		}
	})

	t.Run("存储未启用：不尝试上传", func(t *testing.T) {
		coll := &fakeURLCollector{result: &CollectorResult{Markdown: "md", Screenshot: []byte("PNG")}}
		store := &fakeScreenshotStore{enabled: false}
		p := &AIPipeline{}
		p.SetCollector(coll)
		p.SetScreenshotStore(store)

		article := &model.Article{ID: "art-5", SourceType: "url", SourceURL: strp("https://x.com/d")}
		p.recollectIfURL(context.Background(), article)

		if store.called {
			t.Error("存储未启用不应上传")
		}
		if article.ScreenshotURL != nil {
			t.Error("未上传时 ScreenshotURL 应为空")
		}
	})

	t.Run("无采集器：直接返回", func(t *testing.T) {
		p := &AIPipeline{}
		article := &model.Article{ID: "art-6", SourceType: "url", SourceURL: strp("https://x.com/e"), Content: "原"}
		p.recollectIfURL(context.Background(), article)
		if article.Content != "原" {
			t.Error("无采集器时不应改动正文")
		}
	})
}
