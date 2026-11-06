// Package cos 封装腾讯云对象存储（COS）上传。
//
// 仅暴露本项目需要的「上传字节并返回可访问 URL」能力，凭据与桶配置由上层
// 注入。不碰项目其他包，保持可独立测试（测试用 httptest 模拟 COS 端点）。
package cos

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// Config COS 上传配置。
type Config struct {
	// SecretID / SecretKey 腾讯云 API 密钥。缺任一则 Enabled() 为 false。
	SecretID  string
	SecretKey string
	// Bucket 存储桶名，含 appid 后缀，如 "aiknow-1300000000"。
	Bucket string
	// Region 地域，如 "ap-guangzhou"。
	Region string
	// Endpoint 自定义域名（CDN/加速域名）。为空时用 COS 默认域名拼 URL。
	Endpoint string
	// PathPrefix 对象键前缀，如 "screenshots"。为空则直接放桶根。
	PathPrefix string
	// Timeout 单次上传超时。默认 30s。
	Timeout time.Duration
}

// Enabled 配置是否完整到可上传。
func (c Config) Enabled() bool {
	return c.SecretID != "" && c.SecretKey != "" && c.Bucket != "" && c.Region != ""
}

// Client COS 上传客户端。
type Client struct {
	cfg  Config
	sdk  *cos.Client
	base *cos.BaseURL
}

// New 创建 COS 客户端。未提供 Timeout 时填默认 30s。
func New(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	// NewBucketURL 拼默认桶域名：https://<bucket>.cos.<region>.myqcloud.com
	bucketURL, err := cos.NewBucketURL(cfg.Bucket, cfg.Region, true)
	if err != nil {
		// 桶名/地域非法：构造失败时返回不可用的客户端，Enabled 仍按配置判定，
		// 由 Upload 返回明确错误，避免构造期 panic。
		return &Client{cfg: cfg}
	}

	base := &cos.BaseURL{BucketURL: bucketURL}
	return &Client{
		cfg:  cfg,
		base: base,
		sdk:  cos.NewClient(base, newAuthorizedClient(cfg)),
	}
}

// newAuthorizedClient 带 COS 签名的 http.Client。
func newAuthorizedClient(cfg Config) *http.Client {
	return &http.Client{
		Timeout: cfg.Timeout,
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
		},
	}
}

// Upload 上传字节到 COS，返回可访问的对象 URL。
//
// objectKey 为桶内路径（如 "screenshots/2026/09/abc.png"），内部会加 PathPrefix。
// contentType 如 "image/png"。
func (c *Client) Upload(ctx context.Context, objectKey string, data []byte, contentType string) (string, error) {
	if !c.cfg.Enabled() {
		return "", fmt.Errorf("cos: not configured")
	}
	if c.sdk == nil {
		return "", fmt.Errorf("cos: invalid bucket/region %q/%q", c.cfg.Bucket, c.cfg.Region)
	}
	if objectKey == "" {
		return "", fmt.Errorf("cos: empty object key")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("cos: empty data")
	}

	key := c.fullKey(objectKey)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: contentType,
		},
	}
	if _, err := c.sdk.Object.Put(ctx, key, bytes.NewReader(data), opt); err != nil {
		return "", fmt.Errorf("cos: put object: %w", err)
	}

	return c.publicURL(key), nil
}

// UploadScreenshot 上传某文章的截图，返回可访问 URL（满足 service.ScreenshotStore）。
//
// 对象键固定为 articles/<id>/screenshot.png：**同篇文章重截即覆盖**，不会堆积孤儿文件。
// URL 附带 ?v=<unix> 作缓存键——对象路径不变，但浏览器/CDN 看到新查询串会重新拉取，
// 否则重截后页面可能还是旧图。
func (c *Client) UploadScreenshot(ctx context.Context, id string, data []byte) (string, error) {
	url, err := c.Upload(ctx, BuildScreenshotKey(id, time.Now()), data, "image/png")
	if err != nil {
		return "", err
	}
	return url + "?v=" + strconv.FormatInt(time.Now().Unix(), 10), nil
}

// Enabled 配置是否完整到可上传（满足 service.ScreenshotStore 接口）。
func (c *Client) Enabled() bool {
	return c.cfg.Enabled()
}

// fullKey 拼接 PathPrefix 与对象键。
func (c *Client) fullKey(objectKey string) string {
	prefix := strings.Trim(c.cfg.PathPrefix, "/")
	key := strings.TrimLeft(objectKey, "/")
	if prefix == "" {
		return key
	}
	return prefix + "/" + key
}

// publicURL 生成对象的可访问 URL。配置了自定义 Endpoint 时用其域名。
func (c *Client) publicURL(key string) string {
	if c.cfg.Endpoint != "" {
		return normalizeScheme(c.cfg.Endpoint) + "/" + key
	}
	if c.base != nil && c.base.BucketURL != nil {
		u := *c.base.BucketURL
		u.Path = "/" + key
		return u.String()
	}
	return key
}

// normalizeScheme 规范化自定义域名：去掉尾部斜杠，缺 scheme 时补 https://。
//
// 缺了 scheme 的 URL（如 "cos.example.com/a.png"）会被浏览器当作相对路径，
// 导致图片加载失败——调用方容易漏写，这里统一兜底。
func normalizeScheme(endpoint string) string {
	ep := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if ep == "" {
		return ep
	}
	if !strings.Contains(ep, "://") {
		return "https://" + ep
	}
	return ep
}

// BuildScreenshotKey 生成截图对象键：<yyyy>/<mm>/<id>.png。
//
// 只含年月分层，不含 "screenshots" 前缀——路径命名空间由 Config.PathPrefix 负责
// （默认 "screenshots"）。若此处再硬编码前缀，会与 PathPrefix 叠加成
// "aiknow/screenshots/screenshots/..."。
// BuildScreenshotKey 截图对象键：articles/<id>/screenshot.png。
//
// 刻意不用日期目录：按 id 归档后「一篇文章的所有产物」在同一前缀下
// （截图 + 正文图片），删文章时整个目录一起清；固定文件名保证重截覆盖旧对象。
// t 参数保留只为兼容调用方签名。
func BuildScreenshotKey(id string, t time.Time) string {
	return fmt.Sprintf("articles/%s/screenshot.png", sanitizeKeySegment(id))
}

// sanitizeKeySegment 清洗对象键片段，只保留安全字符。
func sanitizeKeySegment(s string) string {
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		default:
			// 其余字符（含 . / 空格 等）替换为 -，避免破坏对象键结构
			b.WriteByte('-')
		}
	}
	return b.String()
}
