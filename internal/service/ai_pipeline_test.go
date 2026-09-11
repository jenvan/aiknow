package service

import (
	"strings"
	"testing"
)

// TestSplitContentTerminates 覆盖曾导致死循环的场景（window 策略，滑窗兜底路径）。
//
// 末片不满 Size 时 end 被截到 total，若不在此处 break，start = end - overlap
// 会回退到仍小于 total 的位置，下一轮 end 又被截回 total，start 恒定不变，
// 形成死循环。几乎所有真实文章都会命中。
func TestSplitContentTerminates(t *testing.T) {
	cases := []struct {
		name       string
		length     int
		chunkSize  int
		overlap    int
		wantChunks int
	}{
		{"短于 chunkSize", 130, 400, 50, 1},
		{"恰好等于 chunkSize", 400, 400, 50, 1},
		{"略超 chunkSize", 401, 400, 50, 2},
		{"多片且末片不满", 1000, 400, 50, 3},
		{"整除且无重叠", 800, 400, 0, 2},
		{"单字符", 1, 400, 50, 1},
		{"overlap 超过 chunkSize", 500, 100, 100, 401},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := strings.Repeat("中", tc.length)
			cfg := ChunkConfig{Strategy: ChunkStrategyWindow, Size: tc.chunkSize, Overlap: tc.overlap, MinSize: 0}

			chunks, err := Split(content, cfg)
			if err != nil {
				t.Fatalf("Split 返回错误: %v", err)
			}
			if len(chunks) != tc.wantChunks {
				t.Errorf("片数 = %d, 期望 %d", len(chunks), tc.wantChunks)
			}
			// 全部内容必须被覆盖，且每片非空
			for i, c := range chunks {
				if c.WordCount <= 0 {
					t.Errorf("第 %d 片 WordCount = %d, 应为正数", i, c.WordCount)
				}
				if len([]rune(c.Content)) != c.WordCount {
					t.Errorf("第 %d 片 WordCount = %d 与实际字数 %d 不一致", i, c.WordCount, len([]rune(c.Content)))
				}
			}
			if len(chunks) > 0 {
				last := chunks[len(chunks)-1]
				if !strings.HasSuffix(content, last.Content) {
					t.Error("末片未切到正文结尾")
				}
			}
		})
	}
}

// TestSplitContentEmpty 空正文应报错而非返回空切片
func TestSplitContentEmpty(t *testing.T) {
	if _, err := Split("", ChunkConfig{}); err == nil {
		t.Error("空正文应返回错误")
	}
}
