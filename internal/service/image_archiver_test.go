package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeObjectStore struct {
	uploads map[string][]byte
	types   map[string]string
	fail    bool
}

func (f *fakeObjectStore) Enabled() bool { return true }

func (f *fakeObjectStore) Upload(_ context.Context, key string, data []byte, contentType string) (string, error) {
	if f.fail {
		return "", fmt.Errorf("upload boom")
	}
	if f.uploads == nil {
		f.uploads = map[string][]byte{}
		f.types = map[string]string{}
	}
	f.uploads[key] = data
	f.types[key] = contentType
	return "https://cos.example.com/" + key, nil
}

// 1x1 PNG
var tinyPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
	0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
}

func newTestArchiver(store ObjectStore, srv *httptest.Server) *ImageArchiver {
	a := NewImageArchiver(store, "test-agent")
	// 测试目标是 127.0.0.1，会被 SSRF 检查拦掉；置 nil 跳过校验
	a.ssrfCheck = nil
	return a
}

func TestArchiveReplacesImagesAndDedupes(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Referer") != "https://mp.weixin.qq.com/" {
			t.Errorf("缺 Referer，公众号图床会拒绝")
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(tinyPNG)
	}))
	defer srv.Close()

	store := &fakeObjectStore{}
	a := newTestArchiver(store, srv)

	md := fmt.Sprintf("开头\n\n![图一](%s/a.png)\n\n中间\n\n![图二](%s/b.png)\n\n![重复](%s/a.png)\n", srv.URL, srv.URL, srv.URL)
	out, err := a.Archive(context.Background(), "art-1", md)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}

	if hits != 2 {
		t.Errorf("下载次数 = %d，去重后应为 2", hits)
	}
	if strings.Contains(out, srv.URL) {
		t.Errorf("原链接未被替换:\n%s", out)
	}
	if strings.Count(out, "https://cos.example.com/articles/art-1/img-") != 3 {
		t.Errorf("替换次数不对:\n%s", out)
	}
	if len(store.uploads) != 2 {
		t.Errorf("上传数 = %d，want 2", len(store.uploads))
	}
}

func TestArchiveKeepsOriginalOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "ok") {
			w.Header().Set("Content-Type", "image/png")
			w.Write(tinyPNG)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a := newTestArchiver(&fakeObjectStore{}, srv)
	md := fmt.Sprintf("![好图](%s/ok.png)\n![坏图](%s/missing.png)\n", srv.URL, srv.URL)

	out, err := a.Archive(context.Background(), "art-2", md)
	if err != nil {
		t.Fatalf("Archive 不应因单张失败报错: %v", err)
	}
	if !strings.Contains(out, srv.URL+"/missing.png") {
		t.Errorf("失败的图应保留原链接:\n%s", out)
	}
	if strings.Contains(out, srv.URL+"/ok.png") {
		t.Errorf("成功的图应被替换:\n%s", out)
	}
}

func TestArchiveSkipsNonImageContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>not an image</html>"))
	}))
	defer srv.Close()

	a := newTestArchiver(&fakeObjectStore{}, srv)
	md := fmt.Sprintf("![伪装成图](%s/x.png)", srv.URL)
	out, _ := a.Archive(context.Background(), "art-3", md)
	if !strings.Contains(out, srv.URL) {
		t.Errorf("非图片响应不应被归档:\n%s", out)
	}
}

func TestArchiveNoImagesPassesThrough(t *testing.T) {
	a := NewImageArchiver(&fakeObjectStore{}, "")
	md := "纯文本，没有图片。"
	out, err := a.Archive(context.Background(), "art-4", md)
	if err != nil || out != md {
		t.Fatalf("无图片时应原样返回: %v %q", err, out)
	}
}

func TestArchiveDisabledStorePassesThrough(t *testing.T) {
	a := NewImageArchiver(nil, "")
	md := "![x](https://mmbiz.qpic.cn/a.png)"
	out, err := a.Archive(context.Background(), "art-5", md)
	if err != nil || out != md {
		t.Fatalf("未配置存储时应原样返回: %v", err)
	}
}

// 公众号正文里长 URL 会被折行，必须仍能识别并清洗
func TestArchiveHandlesNewlineInURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "from=appmsg&watermark=1" {
			t.Errorf("URL 未清洗：query=%q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(tinyPNG)
	}))
	defer srv.Close()

	a := newTestArchiver(&fakeObjectStore{}, srv)
	md := "![折行图](" + srv.URL + "/640?from=appmsg&\nwatermark=1)\n正文\n"
	out, err := a.Archive(context.Background(), "art-6", md)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if strings.Contains(out, "mmbiz") || strings.Contains(out, "\nwatermark") {
		t.Errorf("折行 URL 未被处理:\n%q", out)
	}
	if !strings.Contains(out, "https://cos.example.com/articles/art-6/img-00") {
		t.Errorf("未替换为 COS 链接:\n%q", out)
	}
}
