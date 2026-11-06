package ilink

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// 断网期间不能逐条刷 ERROR：首条立即打，之后 2 分钟内静默，并累计次数。
func TestGetUpdatesErrorLogThrottled(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(Token{BotToken: "t"}, WithLogger(slog.New(slog.NewTextHandler(&buf, nil))))

	for i := 0; i < 5; i++ {
		c.logGetUpdatesError(context.Background(), errors.New("dial tcp: i/o timeout"))
	}
	if n := strings.Count(buf.String(), "getupdates failed"); n != 1 {
		t.Fatalf("2 分钟内应只输出 1 条日志，实际 %d 条:\n%s", n, buf.String())
	}

	// 成功后复位，再次失败应重新输出
	c.resetErrLog()
	c.logGetUpdatesError(context.Background(), errors.New("again"))
	if n := strings.Count(buf.String(), "getupdates failed"); n != 2 {
		t.Fatalf("复位后应再输出一条，实际 %d 条", n)
	}
}
