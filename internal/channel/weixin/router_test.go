package weixin

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/pkg/ilink"
)

// ---- fakes ----

type fakeArticles struct {
	created   []model.ArticleInput
	createErr error
	recent    *model.ArticleListResult
	byID      map[string]*model.Article
}

func (f *fakeArticles) Create(_ context.Context, in model.ArticleInput) (*model.Article, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, in)
	return &model.Article{
		ID:               "art-1",
		Title:            in.Title,
		SourceURL:        in.SourceURL,
		ProcessingStatus: "pending",
	}, nil
}

func (f *fakeArticles) List(context.Context, model.ArticleListQuery) (*model.ArticleListResult, error) {
	if f.recent != nil {
		return f.recent, nil
	}
	return &model.ArticleListResult{Articles: []model.Article{
		{Title: "旧文", ProcessingStatus: "completed"},
	}}, nil
}

func (f *fakeArticles) GetByID(_ context.Context, id string) (*model.Article, error) {
	return f.byID[id], nil
}

type fakeKeywords struct {
	result *model.KeywordSearchResult
	err    error
}

func (f fakeKeywords) SearchKeywords(context.Context, []string, model.KeywordSearchParams) (*model.KeywordSearchResult, error) {
	return f.result, f.err
}

type fakeSemantic struct {
	result *model.SearchResult
	called bool
}

func (f *fakeSemantic) Search(context.Context, string, model.SearchParams) (*model.SearchResult, error) {
	f.called = true
	return f.result, nil
}

type fakeCollector struct {
	doc *CollectedDoc
	err error
}

func (f fakeCollector) Fetch(context.Context, string) (*CollectedDoc, error) {
	return f.doc, f.err
}

func newRouter(a ArticleStore, k KeywordSearcher, s Searcher, c Collector) *Router {
	// 同步 Runner：测试里要断言「回执 + 结果」两条都在调用返回时就产生
	return NewRouter(a, k, s, c, nil, WithRunner(SyncRunner()))
}

func collectReplies(r *Router, msg *ilink.Message) []string {
	var out []string
	r.Handle(context.Background(), msg, func(text string) { out = append(out, text) })
	return out
}

func textMsg(text string) *ilink.Message {
	return &ilink.Message{
		MessageID: 1, FromUserID: "u@im.wechat", ContextToken: "ctx",
		MessageType: ilink.MessageTypeUser,
		Items:       []ilink.Item{{Type: ilink.ItemTypeText, Text: text}},
	}
}

// ---- tests ----

func TestHandleURLCollects(t *testing.T) {
	arts := &fakeArticles{}
	coll := fakeCollector{doc: &CollectedDoc{Title: "文章标题", Markdown: "# 正文"}}
	r := newRouter(arts, fakeKeywords{}, &fakeSemantic{}, coll)

	replies := collectReplies(r, textMsg("看看这篇 https://mp.weixin.qq.com/s/abc 不错"))

	if len(replies) != 2 {
		t.Fatalf("replies = %d, want 2 (回执 + 结果)", len(replies))
	}
	if !strings.Contains(replies[0], "正在采集") {
		t.Errorf("first reply = %q", replies[0])
	}
	if !strings.Contains(replies[1], "已收录") {
		t.Errorf("second reply = %q", replies[1])
	}
	if len(arts.created) != 1 {
		t.Fatalf("created = %d", len(arts.created))
	}
	got := arts.created[0]
	if got.SourceType != "url" || got.SourceURL == nil || *got.SourceURL != "https://mp.weixin.qq.com/s/abc" {
		t.Errorf("input = %+v", got)
	}
	if got.Content != "# 正文" {
		t.Errorf("content = %q, want markdown 优先", got.Content)
	}
}

func TestHandleURLFallsBackToPlainContent(t *testing.T) {
	arts := &fakeArticles{}
	r := newRouter(arts, fakeKeywords{}, &fakeSemantic{}, fakeCollector{
		doc: &CollectedDoc{Title: "t", Content: "纯文本正文"},
	})
	collectReplies(r, textMsg("https://example.com/a"))
	if len(arts.created) != 1 || arts.created[0].Content != "纯文本正文" {
		t.Fatalf("created = %+v", arts.created)
	}
}

func TestHandleDuplicateReportsOnce(t *testing.T) {
	arts := &fakeArticles{createErr: ErrDuplicate}
	r := newRouter(arts, fakeKeywords{}, &fakeSemantic{}, fakeCollector{doc: &CollectedDoc{Title: "t"}})

	replies := collectReplies(r, textMsg("https://mp.weixin.qq.com/s/abc"))
	if len(replies) != 2 || !strings.Contains(replies[1], "已经收录过") {
		t.Fatalf("replies = %v", replies)
	}
}

