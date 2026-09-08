package pgqueue

import (
	"testing"
	"time"
)

func TestDefaultConfigFillsZeroValues(t *testing.T) {
	q := NewQueueWithConfig(nil, "t", Config{})
	def := DefaultConfig()

	if q.config.LeaseDuration != def.LeaseDuration {
		t.Errorf("LeaseDuration 未取默认值：%v", q.config.LeaseDuration)
	}
	if q.config.BaseBackoff != def.BaseBackoff {
		t.Errorf("BaseBackoff 未取默认值：%v", q.config.BaseBackoff)
	}
	if q.config.MaxBackoff != def.MaxBackoff {
		t.Errorf("MaxBackoff 未取默认值：%v", q.config.MaxBackoff)
	}
	if q.config.ReclaimInterval != def.ReclaimInterval {
		t.Errorf("ReclaimInterval 未取默认值：%v", q.config.ReclaimInterval)
	}
}

func TestBackoffExponentialGrowth(t *testing.T) {
	q := NewQueueWithConfig(nil, "t", Config{
		BaseBackoff: 10 * time.Second,
		MaxBackoff:  10 * time.Minute,
	})

	cases := []struct {
		retryCount int
		want       time.Duration
	}{
		{0, 10 * time.Second},  // 越界下限，按第 1 次算
		{1, 10 * time.Second},  // 10 * 2^0
		{2, 20 * time.Second},  // 10 * 2^1
		{3, 40 * time.Second},  // 10 * 2^2
		{6, 320 * time.Second}, // 10 * 2^5，仍在 10 分钟上限内
		{7, 10 * time.Minute},  // 10 * 2^6 = 640s 超过上限，截断
		{100, 10 * time.Minute},
	}

	for _, c := range cases {
		if got := q.backoffFor(c.retryCount); got != c.want {
			t.Errorf("backoffFor(%d) = %v，期望 %v", c.retryCount, got, c.want)
		}
	}
}

func TestBackoffNeverExceedsMax(t *testing.T) {
	q := NewQueueWithConfig(nil, "t", Config{
		BaseBackoff: 1 * time.Second,
		MaxBackoff:  30 * time.Second,
	})

	// 覆盖可能触发 float64 溢出的区间
	for n := 1; n <= 200; n++ {
		got := q.backoffFor(n)
		if got <= 0 {
			t.Fatalf("backoffFor(%d) 返回非正值 %v", n, got)
		}
		if got > 30*time.Second {
			t.Fatalf("backoffFor(%d) = %v 超过上限", n, got)
		}
	}
}

func TestStopReclaimerWithoutStartIsSafe(t *testing.T) {
	q := NewQueue(nil, "t")
	q.StopReclaimer() // 不应 panic 或阻塞
}

func TestQueueName(t *testing.T) {
	if got := NewQueue(nil, "article_processing").Name(); got != "article_processing" {
		t.Errorf("Name() = %q", got)
	}
}

// TestQueueIntegration 完整的入队/出队/租约/死信流程需要数据库。
// 运行方式见 docs/DATABASE_SETUP.md。
func TestQueueIntegration(t *testing.T) {
	t.Skip("需要数据库连接（ParadeDB），见 docs/DATABASE_SETUP.md")
}
