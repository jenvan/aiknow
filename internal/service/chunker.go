package service

import (
	"fmt"
	"strings"

	"github.com/jenvan/aiknow/internal/model"
)

// 分块策略常量。
const (
	// ChunkStrategyStructure 结构优先：按「空行」与「标题行」切段，段内超窗再滑窗兜底，
	// 末尾短片段按 MinSize 合并。适用于分段问答 / FAQ / 条款类数据，以及按空行分段的采集正文。
	ChunkStrategyStructure = "structure"
	// ChunkStrategyWindow 纯字符滑窗：按 Size / Overlap 定长切分（历史行为，向后兼容）。
	ChunkStrategyWindow = "window"
)

// ChunkConfig 分块参数（来源为 L2 配置，后台可热改）。
type ChunkConfig struct {
	Strategy string // "structure" | "window"；空值按 "structure" 处理
	Size     int    // 单片目标大小（字符数）
	Overlap  int    // 滑窗重叠（字符数），仅用于超长段落的滑窗兜底
	MinSize  int    // 短片段合并阈值：小于此值且与上一片合计不超 Size 时并入上一片
}

// ChunkDefaults 内置默认：structure 策略、400 字一段、边界不重叠、60 字起合并。
func ChunkDefaults() ChunkConfig {
	return ChunkConfig{
		Strategy: ChunkStrategyStructure,
		Size:     400,
		Overlap:  0,
		MinSize:  60,
	}
}

// normalize 收敛非法/缺省值，保证后续逻辑拿到安全参数。
func normalizeChunkConfig(cfg ChunkConfig) ChunkConfig {
	d := ChunkDefaults()
	if cfg.Size <= 0 {
		cfg.Size = d.Size
	}
	if cfg.MinSize < 0 {
		cfg.MinSize = d.MinSize
	}
	if cfg.Overlap < 0 {
		cfg.Overlap = 0
	}
	if cfg.Overlap >= cfg.Size {
		cfg.Overlap = cfg.Size - 1
	}
	return cfg
}

// Split 将正文切分为 Chunk，按 cfg.Strategy 选择策略。这是全仓唯一的切分实现。
func Split(content string, cfg ChunkConfig) ([]model.Chunk, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("empty content")
	}
	cfg = normalizeChunkConfig(cfg)

	var chunks []model.Chunk
	switch cfg.Strategy {
	case ChunkStrategyWindow:
		chunks = splitWindow([]rune(content), cfg)
	default: // "structure" 及空值
		chunks = splitStructure(content, cfg)
	}
	return chunks, nil
}

// splitWindow 纯字符滑窗（rune 级，中文友好），带死循环保护。
func splitWindow(runes []rune, cfg ChunkConfig) []model.Chunk {
	total := len(runes)
	chunks := make([]model.Chunk, 0)
	start := 0
	for start < total {
		end := start + cfg.Size
		if end > total {
			end = total
		}
		chunks = append(chunks, model.Chunk{
			Content:   string(runes[start:end]),
			WordCount: end - start,
		})
		// 切到文末即结束：末片不满 Size 时 end 被截到 total，若不回退 overlap，
		// 下一步 start 恒定会形成死循环（历史 bug，见 ai_pipeline 原实现注释）。
		if end == total {
			break
		}
		start = end - cfg.Overlap
	}
	return chunks
}

// splitStructure 结构优先切分：
//  1. 按「空行」与「标题行（#…######）」切段，段内保留原始换行；
//  2. 段长 ≤ Size → 整段一片；> Size → 仅对该段滑窗兜底；
//  3. MinSize 合并：把过短的片段并入前一片（合计不超 Size）。
//
// 段落边界不重叠（Overlap=0）；Overlap 只在第 2 步的滑窗兜底中生效。
func splitStructure(content string, cfg ChunkConfig) []model.Chunk {
	segments := structuralSegments(content)
	chunks := make([]model.Chunk, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		segLen := len([]rune(seg))
		if segLen <= cfg.Size {
			chunks = append(chunks, model.Chunk{Content: seg, WordCount: segLen})
		} else {
			chunks = append(chunks, splitWindow([]rune(seg), cfg)...)
		}
	}
	return mergeSmallChunks(chunks, cfg.MinSize, cfg.Size)
}

// structuralSegments 按「空行」与「标题行」把正文切成若干段。
//
// 规则：
//   - 空行（纯空白行）是分段边界；
//   - 标题行（行首 1–6 个 # 后跟空白或行尾）开启新段，且标题与其后正文同段，
//     直到下一个空行或标题再分。这样「## 小节\n正文」自然成块，
//     而采集/FAQ（无 # 前缀）不受影响。
func structuralSegments(content string) []string {
	lines := strings.Split(content, "\n")
	var segs []string
	var cur []string

	flush := func() {
		s := strings.TrimSpace(strings.Join(cur, "\n"))
		if s != "" {
			segs = append(segs, s)
		}
		cur = nil
	}

	for _, ln := range lines {
		t := strings.TrimRight(ln, " \t\r")
		if t == "" {
			flush()
			continue
		}
		if isHeading(t) {
			if len(cur) > 0 {
				flush()
			}
			cur = []string{t}
			continue
		}
		cur = append(cur, t)
	}
	flush()
	return segs
}

// isHeading 判断一行是否为 Markdown 风格标题（行首 1–6 个 # 后跟空白或行尾）。
func isHeading(s string) bool {
	i := 0
	for i < len(s) && s[i] == '#' {
		i++
	}
	if i == 0 || i > 6 {
		return false
	}
	if i >= len(s) {
		return true
	}
	return s[i] == ' ' || s[i] == '\t'
}

// mergeSmallChunks 把过短片段（< minSize）并入前一片，仅当合并后不超 size。
//
// 只合并「过短」的片段：正常大小的问答段落不受影响，避免把相邻话题缝进同一块。
func mergeSmallChunks(chunks []model.Chunk, minSize, size int) []model.Chunk {
	if minSize <= 0 || len(chunks) <= 1 {
		return chunks
	}
	merged := make([]model.Chunk, 0, len(chunks))
	for _, c := range chunks {
		if len(merged) > 0 && len([]rune(c.Content)) < minSize {
			prev := merged[len(merged)-1]
			joined := prev.Content + "\n" + c.Content
			if len([]rune(joined)) <= size {
				merged[len(merged)-1] = model.Chunk{
					Content:   joined,
					WordCount: len([]rune(joined)),
				}
				continue
			}
		}
		merged = append(merged, c)
	}
	return merged
}
