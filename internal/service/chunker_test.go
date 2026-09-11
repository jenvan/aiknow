package service

import (
	"strings"
	"testing"

	"github.com/jenvan/aiknow/internal/model"
)

// 结构优先切分（structure）的行为契约测试。

// TestSplitStructure_FAQ 每个 Q/A 块（≤ 窗口）应恰好成一个 chunk，且不相邻问答互不污染。
func TestSplitStructure_FAQ(t *testing.T) {
	faq := "Q: 什么是 CAS？\nA: CAS 是 compare-and-swap，无主节点，两个写者抢锁只有一个赢，其余重读重验重试。" +
		"\n\n" +
		"Q: 大仓库怎么远程读 pack？\nA: pack 索引放本地，数据按 HTTP range 从桶里拉，UI 点开一个对象才拉那一个对象需要的字节。"
	// 两块各自远小于 size，块间空行；min_size=60 不影响（块都 > 60 字）
	chunks, err := Split(faq, ChunkConfig{Strategy: ChunkStrategyStructure, Size: 400, Overlap: 0, MinSize: 60})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunk 数 = %d, 期望 2（一问一答一块），got: %q", len(chunks), runeLens(chunks))
	}
	// 不跨问答污染：第 1 块只含 CAS、不含 pack；第 2 块相反
	if !strings.Contains(chunks[0].Content, "CAS") || strings.Contains(chunks[0].Content, "pack") {
		t.Errorf("chunk0 应只含 CAS，实际: %q", chunks[0].Content)
	}
	if !strings.Contains(chunks[1].Content, "pack") || strings.Contains(chunks[1].Content, "CAS") {
		t.Errorf("chunk1 应只含 pack，实际: %q", chunks[1].Content)
	}
}

// TestSplitStructure_OverSizeSlidingFallback 单个段落超过 size 时，仅对该段滑窗兜底。
func TestSplitStructure_OverSizeSlidingFallback(t *testing.T) {
	// 一整段（无空行）1000 字，size=400、overlap=50 → 滑窗 3 片
	seg := strings.Repeat("中", 1000)
	chunks, err := Split(seg, ChunkConfig{Strategy: ChunkStrategyStructure, Size: 400, Overlap: 50, MinSize: 0})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	// start=0,350,700 → 片长 400/400/300，共 3 片
	if len(chunks) != 3 {
		t.Fatalf("超长段滑窗片数 = %d, 期望 3, got: %q", len(chunks), runeLens(chunks))
	}
	// 相邻片段有 50 字重叠：第 0 片尾部 50 字 == 第 1 片头部 50 字
	tail := []rune(chunks[0].Content)[len([]rune(chunks[0].Content))-50:]
	head := []rune(chunks[1].Content)[:50]
	if string(tail) != string(head) {
		t.Errorf("相邻片重叠 50 字不符: tail=%q head=%q", string(tail), string(head))
	}
}

// TestSplitStructure_Heading 标题行与其正文同段，标题/小节正确分块。
func TestSplitStructure_Heading(t *testing.T) {
	doc := "# 标题\n\n" +
		"## 小节一\n" + strings.Repeat("一", 100) + "\n\n" +
		"## 小节二\n" + strings.Repeat("二", 100)
	// min_size=0 避免短段合并干扰，size 足够大不触发滑窗
	chunks, err := Split(doc, ChunkConfig{Strategy: ChunkStrategyStructure, Size: 400, Overlap: 0, MinSize: 0})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	// 三段：# 标题 / (## 小节一 + 正文) / (## 小节二 + 正文)
	if len(chunks) != 3 {
		t.Fatalf("chunk 数 = %d, 期望 3, got: %q", len(chunks), runeLens(chunks))
	}
	// 小节标题须与其正文在同一 chunk（不拆开）
	if !strings.Contains(chunks[1].Content, "## 小节一") || !strings.Contains(chunks[1].Content, strings.Repeat("一", 50)) {
		t.Errorf("chunk1 应包含『## 小节一』与其正文，实际: %q", chunks[1].Content)
	}
	if !strings.Contains(chunks[2].Content, "## 小节二") {
		t.Errorf("chunk2 应包含『## 小节二』，实际: %q", chunks[2].Content)
	}
}

// TestSplitStructure_MinSizeMerge 过短片段按 min_size 并入前一片（合计不超 size）。
func TestSplitStructure_MinSizeMerge(t *testing.T) {
	// 前段 70 字（> 60），后段 3 字（< 60），空行分隔
	doc := strings.Repeat("长", 70) + "\n\n" + "短段"
	chunks, err := Split(doc, ChunkConfig{Strategy: ChunkStrategyStructure, Size: 400, Overlap: 0, MinSize: 60})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	// 后段 3 字 < 60，并入前段（70+1+3=74 ≤ 400）→ 1 chunk
	if len(chunks) != 1 {
		t.Fatalf("合并后 chunk 数 = %d, 期望 1, got: %q", len(chunks), runeLens(chunks))
	}
	if !strings.Contains(chunks[0].Content, "短段") || !strings.Contains(chunks[0].Content, strings.Repeat("长", 70)) {
		t.Errorf("合并后的 chunk 应同时含前后段，实际: %q", chunks[0].Content)
	}
}

// TestSplitStructure_MinSizeZeroNoMerge min_size=0 时不做合并。
func TestSplitStructure_MinSizeZeroNoMerge(t *testing.T) {
	doc := strings.Repeat("长", 70) + "\n\n" + "短段"
	chunks, err := Split(doc, ChunkConfig{Strategy: ChunkStrategyStructure, Size: 400, Overlap: 0, MinSize: 0})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("min_size=0 应保留 2 段, 实际 %d", len(chunks))
	}
}

// TestSplitStructure_NoBlankLineFallsBackToSliding 无空行无标题时退化为滑窗。
func TestSplitStructure_NoBlankLineFallsBackToSliding(t *testing.T) {
	doc := strings.Repeat("a", 900) // 单段 900 字，size=400、overlap=0 → 3 片（步长 400）
	chunks, err := Split(doc, ChunkConfig{Strategy: ChunkStrategyStructure, Size: 400, Overlap: 0, MinSize: 0})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("无空行单段滑窗片数 = %d, 期望 3", len(chunks))
	}
}

// TestSplitWindow_StrategyExplicit 显式 window 策略 = 历史定长滑窗行为。
func TestSplitWindow_StrategyExplicit(t *testing.T) {
	doc := strings.Repeat("中", 1000)
	chunks, err := Split(doc, ChunkConfig{Strategy: ChunkStrategyWindow, Size: 400, Overlap: 50, MinSize: 0})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("window 策略 1000 字片数 = %d, 期望 3", len(chunks))
	}
}

// TestSplit_Empty 空/纯空白正文报错。
func TestSplit_Empty(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\n\n"} {
		if _, err := Split(in, ChunkConfig{}); err == nil {
			t.Errorf("空/空白正文 %q 应报错", in)
		}
	}
}

// runeLens 返回各 chunk 的 rune 长度列表，用于失败时输出诊断。
func runeLens(chunks []model.Chunk) []int {
	lens := make([]int, len(chunks))
	for i, c := range chunks {
		lens[i] = len([]rune(c.Content))
	}
	return lens
}
