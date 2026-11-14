package wechatweb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jenvan/aiknow/pkg/wechatweb"
)

// fakeRunner 立即以指定错误退出，用于验证 Manager 的停止/回调逻辑。
type fakeRunner struct {
	err  error
	wait chan struct{}
}

func (f *fakeRunner) Run(ctx context.Context) error {
	if f.wait != nil {
		<-f.wait
	}
	return f.err
}

func TestManagerReportsSessionExpired(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)
	expired := make(chan error, 1)
	m.SetOnSessionExpired(func(err error) { expired <- err })
	m.newRunner = func() runnable { return &fakeRunner{err: wechatweb.ErrSessionExpired} }

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case err := <-expired:
		if !errors.Is(err, wechatweb.ErrSessionExpired) {
			t.Errorf("回调收到的不是会话失效错误: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("会话失效后未触发 OnSessionExpired 回调")
	}
	// 停止后 running 必须为 false（状态页据此显示"已停止"）
	m.waitStopped(t)
}

// 网络类错误不应该触发"重新扫码"提醒。
func TestManagerDoesNotAlertOnNetworkError(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)
	called := make(chan struct{}, 1)
	m.SetOnSessionExpired(func(error) { called <- struct{}{} })
	m.newRunner = func() runnable { return &fakeRunner{err: errors.New("dial tcp: i/o timeout")} }

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	m.waitStopped(t)
	select {
	case <-called:
		t.Error("网络错误不应触发会话失效告警")
	default:
	}
}

func (m *Manager) waitStopped(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !m.Running() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Manager 未在预期时间内停止")
}
