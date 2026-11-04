// Package webfetch 通过外部 browserless 服务获取「渲染后的 HTML + 全页截图」。
//
// 它只负责「网络层渲染」：把 URL 交给 browserless 的 /function 端点（自定义
// Puppeteer 脚本），一次会话内拿到 JavaScript 执行完的文档和截图。正文提取与
// Markdown 化由上层（collector + readability / htmlclean）负责，本包不碰项目
// 其他包，保持可独立测试。
package webfetch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config 渲染客户端配置。
type Config struct {
	// BaseURL browserless 服务基础地址，如 http://10.0.0.5:3000。
	// 缺省为空时 Enabled() 返回 false，渲染层整体关闭。
	BaseURL string
	// Token browserless 访问令牌，请求头 x-access-token。
	Token string
	// Timeout 单次渲染硬超时（含 browserless 内部渲染耗时）。默认 30s。
	Timeout time.Duration
	// WaitUntil 等待加载时机，browserless /content 接受 load / networkidle 等。
	// 留空则由 browserless 使用默认。
	WaitUntil string
}

// DefaultConfig 默认渲染配置。
func DefaultConfig() Config {
	return Config{
		Timeout:   30 * time.Second,
		WaitUntil: "networkidle",
	}
}

// New 创建渲染客户端。仅对未设置的字段（Timeout 为 0、WaitUntil 为空）填默认值，
// 不覆盖调用方已提供的 BaseURL / Token。
func New(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultConfig().Timeout
	}
	if cfg.WaitUntil == "" {
		cfg.WaitUntil = DefaultConfig().WaitUntil
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{cfg: cfg, http: newHTTPClient(cfg.Timeout)}
}

// Client 外部浏览器渲染客户端。
type Client struct {
	cfg  Config
	http *http.Client
}

// RenderResult 渲染结果。
type RenderResult struct {
	// HTML 渲染后的完整 HTML 文档字符串。
	HTML string
	// FinalURL 浏览器最终落地的 URL（可能经重定向），便于上层记录来源。
	FinalURL string
	// Rendered 是否真的走了渲染路径。
	Rendered bool
	// Screenshot 全页截图的 PNG 字节；未截图时为 nil。
	Screenshot []byte
}

// Enabled 渲染层是否可用（配置了浏览器端点）。
func (c *Client) Enabled() bool {
	return c.cfg.BaseURL != ""
}

