package service

import (
	"context"
	"strings"
	"testing"
)

// TestSplitContentTerminates 覆盖曾导致死循环的场景。
//
// 末片不满 chunkSize 时，end 被截到 totalLen，而 start = end - overlap 会
// 回退到仍小于 totalLen 的位置，下一轮 end 又被截回 totalLen，start 恒定不变，
// chunks 无限增长直至耗尽内存。几乎所有真实文章都会命中。
func TestSplitContentTerminates(t *testing.T) {
	p := &AIPipeline{}
	ctx := context.Background()

	cases := []struct {
		name       string
		length     int
		chunkSize  int
		overlap    int
		wantChunks int
	}{
		// 曾经的死循环用例：正文短于 chunkSize，一片装得下
		{"短于 chunkSize", 130, 400, 50, 1},
		{"恰好等于 chunkSize", 400, 400, 50, 1},
		// 步长 350，第二片起点 350，覆盖到 401 即结束
		{"略超 chunkSize", 401, 400, 50, 2},
		{"多片且末片不满", 1000, 400, 50, 3},
		// 无重叠时步长即 chunkSize，整除边界不应多出空片
		{"整除且无重叠", 800, 400, 0, 2},
		{"单字符", 1, 400, 50, 1},
		// overlap >= chunkSize 被收敛为 chunkSize-1，步长退化为 1：
		// start 取 0..400 各一片，仍能前进并终止
		{"overlap 超过 chunkSize", 500, 100, 100, 401},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := strings.Repeat("中", tc.length)

			chunks, err := p.SplitContent(ctx, content, tc.chunkSize, tc.overlap)
			if err != nil {
				t.Fatalf("SplitContent 返回错误: %v", err)
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
					t.Errorf("第 %d 片 WordCount = %d 与实际字数 %d 不一致",
						i, c.WordCount, len([]rune(c.Content)))
				}
			}
			// 末片须切到正文结尾
			last := chunks[len(chunks)-1]
			if !strings.HasSuffix(content, last.Content) {
				t.Error("末片未切到正文结尾")
			}
		})
	}
}

// TestSplitContentEmpty 空正文应报错而非返回空切片
func TestSplitContentEmpty(t *testing.T) {
	p := &AIPipeline{}
	if _, err := p.SplitContent(context.Background(), "", 400, 50); err == nil {
		t.Error("空正文应返回错误")
	}
}
