package weixin

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/pkg/ilink"
)

// ErrDuplicate 文章已存在（URL 或正文哈希命中）。
//
// 由适配层把 service.ErrDuplicateArticle 翻译成它，channel 层不依赖 service 包，
// 也便于测试注入。
var ErrDuplicate = errors.New("duplicate article")

// CollectedDoc 是采集结果里本层需要的字段。
type CollectedDoc struct {
	Title    string
	Content  string
	Markdown string
}

// ArticleStore 文章读写。
type ArticleStore interface {
	Create(ctx context.Context, input model.ArticleInput) (*model.Article, error)
	List(ctx context.Context, query model.ArticleListQuery) (*model.ArticleListResult, error)
	GetByID(ctx context.Context, id string) (*model.Article, error)
}

// KeywordSearcher 关键词检索（标签/分类加权）。
type KeywordSearcher interface {
	SearchKeywords(ctx context.Context, keywords []string, params model.KeywordSearchParams) (*model.KeywordSearchResult, error)
}

// Searcher 混合语义检索，用于关键词零命中时的降级。
type Searcher interface {
	Search(ctx context.Context, query string, params model.SearchParams) (*model.SearchResult, error)
}

// Collector 抓取网页正文。
type Collector interface {
	Fetch(ctx context.Context, url string) (*CollectedDoc, error)
}

// Emit 发送一条回复（调用方负责按长度切分并真正发出）。
type Emit func(text string)

// Router 把一条微信消息翻译成对既有 service 的调用，并格式化回包。
//
// 它刻意只做「消息 → 意图 → 现有 service」的适配，不重复实现检索/采集逻辑：
// 采集走 ArticleService（含去重与入队），搜索走 KeywordSearchService /
// SearchService，与 MCP 工具、REST API 共用同一套语义。
type Router struct {
	articles  ArticleStore
	keywords  KeywordSearcher
	semantic  Searcher
	collector Collector
	logger    *slog.Logger

	// onQueued 在文章采集入库后调用，用于登记「处理完成后回推微信」。
	// 可为 nil（未启用主动推送时）。
	onQueued func(articleID, userID, contextToken string)
}

// NewRouter 构造路由。
func NewRouter(articles ArticleStore, keywords KeywordSearcher, semantic Searcher, collector Collector, logger *slog.Logger) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	return &Router{
		articles:  articles,
		keywords:  keywords,
		semantic:  semantic,
		collector: collector,
		logger:    logger,
	}
}

// SetOnQueued 注册「已入队」回调，由 Bot 在启动时接到 Pusher 上。
func (r *Router) SetOnQueued(fn func(articleID, userID, contextToken string)) {
	r.onQueued = fn
}

var urlRe = regexp.MustCompile(`(?i)https?://[^\s"'<>）)】》，。；；]+`)

// Handle 处理一条消息，可能产生零到多条回复。
func (r *Router) Handle(ctx context.Context, msg *ilink.Message, emit Emit) {
	text := strings.TrimSpace(msg.Text())
	if text == "" {
		emit("目前只支持文字和链接。\n发文章链接可采集入库，发关键词可搜文章。")
		return
	}

	if strings.HasPrefix(text, "/") {
		r.handleCommand(ctx, text, emit)
		return
	}

	if urls := extractURLs(text); len(urls) > 0 {
		if len(urls) > 1 {
			emit("一次只处理一个链接，先采第一条。")
		}
		r.collect(ctx, msg, urls[0], emit)
		return
	}

	r.search(ctx, text, emit)
}

func (r *Router) handleCommand(ctx context.Context, text string, emit Emit) {
	cmd := strings.Fields(text)[0]
	switch cmd {
	case "/help", "/?", "/hi":
		emit(helpText)
	case "/status":
		res, err := r.articles.List(ctx, model.ArticleListQuery{
			Page: 1, PageSize: 5, SortBy: "created_at", SortOrder: "desc", Tokenized: true,
		})
		if err != nil {
			r.logger.ErrorContext(ctx, "weixin: list recent articles failed", slog.Any("err", err))
			emit("读取最近文章失败，稍后再试。")
			return
		}
		emit(formatRecent(toBriefs(res.Articles)))
	default:
		emit("未知指令 " + cmd + "\n\n" + helpText)
	}
}

func (r *Router) collect(ctx context.Context, msg *ilink.Message, rawURL string, emit Emit) {
	target := normalizeURL(rawURL)
	emit(formatCollecting(target))

	doc, err := r.collector.Fetch(ctx, target)
	if err != nil {
		r.logger.ErrorContext(ctx, "weixin: collect failed", slog.String("url", target), slog.Any("err", err))
		emit(formatCollectFailed(target, shortErr(err)))
		return
	}

	content := doc.Markdown
	if strings.TrimSpace(content) == "" {
		content = doc.Content
	}
	src := target
	article, err := r.articles.Create(ctx, model.ArticleInput{
		Title:      firstNonEmpty(doc.Title, "未命名文章"),
		Content:    content,
		SourceType: "url",
		SourceURL:  &src,
	})
	if err != nil {
		if errors.Is(err, ErrDuplicate) {
			emit(formatDuplicate(target))
			return
		}
		r.logger.ErrorContext(ctx, "weixin: create article failed", slog.String("url", target), slog.Any("err", err))
		emit(formatCollectFailed(target, shortErr(err)))
		return
	}

	if r.onQueued != nil {
		r.onQueued(article.ID, msg.FromUserID, msg.ContextToken)
	}

	emit(formatCollected(articleBrief{
		Title:     article.Title,
		Status:    article.ProcessingStatus,
		SourceURL: target,
	}))
}

