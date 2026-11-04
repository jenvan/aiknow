package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/htmlclean"
	"github.com/jenvan/aiknow/pkg/readability"
	"github.com/jenvan/aiknow/pkg/ssrf"
	"github.com/jenvan/aiknow/pkg/webfetch"
)

// CollectorResult 采集结果
type CollectorResult struct {
	Title string `json:"title"`
	// Content 正文纯文本，供入库 / 分块 / embedding。
	Content string `json:"content"`
	// Markdown 保留结构的正文（标题层级 / 列表 / 代码块），优先入库。
	// 渲染优先路径下，Markdown 与 Content 同源；分块器认 Markdown 的 # 标题与空行。
	Markdown string `json:"markdown,omitempty"`
	// Rendered 是否走了无头浏览器渲染（渲染优先路径）。
	Rendered  bool `json:"rendered,omitempty"`
	Truncated bool `json:"truncated"`

	// Screenshot 全页截图的 PNG 字节（仅渲染路径且启用截图时有值）。
	// 不参与 JSON 序列化：由上层（worker）上传 COS 后写回 articles.screenshot_url。
	Screenshot []byte `json:"-"`
}

// Collector URL 采集器
type Collector struct {
	client    *http.Client
	ssrfCheck *ssrf.Checker
	cleaner   *htmlclean.Cleaner
	renderer  *webfetch.Client // 无头浏览器渲染客户端；Enabled()==false 时渲染层关闭
	config    CollectorConfig
	// settingRepo 可选：非空时每次抓取读取后台 collector.user_agent /
	// collector.max_content_length 覆盖启动配置
	settingRepo repository.SettingRepository
}

// CollectorConfig 采集器配置
type CollectorConfig struct {
	Timeout          time.Duration
	MaxRedirects     int
	MinContentLength int
	UserAgent        string
	MaxContentLength int

	// 无头浏览器渲染
	RenderEnabled   bool          `yaml:"-"` // 总开关；与 BrowserEndpoint 都满足才渲染
	BrowserEndpoint string        // browserless 端点，缺省为空则渲染关闭
	BrowserToken    string        // browserless 访问令牌
	BrowserTimeout  time.Duration // 单页渲染硬超时
}

// DefaultCollectorConfig 默认配置
func DefaultCollectorConfig() CollectorConfig {
	return CollectorConfig{
		Timeout:          10 * time.Second,
		MaxRedirects:     3,
		MinContentLength: 200,
		// 默认 2MB：微信公众号文章头（广告、JS、样式）可达 1MB+，
		// 正文通常在 1MB 之后才开始，100KB 会截断导致提取不到内容。
		MaxContentLength: 2 * 1024 * 1024,
		UserAgent:        "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",

		// 浏览器默认关闭、端点为空；需要渲染时通过配置开启。
		RenderEnabled:   false,
		BrowserEndpoint: "",
		BrowserTimeout:  30 * time.Second,
	}
}

// NewCollector 创建采集器
func NewCollector(config CollectorConfig) *Collector {
	if config.Timeout == 0 {
		config = DefaultCollectorConfig()
	}
	// 缺省兜底：渲染超时未显式设置时填默认
	if config.BrowserTimeout == 0 {
		config.BrowserTimeout = 30 * time.Second
	}

	checker := ssrf.NewChecker()
	cleaner := htmlclean.NewCleaner()
	renderer := webfetch.New(webfetch.Config{
		BaseURL: config.BrowserEndpoint,
		Token:   config.BrowserToken,
		Timeout: config.BrowserTimeout,
	})

	return &Collector{
		client: &http.Client{
			Timeout: config.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= config.MaxRedirects {
					return fmt.Errorf("too many redirects")
				}
				// SSRF 检查
				result, _ := checker.CheckURL(req.URL.String())
				if !result.Allowed {
					return fmt.Errorf("ssrf blocked: %s", result.Reason)
				}
				return nil
			},
		},
		ssrfCheck: checker,
		cleaner:   cleaner,
		renderer:  renderer,
		config:    config,
	}
}

// SetUserAgentSource 设置后台可覆盖的采集配置来源（settings 表）。
//
// 传入后每次抓取都会读取 collector.user_agent 与 collector.max_content_length：
// 非空/大于 0 则覆盖启动配置，否则使用启动配置值，实现「config.yaml 设基线 +
// 后台可覆盖」。二者共用同一 settingRepo。
func (c *Collector) SetUserAgentSource(repo repository.SettingRepository) {
	c.settingRepo = repo
}