// newHTTPClient 渲染客户端专用 http.Client，超时覆盖整次渲染（含远程渲染耗时）。
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// functionCode 交给 browserless /function 执行的 Puppeteer 脚本。
//
// 一次浏览器会话内同时产出「渲染后 HTML」与「全页 base64 截图」，避免
// /content 与 /screenshot 两个端点各渲染一次导致结果不一致、耗时翻倍。
// fullPage:true 保证归档截图覆盖整页（默认仅视口）。
//
// 等待策略：goto 用 "load"（可靠，不依赖网络空闲）；随后**尽力**等 networkidle2
// 让 SPA 异步渲染落定，但用单独的短超时兜住——有持续轮询/埋点/长连接的站点永远
// 达不到 networkidle2，若直接用它作 goto 的 waitUntil 会整页超时失败（实测
// landscape 页由此报 "Navigation timeout of 28000 ms exceeded"）。最后固定再等
// waitMs，进一步提升动态内容的完整度。
//
// 关键：context.userAgent 非空时用 setUserAgent 覆盖浏览器默认 UA。缺了这步，
// 微信等反爬站点会用数据中心桌面 Chromium 的默认 UA 判定为爬虫，返回验证页
// 而非正文（实测：不设 UA 渲染微信得验证页；设移动 UA 得 5MB 全文）。
const functionCode = `
export default async ({ page, context }) => {
  if (context.userAgent) {
    await page.setUserAgent(context.userAgent);
  }
  await page.goto(context.url, { waitUntil: "load", timeout: context.navTimeout });
  try {
    await page.waitForNetworkIdle({ idleTime: 500, timeout: context.idleTimeout });
  } catch (e) {
    // 网络始终不空闲（轮询/埋点/长连接）：忽略，继续用当前 DOM
  }

  // 触发懒加载图片：微信等站点正文图用 data-src 存真实地址，靠 JS 在图片
  // 进入视口时替换 src。整页截图只渲染视口内内容，故必须先滚过整页把懒加载
  // 图全部激活，否则截图里配图位置只有占位（空白/转圈）。
  try {
    await page.evaluate(async () => {
      const step = 600;
      const max = Math.min(document.body.scrollHeight, 100000);
      for (let y = 0; y < max; y += step) {
        window.scrollTo(0, y);
        await new Promise((r) => setTimeout(r, 60));
      }
      window.scrollTo(0, document.body.scrollHeight);
      await new Promise((r) => setTimeout(r, 200));
      window.scrollTo(0, 0);
    });
  } catch (e) {
    // 滚动失败不应阻断截图
  }

  // 等所有 <img> 加载完成。注意：懒加载占位图（data:image/svg）的 complete
  // 为 true、naturalWidth 为 1，会被误判为"已加载"，故这里额外覆盖 naturalWidth<=1
  // 但仍带 data-src 的图，滚动后它们已换上真实 src。
  try {
    await page.evaluate((timeout) => {
      const imgs = [...document.images].filter((i) => i.src && (!i.complete || i.naturalWidth <= 1));
      if (!imgs.length) return;
      return Promise.race([
        Promise.all(imgs.map((i) => new Promise((res) => {
          if (i.complete && i.naturalWidth > 1) return res();
          i.addEventListener("load", res, { once: true });
          i.addEventListener("error", res, { once: true });
        }))),
        new Promise((res) => setTimeout(res, timeout)),
      ]);
    }, context.imgTimeout);
  } catch (e) {
    // 等待图片失败不应阻断截图
  }
  if (context.waitMs) {
    await new Promise((r) => setTimeout(r, context.waitMs));
  }
  const html = await page.content();
  const screenshot = await page.screenshot({ fullPage: true, type: "png", encoding: "base64" });
  return { data: { html, screenshot }, type: "application/json" };
};
`

// functionResponse browserless /function 的 JSON 响应体。
type functionResponse struct {
	Data struct {
		HTML       string `json:"html"`
		Screenshot string `json:"screenshot"`
	} `json:"data"`
}