// search 先用关键词检索（便宜、快、对「打了这个标签的文章」更准），
// 零命中且输入够长时才降级到混合语义检索（要 embed + rerank，有成本）。
func (r *Router) search(ctx context.Context, text string, emit Emit) {
	keywords := splitKeywords(text)
	if len(keywords) == 0 {
		emit(usageHint)
		return
	}

	res, err := r.keywords.SearchKeywords(ctx, keywords, model.KeywordSearchParams{TopK: 5})
	if err == nil && res != nil && len(res.Articles) > 0 {
		emit(formatSearch(toBriefs(res.Articles), text))
		return
	}
	if err != nil {
		r.logger.ErrorContext(ctx, "weixin: keyword search failed", slog.Any("err", err))
	}

	if utf8.RuneCountInString(text) < 8 {
		emit(formatSearch(nil, text))
		return
	}

	sem, err := r.semantic.Search(ctx, text, model.SearchParams{TopK: 3})
	if err != nil || sem == nil || len(sem.Results) == 0 {
		if err != nil {
			r.logger.ErrorContext(ctx, "weixin: semantic search failed", slog.Any("err", err))
		}
		emit(formatSearch(nil, text))
		return
	}
	emit(formatChunks(r.chunksWithTitles(ctx, sem.Results), text))
}

// chunksWithTitles 混合检索只返回 chunk（不含标题/链接），按 article_id 补全。
// 同名文章去重，最多查 3 次。
func (r *Router) chunksWithTitles(ctx context.Context, chunks []model.SearchChunk) []chunkBrief {
	cache := map[string]*model.Article{}
	out := make([]chunkBrief, 0, len(chunks))
	for _, c := range chunks {
		art, ok := cache[c.ArticleID]
		if !ok {
			fetched, err := r.articles.GetByID(ctx, c.ArticleID)
			if err != nil {
				r.logger.WarnContext(ctx, "weixin: get article for chunk failed",
					slog.String("article_id", c.ArticleID), slog.Any("err", err))
			}
			art = fetched
			cache[c.ArticleID] = art
		}
		b := chunkBrief{Content: c.Content}
		if art != nil {
			b.ArticleTitle = art.Title
			if art.SourceURL != nil {
				b.SourceURL = *art.SourceURL
			}
		}
		out = append(out, b)
	}
	return out
}

// extractURLs 从消息文本里提取链接（用户是「粘贴链接文本」，不是转发卡片）。
func extractURLs(text string) []string {
	matches := urlRe.FindAllString(text, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		// 去掉粘贴时常带的尾部标点
		m = strings.TrimRight(m, ".,;:!?，。；：！？、")
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}

// normalizeURL 规范化链接后再入库，减少「同一篇文章被当成两篇」。
//
// 公众号文章常带 #rd 之类的 fragment，以及 scene/from 这类跟踪参数：
// 当前去重是 source_url 精确比较（service/article.go），带这些尾巴就会重复。
func normalizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""
	u.RawFragment = ""
	q := u.Query()
	for _, k := range []string{"scene", "from", "isappinstalled", "clicktime", "enterid", "ascene", "devicetype", "version", "nettype", "lang", "exportkey", "pass_ticket", "wx_header"} {
		q.Del(k)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// splitKeywords 把用户输入切成关键词：空格/逗号/顿号都算分隔。
func splitKeywords(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', ',', '，', '、', ';', '；':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

type articleBrief struct {
	Title     string
	Category  string
	Tags      []string
	SourceURL string
	Status    string
}

func (a articleBrief) metaLine() string {
	parts := make([]string, 0, 2)
	if a.Category != "" {
		parts = append(parts, a.Category)
	}
	if len(a.Tags) > 0 {
		parts = append(parts, strings.Join(a.Tags, " / "))
	}
	return strings.Join(parts, " · ")
}

type chunkBrief struct {
	ArticleTitle string
	Content      string
	SourceURL    string
}

func toBriefs(articles []model.Article) []articleBrief {
	out := make([]articleBrief, 0, len(articles))
	for _, a := range articles {
		b := articleBrief{
			Title:    a.Title,
			Category: a.CategoryName,
			Status:   a.ProcessingStatus,
		}
		for _, t := range a.Tags {
			b.Tags = append(b.Tags, t.Name)
		}
		if a.SourceURL != nil {
			b.SourceURL = *a.SourceURL
		}
		out = append(out, b)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func shortErr(err error) string {
	s := err.Error()
	if utf8.RuneCountInString(s) > 120 {
		return string([]rune(s)[:120]) + "…"
	}
	return s
}
