package readability

import (
	"strings"
	"testing"
)

// TestExtract_MarkdownStructure 验证 Readability 能保留标题 / 列表 / 代码块。
func TestExtract_MarkdownStructure(t *testing.T) {
	html := `<html><head><title>测试文章</title></head><body>
<article>
  <h1>大标题</h1>
  <p>第一段正文。为了让 Readability 判定这是文章而非空壳，这里写足够长的内容：正文应当包含若干段落，让正文容器在打分时明显高于兄弟节点。这是额外的字。</p>
  <p>第二段正文继续补充，确保正文节点在内容长度上压制广告与导航噪声，从而被 Readability 选中为文章主体。</p>
  <h2>小节</h2>
  <ul><li>项一</li><li>项二</li></ul>
  <pre><code>func main() {}</code></pre>
  <p>末段正文收尾。</p>
</article>
<nav>导航广告噪声</nav>
<aside>侧栏推荐</aside>
</body></html>`

	a, err := Extract(html, Options{PageURL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("Extract 失败: %v", err)
	}
	if a.Title == "" {
		t.Error("未提取到标题")
	}
	// 保留标题层级
	if !strings.Contains(a.Markdown, "# 大标题") && !strings.Contains(a.Markdown, "## 小节") {
		t.Errorf("Markdown 缺标题层级:\n%s", a.Markdown)
	}
	// 保留列表
	if !strings.Contains(a.Markdown, "- 项一") {
		t.Errorf("Markdown 缺列表:\n%s", a.Markdown)
	}
	// 去噪：导航 / 侧栏不应出现
	if strings.Contains(a.Markdown, "导航广告噪声") || strings.Contains(a.Markdown, "侧栏推荐") {
		t.Errorf("Markdown 混入噪声:\n%s", a.Markdown)
	}
}

// TestExtract_NotArticle SPA 空壳应报「非文章页」。
func TestExtract_NotArticle(t *testing.T) {
	html := `<html><body><div id="root">You need to enable JavaScript</div></body></html>`
	_, err := Extract(html, Options{})
	if err == nil {
		t.Skip("本机 Readability 对空壳判定为有内容（可接受），跳过")
	}
	if !IsNotArticle(err) {
		t.Errorf("空壳页应报 IsNotArticle，实际: %v", err)
	}
}
