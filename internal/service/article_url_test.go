package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jenvan/aiknow/internal/model"
)

// ---- fakes ----

type fakeArticleRepo struct {
	byURL     *model.Article
	byHash    *model.Article
	created   []*model.Article
	createErr error
}

func (f *fakeArticleRepo) Create(_ context.Context, a *model.Article) error {
	if f.createErr != nil {
		return f.createErr
	}
	if a.ID == "" {
		a.ID = "art-1"
	}
	f.created = append(f.created, a)
	return nil
}

func (f *fakeArticleRepo) GetByID(_ context.Context, id string) (*model.Article, error) {
	for _, a := range f.created {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, nil
}

func (f *fakeArticleRepo) FindByURL(context.Context, string) (*model.Article, error) {
	return f.byURL, nil
}

func (f *fakeArticleRepo) FindByContentHash(context.Context, string) (*model.Article, error) {
	return f.byHash, nil
}

func (f *fakeArticleRepo) List(context.Context, model.ArticleListQuery) (*model.ArticleListResult, error) {
	return &model.ArticleListResult{}, nil
}
func (f *fakeArticleRepo) Update(context.Context, *model.Article) error      { return nil }
func (f *fakeArticleRepo) SoftDelete(context.Context, string, *string) error { return nil }
func (f *fakeArticleRepo) Restore(context.Context, string) error             { return nil }
func (f *fakeArticleRepo) CountByStatus(context.Context) (map[string]int, error) {
	return nil, nil
}
func (f *fakeArticleRepo) BulkUpdateStatus(context.Context, []string, string) error { return nil }
func (f *fakeArticleRepo) BulkSoftDelete(context.Context, []string, *string) error  { return nil }
func (f *fakeArticleRepo) GetArticleStatus(context.Context, string) (*model.ArticleStatus, error) {
	return nil, nil
}

func newServiceForURL(repo *fakeArticleRepo, coll *fakeURLCollector) *articleServiceImpl {
	s := NewArticleService(repo, nil, nil, nil)
	if coll != nil {
		s.SetCollector(coll)
	}
	return s
}

// ---- tests ----

func TestCreateFromURLPrefersMarkdown(t *testing.T) {
	repo := &fakeArticleRepo{}
	coll := &fakeURLCollector{result: &CollectorResult{
		Title: "一篇公众号文章", Content: "纯文本正文", Markdown: "# 标题\n\n正文",
	}}
	s := newServiceForURL(repo, coll)

	a, err := s.CreateFromURL(context.Background(), "https://mp.weixin.qq.com/s/abc?scene=1#rd")
	if err != nil {
		t.Fatalf("CreateFromURL: %v", err)
	}
	if a.SourceType != "url" || a.SourceURL == nil || *a.SourceURL != "https://mp.weixin.qq.com/s/abc" {
		t.Errorf("source = %s %v, want url + 归一后的链接", a.SourceType, a.SourceURL)
	}
	if a.Title != "一篇公众号文章" {
		t.Errorf("title = %q", a.Title)
	}
	if len(repo.created) != 1 || repo.created[0].Content != "# 标题\n\n正文" {
		t.Errorf("content = %+v, want markdown 优先", repo.created)
	}
	// 采集器收到的必须是归一后的 URL
	if coll.gotURL != "https://mp.weixin.qq.com/s/abc" {
		t.Errorf("collector got %q", coll.gotURL)
	}
}

func TestCreateFromURLFallsBackToPlainContent(t *testing.T) {
	repo := &fakeArticleRepo{}
	coll := &fakeURLCollector{result: &CollectorResult{Title: "t", Content: "纯文本"}}
	s := newServiceForURL(repo, coll)

	if _, err := s.CreateFromURL(context.Background(), "https://example.com/a"); err != nil {
		t.Fatalf("CreateFromURL: %v", err)
	}
	if repo.created[0].Content != "纯文本" {
		t.Errorf("content = %q", repo.created[0].Content)
	}
}

func TestCreateFromURLDuplicate(t *testing.T) {
	repo := &fakeArticleRepo{byURL: &model.Article{ID: "existing"}}
	s := newServiceForURL(repo, &fakeURLCollector{result: &CollectorResult{Title: "t", Content: "c"}})

	_, err := s.CreateFromURL(context.Background(), "https://example.com/a")
	if !errors.Is(err, ErrDuplicateArticle) {
		t.Fatalf("err = %v, want ErrDuplicateArticle", err)
	}
	if len(repo.created) != 0 {
		t.Error("duplicate must not create a new article")
	}
}

func TestCreateFromURLDuplicateSkipsCollection(t *testing.T) {
	coll := &fakeURLCollector{result: &CollectorResult{Title: "t", Content: "c"}}
	s := newServiceForURL(&fakeArticleRepo{byURL: &model.Article{ID: "existing"}}, coll)

	_, err := s.CreateFromURL(context.Background(), "https://example.com/a")
	if !errors.Is(err, ErrDuplicateArticle) {
		t.Fatalf("err = %v, want ErrDuplicateArticle", err)
	}
	if coll.called {
		t.Error("重复链接不应触发采集（渲染很贵）")
	}
}

func TestCreateFromURLWithoutCollector(t *testing.T) {
	s := newServiceForURL(&fakeArticleRepo{}, nil)
	_, err := s.CreateFromURL(context.Background(), "https://example.com/a")
	if !errors.Is(err, ErrCollectorNotConfigured) {
		t.Fatalf("err = %v, want ErrCollectorNotConfigured", err)
	}
}

func TestCreateFromURLEmptyContent(t *testing.T) {
	s := newServiceForURL(&fakeArticleRepo{}, &fakeURLCollector{result: &CollectorResult{Title: "t"}})
	if _, err := s.CreateFromURL(context.Background(), "https://example.com/a"); err == nil {
		t.Fatal("empty content must error")
	}
}

func TestCreateFromURLEmptyInput(t *testing.T) {
	s := newServiceForURL(&fakeArticleRepo{}, &fakeURLCollector{})
	if _, err := s.CreateFromURL(context.Background(), "  "); !errors.Is(err, ErrEmptyURL) {
		t.Fatalf("err = %v, want ErrEmptyURL", err)
	}
}
