// Package readability 基于 Mozilla Readability（go 移植版）提取正文，
// 输出结构化 Markdown。用于「通用文章 / 文档站」场景——相比选择器启发式，
// 它能保留标题层级、列表、表格、代码块，对后续分块更友好。
//
// 微信等已定向优化的平台仍走 pkg/htmlclean 的专用规则；本包处理其余情况。
package readability

import (
	"bytes"
	"net/url"
	"strings"

	readability "codeberg.org/readeck/go-readability/v2"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// Article 一次提取的产物。
type Article struct {
	// Title 提取到的文章标题（Readability 从 meta/正文推断）。可能为空。
	Title string
	// Byline 作者（若可提取）。
	Byline string
	// Markdown 正文的 Markdown 表示（保留标题层级/列表/表格/代码块）。
	Markdown string
	// Text 正文的纯文本表示（用于最小内容长度校验、摘要等）。
	Text string
}

// Options 提取选项。
type Options struct {
	// PageURL 页面 URL，供 Readability 解析相对链接、推断标题。可为空。
	PageURL string
}

// Extract 从 HTML 字符串提取正文，返回 Markdown + 纯文本。
//
// htmlDoc 是「渲染后」的完整 HTML（或 http 抓到的原始 HTML）。readability
// 会自行打分选出正文节点，并清理广告/导航/评论等噪声。
func Extract(htmlDoc string, opts Options) (*Article, error) {
	var pageURL *url.URL
	if opts.PageURL != "" {
		u, err := url.Parse(opts.PageURL)
		if err != nil {
			// URL 非法不应阻断提取，退化为无 URL 上下文
			pageURL = nil
		} else {
			pageURL = u
		}
	}

	article, err := readability.FromReader(strings.NewReader(htmlDoc), pageURL)
	if err != nil {
		return nil, err
	}
	// Node 为空表示 Readability 判定「非文章页」（如纯 SPA 空壳、目录页）。
	if article.Node == nil {
		return nil, errNotArticle
	}

	var htmlBuf bytes.Buffer
	if err := article.RenderHTML(&htmlBuf); err != nil {
		return nil, err
	}

	var textBuf bytes.Buffer
	// RenderText 产出 InnerText（含换行），用于长度校验
	if err := article.RenderText(&textBuf); err != nil {
		textBuf.Reset()
		// 文本渲染失败不致命，Markdown 仍可用
	}

	md, merr := htmltomarkdown.ConvertString(htmlBuf.String())
	if merr != nil {
		// Markdown 转换失败时降级为纯 HTML，保证内容不丢
		md = htmlBuf.String()
	}

	return &Article{
		Title:    article.Title(),
		Byline:   article.Byline(),
		Markdown: strings.TrimSpace(md),
		Text:     strings.TrimSpace(textBuf.String()),
	}, nil
}

// errNotArticle 标记「Readability 判定非文章页」。调用方可据此决定是否回退到
// 选择器启发式（htmlclean）。
var errNotArticle = readabilityErr{msg: "readability: not an article (no content node)"}

type readabilityErr struct{ msg string }

func (e readabilityErr) Error() string { return e.msg }

// IsNotArticle 判断 err 是否为「非文章页」。
func IsNotArticle(err error) bool {
	_, ok := err.(readabilityErr)
	return ok
}
