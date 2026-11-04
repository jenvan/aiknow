package weixin

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/ilink"
)

// Bot 把 iLink 客户端与 Router 粘起来：长轮询 → 鉴权/去重 → 路由 → 回包。
//
// v1 只处理单聊：群聊消息（group_id 非空）直接忽略——群里的机器人要先有
// @ 判定与更严的白名单，否则很容易在群里刷屏。
type Bot struct {
	repo   repository.WeixinBotRepository
	router *Router
	logger *slog.Logger

	client    *ilink.Client
	account   *model.WeixinBotAccount
	lastToken string

	// allowFromOverride 来自 L1 配置 WEIXIN_BOT_ALLOW_FROM：非空时覆盖库里
	// 的白名单，省去为改白名单重新扫码（凭据仍以库为准）。
	allowFromOverride string

	// pusher 可选：注入后采集入库的文章会在处理完成时回推微信
	pusher *Pusher

	mu   sync.Mutex
	seen map[int64]time.Time
}

// seenCap 去重表容量（消息量很小，500 条足够覆盖一次重启窗口）。
const seenCap = 500

// BotOption 定制 Bot。
type BotOption func(*Bot)

// WithAllowFrom 用配置覆盖账号里的白名单。
func WithAllowFrom(csv string) BotOption {
	return func(b *Bot) { b.allowFromOverride = csv }
}

// WithPusher 注入「处理完成回推」的实现。
//
// 传 nil 等于关闭回推：只有「发链接 → 收到回执」这一步，用户靠 /status 自查。
func WithPusher(p *Pusher) BotOption {
	return func(b *Bot) { b.pusher = p }
}

// NewBot 构造 Bot。
func NewBot(repo repository.WeixinBotRepository, router *Router, logger *slog.Logger, opts ...BotOption) *Bot {
	if logger == nil {
		logger = slog.Default()
	}
	b := &Bot{
		repo:   repo,
		router: router,
		logger: logger,
		seen:   make(map[int64]time.Time, seenCap),
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Run 启动长轮询，阻塞直到 ctx 取消或会话过期。
//
// 会话过期（-14）返回 ilink.ErrSessionExpired：token 已失效，只能重新扫码
// （`aiknow-server weixin login`），不能自动恢复。
func (b *Bot) Run(ctx context.Context) error {
	acc, err := b.repo.Get(ctx)
	if err != nil {
		return err
	}
	if acc == nil {
		return errors.New("weixin bot: 尚未扫码登录，先执行 `aiknow-server weixin login`")
	}

	if b.allowFromOverride != "" {
		acc.AllowFrom = b.allowFromOverride
	}
	b.account = acc
	b.lastToken = acc.LastContextToken
	b.client = ilink.NewClient(ilink.Token{
		BotToken: acc.BotToken,
		BotID:    acc.BotID,
		UserID:   acc.OwnerUserID,
		BaseURL:  acc.BaseURL,
	}, ilink.WithLogger(b.logger))

	if b.pusher != nil {
		b.pusher.Attach(b.client)
		b.router.SetOnQueued(b.pusher.Expect)
	}

	b.logger.InfoContext(ctx, "weixin bot started",
		slog.String("bot_id", acc.BotID),
		slog.String("owner", acc.OwnerUserID),
		slog.Bool("cursor_saved", acc.Cursor != ""),
	)

	return b.client.Run(ctx, acc.Cursor, b.handle, b.persistCursor)
}

func (b *Bot) handle(ctx context.Context, msg *ilink.Message) error {
	if !msg.IsUserMessage() {
		return nil
	}
	if msg.IsGroup() {
		b.logger.DebugContext(ctx, "weixin: ignore group message", slog.String("group_id", msg.GroupID))
		return nil
	}
	if !b.account.Allows(msg.FromUserID) {
		// 不回复：未授权用户的消息一律静默丢弃，避免把 bot 变成可探测的服务
		b.logger.WarnContext(ctx, "weixin: ignored message from unauthorized user",
			slog.String("from", msg.FromUserID))
		return nil
	}
	if b.duplicate(msg.MessageID) {
		b.logger.DebugContext(ctx, "weixin: duplicate message ignored", slog.Int64("message_id", msg.MessageID))
		return nil
	}

	b.lastToken = msg.ContextToken
	b.logger.InfoContext(ctx, "weixin: inbound message",
		slog.Int64("message_id", msg.MessageID),
		slog.String("from", msg.FromUserID),
		slog.Int("items", len(msg.Items)),
		slog.Int("text_runes", len([]rune(msg.Text()))))

	b.router.Handle(ctx, msg, func(text string) {
		b.send(ctx, msg.FromUserID, msg.ContextToken, text)
	})
	return nil
}

// send 按长度切分后逐条发送；单条失败只记日志，不影响后续分片。
func (b *Bot) send(ctx context.Context, toUserID, contextToken, text string) {
	parts := splitReply(text, maxReplyRunes)
	b.logger.InfoContext(ctx, "weixin: sending reply",
		slog.Int("parts", len(parts)), slog.Int("runes", len([]rune(text))))
	for _, part := range parts {
		if err := b.client.SendText(ctx, toUserID, contextToken, part); err != nil {
			b.logger.ErrorContext(ctx, "weixin: send reply failed", slog.Any("err", err))
			return
		}
		// 微信侧偶发丢弃紧邻的连续消息，留一点间隔
		time.Sleep(150 * time.Millisecond)
	}
}

// persistCursor 把消费位点写回库。
//
// 用独立 context：调用方的 ctx 可能已因关闭流程被取消，而游标写入恰恰
// 是「下次启动不重复消费」的关键，不能跟着一起取消（与队列回写状态同理）。
func (b *Bot) persistCursor(ctx context.Context, cursor string) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := b.repo.UpdateProgress(writeCtx, b.account.BotID, cursor, b.lastToken); err != nil {
		b.logger.ErrorContext(writeCtx, "weixin: persist cursor failed", slog.Any("err", err))
	}
}

// duplicate 按 message_id 去重。游标是 at-least-once 语义，重启后可能重放。
func (b *Bot) duplicate(id int64) bool {
	if id == 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.seen[id]; ok {
		return true
	}
	if len(b.seen) >= seenCap {
		// 直接清空：消息量低，简单胜过精确 LRU
		b.seen = make(map[int64]time.Time, seenCap)
	}
	b.seen[id] = time.Now()
	return false
}
