package weixin

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jenvan/aiknow/internal/model"
)

// textSender 是本层需要的发送能力（*ilink.Client 实现它）。
//
// 抽成接口是为了让推送逻辑可测：真实客户端发消息要打网络，测试里给它一个
// 记录调用的假实现即可。
type textSender interface {
	SendText(ctx context.Context, toUserID, contextToken, text string) error
}

// pendingPush 是一条等待「处理完成」的推送任务。
type pendingPush struct {
	userID string
	token  string
	at     time.Time
}

// Pusher 把「文章 AI 处理完成」主动推回微信。
//
// 只推送**由微信入口发起**的采集：后台/API 录入的文章没有对话上下文，
// 硬推给微信用户只会造成骚扰。所以这里用「登记 + 兑现」：
//   - 采集入库成功时 Expect() 登记 articleID → 对话上下文；
//   - worker 处理完成时 Deliver() 兑现，未登记的 articleID 直接忽略。
type Pusher struct {
	articles ArticleStore
	sender   textSender
	logger   *slog.Logger

	mu      sync.Mutex
	pending map[string]pendingPush

	// ttl 超过这个时间的登记视为过期（用户早就不要这条推送了）
	ttl time.Duration
}

// maxPending 登记表上限，防止异常情况下无限增长。
const maxPending = 200

// NewPusher 构造推送器。
//
// sender 在 Bot.Run 里通过 Attach 注入：客户端要等账号从库里读出来才能创建，
// 而 Pusher 需要在装配期就交给 worker 回调，故拆成两步。
func NewPusher(articles ArticleStore, logger *slog.Logger) *Pusher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pusher{
		articles: articles,
		logger:   logger,
		pending:  make(map[string]pendingPush, 16),
		ttl:      30 * time.Minute,
	}
}

// Attach 注入发送端（Bot.Run 启动轮询前调用，此后只读，无并发问题）。
func (p *Pusher) Attach(sender textSender) { p.sender = sender }

// Expect 登记一条待推送的文章（采集入库成功后调用）。
func (p *Pusher) Expect(articleID, userID, token string) {
	if articleID == "" || userID == "" || token == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.pending) >= maxPending {
		// 满了直接清空：宁可漏推，也不要在内存里堆着
		p.pending = make(map[string]pendingPush, 16)
	}
	p.pending[articleID] = pendingPush{userID: userID, token: token, at: time.Now()}
}

// Deliver 兑现推送。worker 处理完成后调用，未登记的文章直接返回。
//
// 推送失败只记日志：微信下行本身可能静默丢弃（见 docs/WEIXIN_BOT.md），
// 但文章已经入库并处理完成，不值得为此重试整条流水线。
func (p *Pusher) Deliver(ctx context.Context, articleID string) {
	p.mu.Lock()
	item, ok := p.pending[articleID]
	if ok {
		delete(p.pending, articleID)
	}
	p.mu.Unlock()
	if !ok {
		return
	}
	if time.Since(item.at) > p.ttl {
		p.logger.WarnContext(ctx, "weixin: drop stale push", slog.String("article_id", articleID))
		return
	}

	if p.sender == nil {
		p.logger.WarnContext(ctx, "weixin: pusher has no sender, skip push")
		return
	}

	article, err := p.articles.GetByID(ctx, articleID)
	if err != nil || article == nil {
		p.logger.WarnContext(ctx, "weixin: load article for push failed",
			slog.String("article_id", articleID), slog.Any("err", err))
		return
	}

	text := formatProcessed(article)
	if err := p.sender.SendText(ctx, item.userID, item.token, text); err != nil {
		p.logger.ErrorContext(ctx, "weixin: push processed article failed",
			slog.String("article_id", articleID), slog.Any("err", err))
		return
	}
	p.logger.InfoContext(ctx, "weixin: pushed processed article",
		slog.String("article_id", articleID), slog.String("to", item.userID))
}

// formatProcessed 组装「处理完成」回包：摘要 + 分类标签 + 原文链接。
func formatProcessed(a *model.Article) string {
	var b strings.Builder
	b.WriteString("✅ 处理完成《")
	b.WriteString(a.Title)
	b.WriteString("》\n")

	if a.Summary != nil && strings.TrimSpace(*a.Summary) != "" {
		b.WriteString("\n摘要：")
		b.WriteString(snippet(*a.Summary, 200))
		b.WriteString("\n")
	}

	var meta []string
	if a.CategoryName != "" {
		meta = append(meta, "分类 "+a.CategoryName)
	}
	if len(a.Tags) > 0 {
		names := make([]string, 0, len(a.Tags))
		for _, t := range a.Tags {
			names = append(names, t.Name)
		}
		meta = append(meta, "标签 "+strings.Join(names, "/"))
	}
	if len(meta) > 0 {
		b.WriteString("\n" + strings.Join(meta, " · ") + "\n")
	}

	if a.SourceURL != nil && *a.SourceURL != "" {
		b.WriteString("\n原文：" + *a.SourceURL)
	}
	return strings.TrimRight(b.String(), "\n")
}
