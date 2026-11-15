package weixin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/ilink"
)

// Notifier 把「其它通道入库」的结果回执发到微信。
//
// 背景：文件传输助手通道是**只收不发**的（发送是风控面最大的动作），
// 但用户需要知道"到底收没收到"。所以回执统一走 iLink Bot：
// 入库瞬间发一条，处理完成再由 Pusher 补一条（复用 Bot 的会话上下文）。
//
// 注意：这里用的是 bot 账号「最近一条 inbound 消息」的 context_token。
// 它可能已经过期（用户很久没跟 bot 说话），此时发送会失败——只记日志，
// 不回落到其它通道，避免把"没收到回执"变成更难排查的问题。
type Notifier struct {
	repo   repository.WeixinBotRepository
	pusher *Pusher
	logger *slog.Logger

	// pending 发送失败（通常是 context_token 过期）的告警，等下一次收到
	// 用户消息、token 刷新后再补发。上限 5 条，去重。
	mu      sync.Mutex
	pending []string
}

// NewNotifier 创建回执发送器。pusher 可为 nil（则不发"处理完成"）。
func NewNotifier(repo repository.WeixinBotRepository, pusher *Pusher, logger *slog.Logger) *Notifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &Notifier{repo: repo, pusher: pusher, logger: logger}
}

// Ingested 入库成功回执。
func (n *Notifier) Ingested(ctx context.Context, article *model.Article) {
	if article == nil {
		return
	}
	text := fmt.Sprintf("📥 已收录《%s》\n来源：文件传输助手转发\n处理中，完成后会再通知你", article.Title)
	n.send(ctx, text, article.ID)
}

// Duplicate 重复链接回执（不打扰：一条短句）。
func (n *Notifier) Duplicate(ctx context.Context, url string) {
	n.send(ctx, "ℹ️ 这篇已经收录过了："+shortURL(url), "")
}

// Alert 发送一条系统告警（如「文件传输助手会话失效」）。
//
// 同样走 bot 的 context_token；bot 会话本身失效时这里会失败，只记日志——
// 告警失败不能再触发告警，否则会绕成环。
func (n *Notifier) Alert(ctx context.Context, text string) {
	if n.send(ctx, text, "") {
		return
	}
	// 发不出去就先记着：iLink 只允许在"用户最近说过话"的会话里回复，
	// 等到下一次 inbound 消息刷新 context_token 后由 FlushPending 补发。
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, p := range n.pending {
		if p == text {
			return
		}
	}
	if len(n.pending) >= 5 {
		n.pending = n.pending[1:]
	}
	n.pending = append(n.pending, text)
}

// FlushPending 补发积压的告警（在收到用户消息、token 已刷新后调用）。
func (n *Notifier) FlushPending(ctx context.Context) {
	n.mu.Lock()
	items := append([]string(nil), n.pending...)
	n.mu.Unlock()
	if len(items) == 0 {
		return
	}

	var remain []string
	for _, text := range items {
		if !n.send(ctx, text, "") {
			remain = append(remain, text)
		}
	}
	n.mu.Lock()
	n.pending = remain
	n.mu.Unlock()
}

// send 发一条文本；articleID 非空时登记"处理完成"推送。
func (n *Notifier) send(ctx context.Context, text, articleID string) bool {
	acc, err := n.repo.Get(ctx)
	if err != nil || acc == nil {
		n.logger.Debug("weixin: 未登录 bot，跳过回执")
		return false
	}
	if acc.LastContextToken == "" {
		n.logger.Debug("weixin: 没有可用的 context_token，跳过回执")
		return false
	}

	client := ilink.NewClient(ilink.Token{
		BotToken: acc.BotToken,
		BotID:    acc.BotID,
		UserID:   acc.OwnerUserID,
		BaseURL:  acc.BaseURL,
	}, ilink.WithLogger(n.logger))

	if err := client.SendText(ctx, acc.OwnerUserID, acc.LastContextToken, text); err != nil {
		n.logger.Warn("weixin: 回执发送失败（context_token 可能已过期），稍后重试", "err", err)
		return false
	}
	if articleID != "" && n.pusher != nil {
		// 登记"处理完成"推送：worker 跑完后用同一上下文再发一条
		n.pusher.Expect(articleID, acc.OwnerUserID, acc.LastContextToken)
	}
	return true
}

func shortURL(u string) string {
	if len(u) > 60 {
		return u[:60] + "…"
	}
	return strings.TrimSpace(u)
}