// effectiveUserAgent 解析最终 User-Agent：后台覆盖优先，否则用启动配置
func (c *Collector) effectiveUserAgent(ctx context.Context) string {
	ua := c.config.UserAgent
	if c.settingRepo != nil {
		if s, err := c.settingRepo.GetByKey(ctx, "collector.user_agent"); err == nil && s != nil {
			var override string
			if json.Unmarshal(s.Value, &override) == nil && override != "" {
				ua = override
			}
		}
	}
	return ua
}

// effectiveMaxContentLength 解析单次采集读取的 HTML 正文字节上限：
// 后台 collector.max_content_length 覆盖优先（>0 才生效），否则用启动配置默认值。
func (c *Collector) effectiveMaxContentLength(ctx context.Context) int {
	limit := c.config.MaxContentLength
	if c.settingRepo != nil {
		if s, err := c.settingRepo.GetByKey(ctx, "collector.max_content_length"); err == nil && s != nil {
			var override int
			if json.Unmarshal(s.Value, &override) == nil && override > 0 {
				limit = override
			}
		}
	}
	return limit
}

// errContentTooShort 提取到的正文过短（多为 SPA 空壳或反爬页），
// FetchLight 据此决定是否回退渲染。
var errContentTooShort = errors.New("content too short")

// Fetch 采集 URL 内容。
//
// 流程（渲染优先）：
//  1. SSRF 校验。
//  2. 渲染层启用时，先调无头浏览器：一次会话拿「渲染后 HTML + 全页截图」。
//  3. 渲染失败（未配置 / 超时 / 报错）时降级为轻量 http 抓源码，保证可用性。
//  4. 提取器路由：微信公众号 host 走 htmlclean 专用规则，其余走 readability
//     打分提取；readability 判定「非文章页」时回退 htmlclean，保证不丢内容。
//
// 输出的 Content 为纯文本，Markdown 为保留结构的版本（二者同源）；
// 截图为 PNG 字节，随结果返回供上层上传归档。
func (c *Collector) Fetch(ctx context.Context, url string) (*CollectorResult, error) {
	return c.fetch(ctx, url, true)
}

// FetchLight 只走轻量 http（不渲染、不截图），正文过短时才回退渲染。
//
// 用于「录入/剪藏」路径：正文质量不是关键——URL 文章入库后，worker 处理时会
// 重新采集（渲染 + 截图归档）；但响应速度很关键，公众号 http 抓取 1-3 秒，
// 渲染要约 30 秒。回退只在「正文太短」（典型 SPA 空壳）时发生，其它错误
// （404 / SSRF / 超时）直接返回，避免把失败也拖成 30 秒。
func (c *Collector) FetchLight(ctx context.Context, url string) (*CollectorResult, error) {
	res, err := c.fetch(ctx, url, false)
	if err == nil {
		return res, nil
	}
	if !errors.Is(err, errContentTooShort) || !c.renderUsable() {
		return nil, err
	}
	fmt.Printf("提示: %s 的 http 正文过短，回退渲染\n", url)
	return c.fetch(ctx, url, true)
}

func (c *Collector) renderUsable() bool {
	return c.config.RenderEnabled && c.renderer != nil && c.renderer.Enabled()
}

func (c *Collector) fetch(ctx context.Context, url string, allowRender bool) (*CollectorResult, error) {
	// SSRF 检查（渲染与 http 两条路都在其后再发起请求）
	ssrfResult, err := c.ssrfCheck.CheckURL(url)
	if err != nil {
		return nil, fmt.Errorf("ssrf check: %w", err)
	}
	if !ssrfResult.Allowed {
		return nil, fmt.Errorf("ssrf blocked: %s", ssrfResult.Reason)
	}

	html, screenshot, rendered, truncated, fetchErr := c.fetchHTML(ctx, url, allowRender)
	if fetchErr != nil {
		return nil, fmt.Errorf("fetch url: %w", fetchErr)
	}

	// 提取标题和内容（按 host 路由提取器）
	extracted := c.extract(url, html)
	title := extracted.Title
	if title == "" {
		title = "Untitled"
	}

	collectorResult := &CollectorResult{
		Title:      title,
		Content:    extracted.Text,
		Markdown:   extracted.Markdown,
		Rendered:   rendered,
		Truncated:  truncated,
		Screenshot: screenshot,
	}

	// 检查最小内容长度
	if len([]rune(extracted.Text)) < c.config.MinContentLength {
		return nil, fmt.Errorf("%w: %d characters", errContentTooShort, len([]rune(extracted.Text)))
	}

	return collectorResult, nil
}