func TestHandleCollectError(t *testing.T) {
	r := newRouter(&fakeArticles{}, fakeKeywords{}, &fakeSemantic{},
		fakeCollector{err: errorString("boom")})
	replies := collectReplies(r, textMsg("https://x.com/a"))
	if len(replies) != 2 || !strings.Contains(replies[1], "采集失败") {
		t.Fatalf("replies = %v", replies)
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestHandleKeywordSearch(t *testing.T) {
	kw := fakeKeywords{result: &model.KeywordSearchResult{
		Total: 1,
		Articles: []model.Article{{
			Title: "Go 并发", CategoryName: "技术",
			Tags: []model.Tag{{Name: "golang"}}, ProcessingStatus: "completed",
		}},
	}}
	sem := &fakeSemantic{}
	r := newRouter(&fakeArticles{}, kw, sem, fakeCollector{})

	replies := collectReplies(r, textMsg("golang 并发"))
	if len(replies) != 1 || !strings.Contains(replies[0], "Go 并发") {
		t.Fatalf("replies = %v", replies)
	}
	if sem.called {
		t.Error("语义检索不应在关键词命中时被调用")
	}
}

func TestHandleSearchFallsBackToSemantic(t *testing.T) {
	sem := &fakeSemantic{result: &model.SearchResult{Results: []model.SearchChunk{
		{ArticleID: "a1", Content: "片段内容"},
	}}}
	arts := &fakeArticles{byID: map[string]*model.Article{
		"a1": {ID: "a1", Title: "被引用的文章"},
	}}
	r := newRouter(arts, fakeKeywords{result: &model.KeywordSearchResult{}}, sem, fakeCollector{})

	replies := collectReplies(r, textMsg("父母牵线怎么提高匹配准确率"))
	if !sem.called {
		t.Fatal("长问句关键词零命中时应降级到语义检索")
	}
	if len(replies) != 1 || !strings.Contains(replies[0], "被引用的文章") {
		t.Fatalf("replies = %v", replies)
	}
}

func TestHandleShortKeywordNoSemanticFallback(t *testing.T) {
	sem := &fakeSemantic{}
	r := newRouter(&fakeArticles{}, fakeKeywords{result: &model.KeywordSearchResult{}}, sem, fakeCollector{})

	replies := collectReplies(r, textMsg("K8s"))
	if sem.called {
		t.Error("短关键词零命中不该触发语义检索")
	}
	if len(replies) != 1 || !strings.Contains(replies[0], "没找到") {
		t.Fatalf("replies = %v", replies)
	}
}

func TestHandleCommands(t *testing.T) {
	r := newRouter(&fakeArticles{}, fakeKeywords{}, &fakeSemantic{}, fakeCollector{})

	if got := collectReplies(r, textMsg("/help"))[0]; !strings.Contains(got, "发 **文章链接**") {
		t.Errorf("/help = %q", got)
	}
	if got := collectReplies(r, textMsg("/status"))[0]; !strings.Contains(got, "旧文") {
		t.Errorf("/status = %q", got)
	}
	if got := collectReplies(r, textMsg("/unknown"))[0]; !strings.Contains(got, "未知指令") {
		t.Errorf("/unknown = %q", got)
	}
}

func TestHandleEmptyMessage(t *testing.T) {
	r := newRouter(&fakeArticles{}, fakeKeywords{}, &fakeSemantic{}, fakeCollector{})
	msg := &ilink.Message{MessageType: ilink.MessageTypeUser, Items: []ilink.Item{{Type: ilink.ItemTypeImage}}}
	replies := collectReplies(r, msg)
	if len(replies) != 1 || !strings.Contains(replies[0], "只支持文字") {
		t.Fatalf("replies = %v", replies)
	}
}

func TestExtractURLs(t *testing.T) {
	got := extractURLs("看这个 https://mp.weixin.qq.com/s/abc，还有 #rd 尾巴和 https://a.com/x?y=1。")
	if len(got) != 2 {
		t.Fatalf("urls = %v", got)
	}
	if got[0] != "https://mp.weixin.qq.com/s/abc" {
		t.Errorf("url[0] = %q", got[0])
	}
}

func TestSplitKeywords(t *testing.T) {
	got := splitKeywords("父母牵线 匹配、准确率,推荐")
	want := []string{"父母牵线", "匹配", "准确率", "推荐"}
	if len(got) != len(want) {
		t.Fatalf("got = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitReplyRespectsLimit(t *testing.T) {
	long := strings.Repeat("行内容\n", 400)
	parts := splitReply(long, 100)
	if len(parts) < 2 {
		t.Fatalf("parts = %d, want >= 2", len(parts))
	}
	for _, p := range parts {
		if len([]rune(p)) > 100 {
			t.Errorf("part too long: %d runes", len([]rune(p)))
		}
	}
	// 不能丢内容
	if got := strings.Join(parts, "\n"); strings.Count(got, "行内容") != 400 {
		t.Errorf("content lost: %d lines", strings.Count(got, "行内容"))
	}
}

func TestBotAllowsOnlyOwnerByDefault(t *testing.T) {
	acc := &model.WeixinBotAccount{OwnerUserID: "me@im.wechat"}
	if !acc.Allows("me@im.wechat") {
		t.Error("owner should be allowed")
	}
	if acc.Allows("other@im.wechat") {
		t.Error("empty allow_from must not allow others")
	}
	acc.AllowFrom = "a@im.wechat, b@im.wechat"
	if !acc.Allows("b@im.wechat") {
		t.Error("allow_from entry should be allowed")
	}
	if acc.Allows("") {
		t.Error("empty user id must be rejected")
	}
}

// 异步 Runner 下：调用返回时只应有「正在采集」回执，结果在后台补发。
func TestHandleURLAsyncSendsAckFirst(t *testing.T) {
	arts := &fakeArticles{}
	done := make(chan struct{})
	r := NewRouter(arts, fakeKeywords{}, &fakeSemantic{},
		fakeCollector{doc: &CollectedDoc{Title: "异步文章", Markdown: "# 正文"}},
		nil) // 默认异步 Runner

	var mu sync.Mutex
	var replies []string
	r.Handle(context.Background(), textMsg("https://mp.weixin.qq.com/s/async"), func(text string) {
		mu.Lock()
		replies = append(replies, text)
		n := len(replies)
		mu.Unlock()
		if n == 2 {
			close(done)
		}
	})

	// 立刻应只有回执
	mu.Lock()
	first := len(replies)
	mu.Unlock()
	if first != 1 || !strings.Contains(replies[0], "正在采集") {
		t.Fatalf("first replies = %v, want 仅回执", replies)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("后台采集未在 2s 内完成")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(replies[1], "已收录") {
		t.Errorf("second reply = %q", replies[1])
	}
}
