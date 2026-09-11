package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/htmlclean"
	"github.com/jenvan/aiknow/pkg/ssrf"
)

// CollectorResult 采集结果
type CollectorResult struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

// Collector URL 采集器
type Collector struct {
	client    *http.Client
	ssrfCheck *ssrf.Checker
	cleaner   *htmlclean.Cleaner
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
	}
}

// NewCollector 创建采集器
func NewCollector(config CollectorConfig) *Collector {
	if config.Timeout == 0 {
		config = DefaultCollectorConfig()
	}

	checker := ssrf.NewChecker()
	cleaner := htmlclean.NewCleaner()

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

// Fetch 采集 URL 内容
func (c *Collector) Fetch(ctx context.Context, url string) (*CollectorResult, error) {
	// SSRF 检查
	ssrfResult, err := c.ssrfCheck.CheckURL(url)
	if err != nil {
		return nil, fmt.Errorf("ssrf check: %w", err)
	}
	if !ssrfResult.Allowed {
		return nil, fmt.Errorf("ssrf blocked: %s", ssrfResult.Reason)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", c.effectiveUserAgent(ctx))
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// 限制读取大小（上限可被后台 collector.max_content_length 覆盖）
	maxLen := c.effectiveMaxContentLength(ctx)
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxLen)+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	truncated := len(body) > maxLen
	if truncated {
		body = body[:maxLen]
	}

	// 提取标题和内容
	title, content := c.extractContent(url, body)

	collectorResult := &CollectorResult{
		Title:     title,
		Content:   content,
		Truncated: truncated,
	}

	// 检查最小内容长度
	if len([]rune(content)) < c.config.MinContentLength {
		return nil, fmt.Errorf("content too short: %d characters", len([]rune(content)))
	}

	return collectorResult, nil
}

// extractContent 从 HTML 中提取标题和内容
func (c *Collector) extractContent(url string, body []byte) (string, string) {
	html := string(body)

	// 提取标题：og:title / twitter:title / 微信 #activity-name / <title> 依次兜底。
	// 微信等页面的 <title> 常常为空，真实标题在 og:title 里，故优先取它。
	title := extractTitle(html)
	if title == "" {
		title = "Untitled"
	}

	// 使用 htmlclean 清洗内容
	content := c.cleaner.Clean(html)

	return title, content
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