// fetchHTML 获取页面 HTML（渲染优先，http 兜底）。
//
// 渲染层启用时，先调无头浏览器一次拿「渲染后 HTML + 全页截图」；渲染失败
// （未配置 / 超时 / 报错）则降级为轻量 http 抓源码，保证可用性——此时不带
// 截图（screenshot 为 nil）、rendered 为 false。
//
// 返回 (html, screenshotPNG, rendered, truncated, err)。
func (c *Collector) fetchHTML(ctx context.Context, url string, allowRender bool) (string, []byte, bool, bool, error) {
	if allowRender && c.config.RenderEnabled && c.renderer.Enabled() {
		// 带上采集 UA：微信等反爬站点会按 UA 判定爬虫，缺了会返回验证页
		if rr, err := c.renderer.RenderAndScreenshot(ctx, url, c.effectiveUserAgent(ctx)); err == nil && rr.HTML != "" {
			return rr.HTML, rr.Screenshot, true, false, nil
		} else if err != nil {
			fmt.Printf("警告: 渲染 %s 失败，降级为轻量 http: %v\n", url, err)
		}
	}
	html, truncated, err := c.fetchHTTP(ctx, url)
	return html, nil, false, truncated, err
}

// fetchHTTP 轻量 http 抓取源码（降级路径）。返回 (html, truncated, error)。
// 抓取失败时返回空串，由调用方按「内容过短」处理。
func (c *Collector) fetchHTTP(ctx context.Context, url string) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", c.effectiveUserAgent(ctx))
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// 限制读取大小（上限可被后台 collector.max_content_length 覆盖）
	maxLen := c.effectiveMaxContentLength(ctx)
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxLen)+1))
	if err != nil {
		return "", false, fmt.Errorf("read body: %w", err)
	}

	truncated := len(body) > maxLen
	if truncated {
		body = body[:maxLen]
	}
	return string(body), truncated, nil
}

// extractedContent 提取结果。
type extractedContent struct {
	Title    string
	Text     string
	Markdown string
}

// extract 从 HTML 提取正文，按 host 路由提取器：
//   - 微信公众号（mp.weixin.qq.com）→ htmlclean 专用 #js_content 规则（比通用
//     Readability 更准，Readability 对微信常判「非文章页」）。
//   - 其余 → readability 打分提取（保留结构）；判定「非文章页」时回退 htmlclean。
func (c *Collector) extract(url, html string) extractedContent {
	title := extractTitle(html)

	host := hostOf(url)
	if isWechatHost(host) {
		return extractedContent{
			Title:    title,
			Text:     c.cleaner.Clean(html),
			Markdown: c.cleaner.CleanMarkdown(html),
		}
	}

	if article, err := readability.Extract(html, readability.Options{PageURL: url}); err == nil {
		return extractedContent{
			Title:    firstNonEmpty(article.Title, title),
			Text:     article.Text,
			Markdown: article.Markdown,
		}
	}

	// 非文章页 / 提取失败：回退到选择器启发式，保证内容不丢
	return extractedContent{
		Title:    title,
		Text:     c.cleaner.Clean(html),
		Markdown: c.cleaner.CleanMarkdown(html),
	}
}

// hostOf 取 URL 的 host（小写）。解析失败返回空串。
func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return strings.ToLower(u.Hostname())
	}
	return ""
}

// isWechatHost 是否为微信公众号文章 host。
func isWechatHost(host string) bool {
	return host == "mp.weixin.qq.com" || strings.HasSuffix(host, ".weixin.qq.com")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// extractTitle 按优先级从 HTML 中取标题，取到即返回（已 TrimSpace）：
// og:title → twitter:title → #activity-name(微信) → 正文 H1 → <title>。
func extractTitle(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	// 1) Open Graph / Twitter 卡片标题（微信、多数现代站点都在此）
	for _, sel := range []string{
		"meta[property='og:title']",
		"meta[name='twitter:title']",
	} {
		if t := strings.TrimSpace(doc.Find(sel).First().AttrOr("content", "")); t != "" {
			return t
		}
	}

	// 2) 微信正文区标题 #activity-name
	if t := strings.TrimSpace(doc.Find("#activity-name").First().Text()); t != "" {
		return t
	}

	// 3) 首个正文 H1
	if t := strings.TrimSpace(doc.Find("h1").First().Text()); t != "" {
		return t
	}

	// 4) <title> 兜底
	return strings.TrimSpace(doc.Find("title").First().Text())
}
