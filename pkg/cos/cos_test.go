package cos

import (
	"strings"
	"testing"
	"time"
)

func TestConfigEnabled(t *testing.T) {
	full := Config{SecretID: "id", SecretKey: "key", Bucket: "b-125", Region: "ap-guangzhou"}
	if !full.Enabled() {
		t.Error("凭据与桶地域齐全时 Enabled 应为 true")
	}
	for _, c := range []Config{
		{SecretKey: "key", Bucket: "b", Region: "r"},
		{SecretID: "id", Bucket: "b", Region: "r"},
		{SecretID: "id", SecretKey: "key", Region: "r"},
		{SecretID: "id", SecretKey: "key", Bucket: "b"},
	} {
		if c.Enabled() {
			t.Errorf("缺字段时 Enabled 应为 false: %+v", c)
		}
	}
}

func TestUpload_NotConfigured(t *testing.T) {
	c := New(Config{})
	if _, err := c.Upload(t.Context(), "a.png", []byte("x"), "image/png"); err == nil {
		t.Error("未配置时 Upload 应报错")
	}
}

func TestFullKey(t *testing.T) {
	c := New(Config{PathPrefix: "/screenshots/", Bucket: "b", Region: "r", SecretID: "i", SecretKey: "k"})
	if got := c.fullKey("/2026/09/a.png"); got != "screenshots/2026/09/a.png" {
		t.Errorf("fullKey = %q", got)
	}
	// 无前缀
	c2 := New(Config{Bucket: "b", Region: "r", SecretID: "i", SecretKey: "k"})
	if got := c2.fullKey("a.png"); got != "a.png" {
		t.Errorf("无前缀 fullKey = %q", got)
	}
}

func TestPublicURL(t *testing.T) {
	// 默认域名
	c := New(Config{Bucket: "mybucket-1250000000", Region: "ap-guangzhou", SecretID: "i", SecretKey: "k"})
	u := c.publicURL("screenshots/2026/09/a.png")
	if !strings.Contains(u, "mybucket-1250000000.cos.ap-guangzhou.myqcloud.com") {
		t.Errorf("默认域名 URL = %q", u)
	}
	if !strings.HasSuffix(u, "/screenshots/2026/09/a.png") {
		t.Errorf("URL 路径不对: %q", u)
	}

	// 自定义 Endpoint（CDN）
	c2 := New(Config{Endpoint: "https://cdn.example.com/", SecretID: "i", SecretKey: "k"})
	if got := c2.publicURL("a.png"); got != "https://cdn.example.com/a.png" {
		t.Errorf("自定义 Endpoint URL = %q", got)
	}

	// 自定义 Endpoint 缺 scheme：应自动补 https://（否则浏览器当相对路径）
	c3 := New(Config{Endpoint: "cdn.example.com", SecretID: "i", SecretKey: "k"})
	if got := c3.publicURL("a.png"); got != "https://cdn.example.com/a.png" {
		t.Errorf("缺 scheme 的 Endpoint 应补 https://，实际 %q", got)
	}
	// 带 http:// 的不应被改成 https
	c4 := New(Config{Endpoint: "http://intranet.local", SecretID: "i", SecretKey: "k"})
	if got := c4.publicURL("a.png"); got != "http://intranet.local/a.png" {
		t.Errorf("http:// 不应被改写，实际 %q", got)
	}
}

func TestBuildScreenshotKey(t *testing.T) {
	ts := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	got := BuildScreenshotKey("abc-123", ts)
	// 按文章归档、固定文件名：重截覆盖旧对象，不留孤儿；日期不参与路径
	if got != "articles/abc-123/screenshot.png" {
		t.Errorf("BuildScreenshotKey = %q", got)
	}
	if strings.Contains(got, "2026") {
		t.Errorf("路径不应包含日期: %q", got)
	}
	// 特殊字符被清洗
	got2 := BuildScreenshotKey("a/b c.png", ts)
	if strings.Contains(got2, " ") {
		t.Errorf("空格未清洗: %q", got2)
	}
	// 空 ID
	if got3 := BuildScreenshotKey("", ts); !strings.Contains(got3, "unknown") {
		t.Errorf("空 ID 应回落 unknown: %q", got3)
	}
}
