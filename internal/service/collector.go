package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
	// settingRepo 可选：非空时每次抓取读取后台 collector.user_agent 覆盖启动配置
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
		MaxContentLength: 100 * 1024,
		UserAgent:        "AiKnow/1.0 (Internal Knowledge Base)",
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

// SetUserAgentSource 设置后台 User-Agent 覆盖来源（settings 表）。
//
// 传入后每次抓取都会读取 collector.user_agent 配置：非空则覆盖启动配置，
// 为空（或未设置）则使用启动配置值，实现「config.yaml 设基线 + 后台可覆盖」。
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

	// 限制读取大小
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.config.MaxContentLength)+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	truncated := len(body) > int(c.config.MaxContentLength)
	if truncated {
		body = body[:c.config.MaxContentLength]
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

	// 提取标题（查找 <title> 标签）
	title := "Untitled"
	if idx := strings.Index(html, "<title>"); idx >= 0 {
		start := idx + 7
		if end := strings.Index(html[start:], "</title>"); end >= 0 {
			title = strings.TrimSpace(html[start : start+end])
		}
	}

	// 使用 htmlclean 清洗内容
	content := c.cleaner.Clean(html)

	return title, content
}
