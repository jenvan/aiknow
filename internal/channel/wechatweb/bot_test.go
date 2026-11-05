package wechatweb

import (
	"context"
	"testing"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/pkg/wechatweb"
)

type fakeCreator struct {
	urls []string
	err  error
}

func (f *fakeCreator) CreateFromURL(_ context.Context, url string) (*model.Article, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.urls = append(f.urls, url)
	return &model.Article{ID: "art-1", Title: "测试文章", ProcessingStatus: "pending"}, nil
}

type fakeNotifier struct {
	ingested   int
	duplicates int
	lastTitle  string
}

func (f *fakeNotifier) Ingested(_ context.Context, a *model.Article) {
	f.ingested++
	f.lastTitle = a.Title
}

func (f *fakeNotifier) Duplicate(context.Context, string) { f.duplicates++ }

func newTestRunner(c ArticleCreator, n Notifier) *Runner {
	r := NewRunner(nil, nil, c, nil)
	r.seen = map[string]time.Time{}
	r.notifier = n
	return r
}

func TestHandleArticleCard(t *testing.T) {
	c := &fakeCreator{}
	n := &fakeNotifier{}
	r := newTestRunner(c, n)

	r.handle(context.Background(), &wechatweb.Message{
		MsgID: "1", MsgType: 49, AppMsg: 5,
		URL:   "https://mp.weixin.qq.com/s?__biz=X&mid=1&idx=1&sn=abc#rd",
		Title: "卡片",
	})

	if len(c.urls) != 1 {
		t.Fatalf("未触发入库: %v", c.urls)
	}
	if n.ingested != 1 || n.lastTitle != "测试文章" {
		t.Errorf("回执未发送: %+v", n)
	}
}

func TestHandleTextWithWeixinLink(t *testing.T) {
	c := &fakeCreator{}
	r := newTestRunner(c, &fakeNotifier{})

	r.handle(context.Background(), &wechatweb.Message{
		MsgID: "2", MsgType: 1, Text: "看看这篇 https://mp.weixin.qq.com/s/abc 很不错",
	})
	if len(c.urls) != 1 || c.urls[0] != "https://mp.weixin.qq.com/s/abc" {
		t.Fatalf("文本里的链接未提取: %v", c.urls)
	}
}

func TestHandleIgnoresNonWeixinAndOtherTypes(t *testing.T) {
	c := &fakeCreator{}
	r := newTestRunner(c, &fakeNotifier{})

	cases := []*wechatweb.Message{
		{MsgID: "3", MsgType: 49, URL: "https://example.com/a"}, // 非公众号
		{MsgID: "4", MsgType: 1, Text: "https://example.com/b"}, // 非公众号文本
		{MsgID: "5", MsgType: 34, Text: "语音"},                   // 语音
		{MsgID: "6", MsgType: 3, URL: ""},                       // 图片
		{MsgID: "7", MsgType: 1, Text: "纯文字，没有链接"},              // 纯文本
	}
	for _, m := range cases {
		r.handle(context.Background(), m)
	}
	if len(c.urls) != 0 {
		t.Fatalf("不该入库: %v", c.urls)
	}
}

func TestHandleDedupeByMsgID(t *testing.T) {
	c := &fakeCreator{}
	r := newTestRunner(c, &fakeNotifier{})
	m := &wechatweb.Message{MsgID: "same", MsgType: 49, URL: "https://mp.weixin.qq.com/s/a"}

	r.handle(context.Background(), m)
	r.handle(context.Background(), m)
	if len(c.urls) != 1 {
		t.Fatalf("同一 MsgId 应只处理一次, got %d", len(c.urls))
	}
}

func TestHandleDuplicateNotifies(t *testing.T) {
	c := &fakeCreator{err: ErrDuplicate}
	n := &fakeNotifier{}
	r := newTestRunner(c, n)

	r.handle(context.Background(), &wechatweb.Message{
		MsgID: "8", MsgType: 49, URL: "https://mp.weixin.qq.com/s/dup",
	})
	if n.duplicates != 1 || n.ingested != 0 {
		t.Errorf("重复回执不对: %+v", n)
	}
}