// RenderAndScreenshot 用外部浏览器渲染 URL，一次调用同时返回渲染后 HTML 与全页截图。
//
// 走 browserless 的 /function 端点（自定义 Puppeteer 代码），单次会话内完成
// page.content() 与 page.screenshot({fullPage:true})。截图以 base64 返回，本方法
// 解码为 PNG 字节。调用方必须先完成 SSRF 校验（见 collector）。
//
// userAgent 非空时设为浏览器页面的 User-Agent（反爬站点如微信必需）；为空则用
// 浏览器默认 UA。之所以按调用传入而非放 Config：UA 可被后台 settings 覆盖并热生效。
//
// 任一环节失败即整体报错，由调用方决定是否降级为纯 http。
func (c *Client) RenderAndScreenshot(ctx context.Context, rawURL, userAgent string) (*RenderResult, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("webfetch: browser endpoint not configured")
	}
	if rawURL == "" {
		return nil, fmt.Errorf("webfetch: empty url")
	}

	fnCtx := map[string]any{
		"url": rawURL,
		// 页面导航（load 事件）超时略小于客户端总超时，让页面侧先失败并返回可读错误
		"navTimeout": int(c.cfg.Timeout.Milliseconds() - 2000),
		// 尽力等网络空闲的单独超时（不阻断）：达到即继续，达不到也继续
		"idleTimeout": int(idleTimeout(c.cfg.Timeout).Milliseconds()),
		// 等所有 <img> 加载完成的超时（不阻断）：个别图始终不完成时不卡死
		"imgTimeout": int(idleTimeout(c.cfg.Timeout).Milliseconds()),
		// 网络空闲后再固定等一小段，让最后一批 JS 渲染落定
		"waitMs": 500,
	}
	if userAgent != "" {
		fnCtx["userAgent"] = userAgent
	}
	payload := map[string]any{
		"code":    functionCode,
		"context": fnCtx,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("webfetch: marshal function request: %w", err)
	}

	endpoint := c.cfg.BaseURL + "/function"
	if c.cfg.Token != "" {
		// /function 从查询参数取令牌；header 形态另作兼容
		endpoint += "?token=" + urlQueryEscape(c.cfg.Token)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("webfetch: build function request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Token != "" {
		req.Header.Set("x-access-token", c.cfg.Token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webfetch: call browserless: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("webfetch: browserless returned %d: %s", resp.StatusCode, truncate(string(buf), 200))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("webfetch: read body: %w", err)
	}

	var fr functionResponse
	if err := json.Unmarshal(raw, &fr); err != nil {
		return nil, fmt.Errorf("webfetch: decode function response: %w", err)
	}
	if fr.Data.HTML == "" {
		return nil, fmt.Errorf("webfetch: function returned empty html")
	}

	result := &RenderResult{
		HTML:     fr.Data.HTML,
		FinalURL: rawURL,
		Rendered: true,
	}
	if fr.Data.Screenshot != "" {
		shot, derr := base64.StdEncoding.DecodeString(fr.Data.Screenshot)
		if derr != nil {
			// 截图解码失败不影响正文：退化为「有 HTML、无截图」
			return result, fmt.Errorf("webfetch: decode screenshot: %w", derr)
		}
		result.Screenshot = shot
	}
	return result, nil
}

// Render 用外部浏览器渲染 URL，返回渲染后的 HTML。
//
// 走 browserless 的 HTTP /content 端点（无状态，单次往返返回渲染后文档）。
// 调用方必须先完成 SSRF 校验（见 collector）；本包不重复校验，保持纯网络职责。
func (c *Client) Render(ctx context.Context, rawURL string) (*RenderResult, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("webfetch: browser endpoint not configured")
	}
	if rawURL == "" {
		return nil, fmt.Errorf("webfetch: empty url")
	}

	// 用 URLQuery 传参，避免特殊字符问题
	payload := map[string]any{"url": rawURL}
	if c.cfg.WaitUntil != "" {
		payload["waitUntil"] = c.cfg.WaitUntil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("webfetch: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/content", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("webfetch: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Token != "" {
		req.Header.Set("x-access-token", c.cfg.Token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webfetch: call browserless: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 读取部分响应体便于诊断，但限制大小
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("webfetch: browserless returned %d: %s", resp.StatusCode, truncate(string(buf), 200))
	}

	// browserless /content 默认返回渲染后的 HTML 文本（非 JSON 包装）
	htmlStr, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("webfetch: read body: %w", err)
	}

	// 部分版本 /content 返回 JSON 包 {content, page,...}；两种都兼容。
	if trimmed := strings.TrimSpace(string(htmlStr)); strings.HasPrefix(trimmed, "{") {
		var wrapped struct {
			Content string `json:"content"`
			Page    struct {
				URL string `json:"url"`
			} `json:"page"`
		}
		if jerr := json.Unmarshal(htmlStr, &wrapped); jerr == nil && wrapped.Content != "" {
			return &RenderResult{
				HTML:     wrapped.Content,
				FinalURL: firstNonEmpty(wrapped.Page.URL, rawURL),
				Rendered: true,
			}, nil
		}
	}

	// 直接 HTML
	return &RenderResult{
		HTML:     string(htmlStr),
		FinalURL: rawURL,
		Rendered: true,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// urlQueryEscape 转义查询参数值。
func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}

// idleTimeout 计算「尽力等网络空闲 / 等图片加载」的子超时：取总渲染超时的 1/3，
// 随 COLLECTOR_BROWSER_TIMEOUT 线性变化（不封顶），下限 3s 防总超时过小取到近似 0。
//
// 为何取 1/3：这两步是「尽力而为」，不该吃掉全部预算——导航用 load 事件已保证
// 页面基本就绪，剩余 2/3 留给后续的 waitMs 与服务端写响应。比例固定（而非封顶），
// 使调大总超时能等比放宽等待时间，行为可预测。
func idleTimeout(total time.Duration) time.Duration {
	t := total / 3
	if t < 3*time.Second {
		t = 3 * time.Second
	}
	return t
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
