package htmlclean

import (
	"regexp"
	"strings"
)

// Cleaner HTML 清洗器
type Cleaner struct {
	// 扩展字段，后续可接入 bluemonday
	tagRegex *regexp.Regexp
}

// NewCleaner 创建清洗器
func NewCleaner() *Cleaner {
	return &Cleaner{
		tagRegex: regexp.MustCompile(`<[^>]*>`),
	}
}

// Clean 清洗 HTML，返回纯文本
func (c *Cleaner) Clean(html string) string {
	// TODO: 接入 bluemonday
	// 当前实现：移除 HTML 标签
	result := c.tagRegex.ReplaceAllString(html, " ")

	// 解码常见实体
	result = strings.ReplaceAll(result, "&lt;", " ")
	result = strings.ReplaceAll(result, "&gt;", " ")
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&quot;", `"`)

	// 压缩空白
	var sb strings.Builder
	var lastSpace bool
	for _, r := range result {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !lastSpace {
				sb.WriteByte(' ')
				lastSpace = true
			}
		} else {
			sb.WriteRune(r)
			lastSpace = false
		}
	}

	return strings.TrimSpace(sb.String())
}
