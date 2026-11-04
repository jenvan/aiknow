package weixin

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// maxReplyRunes 单条回复的最大字符数。
//
// 微信文本消息有长度上限（具体字节数官方未公开），过长会被截断或发送失败，
// 故统一按 1200 字符切分后分条发送。
const maxReplyRunes = 1200

// splitReply 把一段长文本切成多条，优先在换行处断开，避免把一行从中间劈开。
func splitReply(text string, limit int) []string {
	if limit <= 0 {
		limit = maxReplyRunes
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	for utf8.RuneCountInString(text) > limit {
		runes := []rune(text)
		cut := limit
		// 在窗口内从后往前找换行
		if idx := lastRuneIndex(runes[:limit], '\n'); idx > limit/2 {
			cut = idx + 1
		}
		out = append(out, strings.TrimRight(string(runes[:cut]), "\n"))
		text = strings.TrimLeft(string(runes[cut:]), "\n")
	}
	if strings.TrimSpace(text) != "" {
		out = append(out, text)
	}
	return out
}

func lastRuneIndex(runes []rune, target rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == target {
			return i
		}
	}
	return -1
}

// statusLabel 把处理状态翻译成给人看的中文。
func statusLabel(status string) string {
	switch status {
	case "pending":
		return "排队中"
	case "processing":
		return "正在处理"
	case "completed":
		return "已完成"
	case "failed":
		return "处理失败"
	default:
		return status
	}
}

const helpText = `AiKnow 爱脑 · 微信入口

发 **文章链接** → 采集入库
  （公众号文章：右上角 ··· → 复制链接 → 粘贴发送）

发 **关键词** → 搜索知识库文章
  （多个关键词用空格分隔）

/status   最近入库的文章与处理状态
/help     显示本说明

说明：采集后 AI 摘要与切片需要几十秒，稍后用 /status 查看进度。`

const usageHint = `没看懂这条消息。

发文章链接可以采集入库，发关键词可以搜文章。发 /help 看详细用法。`

func formatCollecting(url string) string {
	return "⏳ 正在采集：\n" + url + "\n（公众号文章约需几秒）"
}

func formatCollected(a articleBrief) string {
	var b strings.Builder
	b.WriteString("✅ 已收录\n《")
	b.WriteString(a.Title)
	b.WriteString("》\n状态：")
	b.WriteString(statusLabel(a.Status))
	b.WriteString("（AI 摘要与切片在后台进行，稍后用 /status 查看）")
	if a.SourceURL != "" {
		b.WriteString("\n原文：")
		b.WriteString(a.SourceURL)
	}
	return b.String()
}

func formatDuplicate(url string) string {
	return "ℹ️ 这篇已经收录过了，没有重复入库。\n" + url
}

func formatCollectFailed(url, reason string) string {
	return fmt.Sprintf("❌ 采集失败：%s\n%s", reason, url)
}

func formatSearch(results []articleBrief, keyword string) string {
	if len(results) == 0 {
		return fmt.Sprintf("没找到与「%s」相关的文章。\n换个关键词试试，或发文章链接直接入库。", keyword)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🔍 「%s」找到 %d 篇：\n", keyword, len(results)))
	for i, a := range results {
		b.WriteString(fmt.Sprintf("\n%d. %s\n", i+1, a.Title))
		meta := a.metaLine()
		if meta != "" {
			b.WriteString("   " + meta + "\n")
		}
		if a.SourceURL != "" {
			b.WriteString("   " + a.SourceURL + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatChunks(chunks []chunkBrief, query string) string {
	if len(chunks) == 0 {
		return fmt.Sprintf("没找到与「%s」相关的内容。", query)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🧠 「%s」相关片段：\n", query))
	for i, c := range chunks {
		b.WriteString(fmt.Sprintf("\n%d. 《%s》\n", i+1, c.ArticleTitle))
		b.WriteString("   " + snippet(c.Content, 120) + "\n")
		if c.SourceURL != "" {
			b.WriteString("   " + c.SourceURL + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatRecent(articles []articleBrief) string {
	if len(articles) == 0 {
		return "知识库还没有文章，发个链接试试。"
	}
	var b strings.Builder
	b.WriteString("最近入库：\n")
	for i, a := range articles {
		b.WriteString(fmt.Sprintf("\n%d. %s\n   状态：%s", i+1, a.Title, statusLabel(a.Status)))
	}
	return b.String()
}

// snippet 截断正文，保留开头若干字符。
func snippet(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
