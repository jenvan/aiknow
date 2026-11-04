package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jenvan/aiknow/pkg/ssrf"
)

// ObjectStore 对象存储（截图之外的通用上传）。
//
// *pkg/cos.Client 已实现 Upload(ctx, key, data, contentType)。抽成接口是为了
// 测试注入假实现，也避免 service 直接依赖 cos 包。
type ObjectStore interface {
	Upload(ctx context.Context, objectKey string, data []byte, contentType string) (string, error)
	Enabled() bool
}

// ImageArchiver 把正文里引用的外链图片下载下来存到对象存储，并替换链接。
//
// 为什么需要：微信公众号图片（mmbiz.qpic.cn）有防盗链——在别的域名下引用会
// 返回 403 或占位图，所以库里存的 Markdown 在后台预览时图片全是坏的；源站
// 文章删除后更是永久失效。归档后预览可用、也不依赖源站。
type ImageArchiver struct {
	store     ObjectStore
	http      *http.Client
	referer   string
	userAgent string
	ssrfCheck *ssrf.Checker

	// maxImages 单篇最多归档张数（公众号长文常见 40+ 张）
	maxImages int
	// maxBytes 单张最大字节数
	maxBytes int64
}

// 正文里的 Markdown 图片：![alt](https://...)
//
// 允许 URL 里出现换行/空格：公众号正文经 html→markdown 后，长 URL 有时会被
// 折行（`...?from=appmsg&\nwatermark=1...`），用 [^)\s]+ 会在换行处截断导致整张图
// 匹配不上。这里放宽到「非右括号」，匹配后再把空白全部去掉。
var markdownImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

// NewImageArchiver 创建归档器。store 为 nil 或未启用时 Archive 直接透传原文。
func NewImageArchiver(store ObjectStore, userAgent string) *ImageArchiver {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
	}
	return &ImageArchiver{
		store:     store,
		http:      &http.Client{Timeout: 20 * time.Second},
		referer:   "https://mp.weixin.qq.com/",
		userAgent: userAgent,
		ssrfCheck: ssrf.NewChecker(),
		maxImages: 60,
		maxBytes:  10 << 20, // 10MB
	}
}

// SetReferer 覆盖下载图片时带的 Referer（默认 mp.weixin.qq.com，
// 公众号图床只认这个来路）。
func (a *ImageArchiver) SetReferer(ref string) {
	if ref != "" {
		a.referer = ref
	}
}

// Archive 下载 markdown 中引用的图片并替换为对象存储 URL。
//
// 失败策略是「尽力而为」：单张失败保留原链接并继续，绝不因为图片问题让整篇
// 文章处理失败——正文才是主体。
func (a *ImageArchiver) Archive(ctx context.Context, articleID, markdown string) (string, error) {
	if a == nil || a.store == nil || !a.store.Enabled() {
		return markdown, nil
	}
	matches := markdownImageRe.FindAllStringSubmatchIndex(markdown, -1)
	if len(matches) == 0 {
		return markdown, nil
	}

	// 先把每个匹配解析成 (原始URL, 可下载URL)；同一 URL 只下载一次
	type span struct {
		start, end int
		alt        string
		src        string // 清洗后的 URL（去掉空白）
	}
	spans := make([]span, 0, len(matches))
	uniq := make(map[string]bool, len(matches))
	order := make([]string, 0, len(matches))

	for _, m := range matches {
		alt := markdown[m[2]:m[3]]
		raw := strings.Join(strings.Fields(markdown[m[4]:m[5]]), "")
		if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			continue // data: / 相对路径等一律不动
		}
		spans = append(spans, span{start: m[0], end: m[1], alt: alt, src: raw})
		if !uniq[raw] {
			uniq[raw] = true
			order = append(order, raw)
		}
	}
	if len(spans) == 0 {
		return markdown, nil
	}

	archived, failed := 0, 0
	replaced := make(map[string]string, len(order))
	for i, src := range order {
		if i >= a.maxImages {
			break
		}
		newURL, err := a.archiveOne(ctx, articleID, i, src)
		if err != nil {
			failed++
			slog.Warn("图片归档失败，保留原链接", "article_id", articleID, "url", src, "err", err)
			continue
		}
		replaced[src] = newURL
		archived++
	}

	// 按区间倒序替换，避免前面的替换影响后面的偏移
	out := markdown
	for i := len(spans) - 1; i >= 0; i-- {
		sp := spans[i]
		dst, ok := replaced[sp.src]
		if !ok {
			continue
		}
		out = out[:sp.start] + "![" + sp.alt + "](" + dst + ")" + out[sp.end:]
	}
	if archived > 0 || failed > 0 {
		slog.Info("图片归档完成", "article_id", articleID, "archived", archived, "failed", failed)
	}
	return out, nil
}

func (a *ImageArchiver) archiveOne(ctx context.Context, articleID string, index int, src string) (string, error) {
	if a.ssrfCheck != nil {
		res, err := a.ssrfCheck.CheckURL(src)
		if err != nil {
			return "", fmt.Errorf("ssrf check: %w", err)
		}
		if !res.Allowed {
			return "", fmt.Errorf("ssrf blocked: %s", res.Reason)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", a.userAgent)
	// 关键：公众号图床按 Referer 放行，缺了会返回占位/403
	req.Header.Set("Referer", a.referer)
	req.Header.Set("Accept", "image/avif,image/webp,image/*,*/*;q=0.8")

	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return "", fmt.Errorf("非图片内容: %s", ct)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, a.maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > a.maxBytes {
		return "", fmt.Errorf("图片超过 %d 字节", a.maxBytes)
	}
	if len(data) < 64 {
		return "", fmt.Errorf("图片过小（%d 字节），疑似占位图", len(data))
	}

	ext := imageExt(ct)
	key := fmt.Sprintf("articles/%s/img-%02d%s", articleID, index, ext)
	url, err := a.store.Upload(ctx, key, data, ct)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	return url, nil
}

func imageExt(contentType string) string {
	switch {
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	case strings.Contains(contentType, "svg"):
		return ".svg"
	default:
		return ".jpg"
	}
}
