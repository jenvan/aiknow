package wechatweb

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/securebox"
	"github.com/jenvan/aiknow/pkg/wechatweb"
)

// runnable 抽象轮询器（*Runner 即实现），便于测试注入"立即失效"的假实现。
type runnable interface {
	Run(ctx context.Context) error
}

// Manager 管理接收器的生命周期。
//
// 为什么需要它：通道原本只在 serve 启动时创建，后台扫码登录成功后还得重启进程才生效，
// 后台页面等于白做。用一个可 Start/Stop 的句柄，扫码成功即可当场拉起接收器。
type Manager struct {
	repo    repository.WechatWebRepository
	box     *securebox.Box
	creator ArticleCreator
	logger  *slog.Logger

	notifier Notifier

	// onExpired 在"会话失效导致接收器停止"时回调一次（用于提醒用户重新扫码）。
	// 只在 ErrSessionExpired 时触发；网络错误导致的停止不打扰。
	onExpired func(error)

	// newRunner 构造轮询器；测试可替换成返回 ErrSessionExpired 的假实现。
	newRunner func() runnable

	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
	lastErr error
	runner  *Runner
}

// NewManager 创建管理器。
func NewManager(repo repository.WechatWebRepository, box *securebox.Box, creator ArticleCreator, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{repo: repo, box: box, creator: creator, logger: logger}
	m.newRunner = func() runnable {
		r := NewRunner(repo, box, creator, logger)
		r.SetNotifier(m.notifier)
		return r
	}
	return m
}

// SetOnSessionExpired 注入"会话失效"回调。
func (m *Manager) SetOnSessionExpired(fn func(error)) { m.onExpired = fn }

// SetNotifier 注入回执发送器（入库/重复时通知到微信 Bot）。
func (m *Manager) SetNotifier(n Notifier) { m.notifier = n }

// Start 启动接收器（已在运行则忽略）。
//
// parent 只用于派生取消信号；这里刻意不继承 parent 的取消——由 Manager.Stop 控制。
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.lastErr = nil
	m.mu.Unlock()

	runner := m.newRunner()
	if real, ok := runner.(*Runner); ok {
		m.mu.Lock()
		m.runner = real
		m.mu.Unlock()
	}

	go func() {
		err := runner.Run(runCtx)
		m.mu.Lock()
		m.running = false
		m.cancel = nil
		m.lastErr = err
		m.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			m.logger.Error("wechatweb: 接收器已停止", "err", err)
			// 会话失效必须主动告诉用户，否则只能靠自己发现"怎么不收消息了"
			if errors.Is(err, wechatweb.ErrSessionExpired) && m.onExpired != nil {
				m.onExpired(err)
			}
		}
	}()
	return nil
}

// Stop 停止接收器。
func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.running = false
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Running 是否在运行。
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// LastPollAt 最近一次 synccheck 成功的时刻（未运行为零值）。
func (m *Manager) LastPollAt() time.Time {
	m.mu.Lock()
	r := m.runner
	m.mu.Unlock()
	if r == nil {
		return time.Time{}
	}
	return r.LastPollAt()
}

// LastError 最近一次停止的原因（正常停止为 nil）。
func (m *Manager) LastError() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastErr == nil {
		return ""
	}
	return m.lastErr.Error()
}
