package htmlclean

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

var entityRe = regexp.MustCompile(`&(lt|gt|amp|quot|apos|nbsp|#\d+|#x[0-9a-fA-F]+);`)

// Cleaner 从 HTML 中提取正文，返回干净的纯文本。
type Cleaner struct{}

// NewCleaner 创建清洗器
func NewCleaner() *Cleaner {
	return &Cleaner{}
}

// Clean 清洗 HTML，返回纯文本
func (c *Cleaner) Clean(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return c.plainFromHTML(html)
	}

	content := extractContent(doc)
	if content == "" {
		bodyNodes := doc.Find("body").Nodes
		if len(bodyNodes) > 0 {
			content = walkNodes(bodyNodes[0], false)
		}
	}

	text := formatText(content)
	return strings.TrimSpace(text)
}

// blockElements 块级元素标签集合
var blockElements = map[string]bool{
	"p": true, "div": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "ul": true, "ol": true,
	"li": true, "blockquote": true, "pre": true, "hr": true,
	"article": true, "section": true, "header": true, "footer": true,
	"nav": true, "table": true, "tr": true, "td": true, "th": true,
	"figcaption": true, "figure": true,
}

// noiseTags 噪声节点选择器（CSS 字符串）
var noiseTags = []string{
	"script", "style", "noscript", "iframe", "template",
	"nav", "aside",
	"[role='navigation']", "[role='banner']", "[role='complementary']",
	".nav", ".navbar", ".sidebar", ".ad", ".ads", ".banner",
	".comment", ".comments", ".related", ".share", ".tag-cloud", ".pagination",
}

// extractContent 用语义选择器定位正文后提取文本
func extractContent(doc *goquery.Document) string {
	// 优先：微信等平台的 js_content 容器
	if el, err := doc.Find("#js_content").Html(); err == nil && el != "" {
		sdoc, err := goquery.NewDocumentFromReader(strings.NewReader(el))
		if err == nil {
			removeNoise(sdoc)
			return walkNodes(sdoc.Find("body").Nodes[0], false)
		}
	}
	// 其次：通用语义标签
	for _, sel := range []string{
		"article", "main", "[role='main']",
		".post-content,.article-content,.entry-content,.post_body",
		".content,.main-content,.article-body",
		"#content,#main,#article",
	} {
		subdoc, err := doc.Find(sel).First().Clone().Html()
		if err == nil && subdoc != "" {
			sdoc, err := goquery.NewDocumentFromReader(strings.NewReader(subdoc))
			if err == nil {
				removeNoise(sdoc)
				return walkNodes(sdoc.Find("body").Nodes[0], false)
			}
		}
	}

	body, err := doc.Find("body").Clone().Html()
	if err != nil || body == "" {
		return ""
	}
	sdoc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return ""
	}
	removeNoise(sdoc)
	bodyNodes := sdoc.Find("body").Nodes
	if len(bodyNodes) > 0 {
		return walkNodes(bodyNodes[0], false)
	}
	return ""
}

// removeNoise 移除噪声节点
func removeNoise(doc *goquery.Document) {
	for _, sel := range noiseTags {
		doc.Find(sel).Remove()
	}
	doc.Find("div:empty,section:empty,article:empty").Remove()
}

// walkNodes 递归遍历 DOM 节点，在块元素边界插入换行
func walkNodes(n *html.Node, parentIsBlock bool) string {
	var sb strings.Builder

	switch n.Type {
	case html.ElementNode:
		tag := n.Data
		isBlock := blockElements[tag]

		if isBlock && !parentIsBlock && sb.Len() > 0 {
			sb.WriteByte('\n')
		}

		// 递归处理子节点
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			sb.WriteString(walkNodes(child, isBlock))
		}

		if isBlock && !parentIsBlock {
			sb.WriteByte('\n')
		}
	case html.TextNode:
		decoded := decodeEntities(n.Data)
		sb.WriteString(decoded)
	}

	return sb.String()
}

// decodeEntities 解码 HTML 实体
func decodeEntities(s string) string {
	return entityRe.ReplaceAllStringFunc(s, func(match string) string {
		switch match {
		case "&lt;":
			return "<"
		case "&gt;":
			return ">"
		case "&amp;":
			return "&"
		case "&quot;":
			return `"`
		case "&apos;":
			return "'"
		case "&nbsp;":
			return " "
		default:
			if strings.HasPrefix(match, "&#") {
				var codepoint int
				if strings.HasPrefix(match, "&#x") {
					fmt.Sscanf(match[3:len(match)-1], "%x", &codepoint)
				} else {
					fmt.Sscanf(match[2:len(match)-1], "%d", &codepoint)
				}
				return string(rune(codepoint))
			}
			return match
		}
	})
}

// formatText 压缩连续空白为单空格
func formatText(text string) string {
	var sb strings.Builder
	prevSpace := false
	for _, r := range text {
		switch r {
		case ' ', '\t', '\r':
			if !prevSpace {
				sb.WriteByte(' ')
				prevSpace = true
			}
		case '\n':
			sb.WriteByte('\n')
			prevSpace = false
		default:
			sb.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(sb.String())
}

// plainFromHTML 兜底实现：纯正则剥除标签
func (c *Cleaner) plainFromHTML(html string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(html, " ")
	text = formatText(text)
	return strings.TrimSpace(text)
}
