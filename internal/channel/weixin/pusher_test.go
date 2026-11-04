package weixin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jenvan/aiknow/internal/model"
)

type fakeSender struct {
	sent []string
	to   string
	tok  string
}

func (f *fakeSender) SendText(_ context.Context, toUserID, contextToken, text string) error {
	f.sent = append(f.sent, text)
	f.to = toUserID
	f.tok = contextToken
	return nil
}

func TestPusherDeliversOnlyRegisteredArticles(t *testing.T) {
	summary := "这是摘要"
	url := "https://mp.weixin.qq.com/s/abc"
	arts := &fakeArticles{byID: map[string]*model.Article{
		"art-1": {
			ID: "art-1", Title: "被采集的文章", Summary: &summary,
			CategoryName: "技术", Tags: []model.Tag{{Name: "golang"}},
			SourceURL: &url,
		},
	}}
	sender := &fakeSender{}
	p := NewPusher(arts, nil)
	p.Attach(sender)

	p.Expect("art-1", "u@im.wechat", "ctx-1")
	p.Deliver(context.Background(), "art-1")

	if len(sender.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(sender.sent))
	}
	msg := sender.sent[0]
	for _, want := range []string{"处理完成", "被采集的文章", "这是摘要", "分类 技术", "golang", url} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if sender.to != "u@im.wechat" || sender.tok != "ctx-1" {
		t.Errorf("target = %s/%s", sender.to, sender.tok)
	}

	// 同一篇不得重复推送
	p.Deliver(context.Background(), "art-1")
	if len(sender.sent) != 1 {
		t.Fatalf("duplicate push: sent = %d", len(sender.sent))
	}
}

func TestPusherIgnoresUnknownArticle(t *testing.T) {
	sender := &fakeSender{}
	p := NewPusher(&fakeArticles{}, nil)
	p.Attach(sender)

	// 后台/API 录入的文章没有登记，不应推到微信
	p.Deliver(context.Background(), "not-from-weixin")
	if len(sender.sent) != 0 {
		t.Fatalf("sent = %d, want 0", len(sender.sent))
	}
}

func TestPusherDropsStaleEntry(t *testing.T) {
	sender := &fakeSender{}
	p := NewPusher(&fakeArticles{}, nil)
	p.Attach(sender)
	p.ttl = time.Millisecond

	p.Expect("art-1", "u@im.wechat", "ctx-1")
	time.Sleep(5 * time.Millisecond)
	p.Deliver(context.Background(), "art-1")

	if len(sender.sent) != 0 {
		t.Fatalf("stale push should be dropped, sent = %d", len(sender.sent))
	}
}

func TestPusherDeliverWithoutSender(t *testing.T) {
	p := NewPusher(&fakeArticles{}, nil)
	p.Expect("art-1", "u@im.wechat", "ctx-1")
	// 未 Attach 发送端：不应 panic
	p.Deliver(context.Background(), "art-1")
}
