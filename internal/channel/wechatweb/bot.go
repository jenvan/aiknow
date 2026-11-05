// Package wechatweb 把「文件传输助手网页版协议」接到 AiKnow：
// 收下手机转发来的公众号文章链接并入库。
//
// **只收不发**：不实现任何发送接口。原因有二——
//  1. 业务上不需要（入库只需要 url）；
//  2. 协议里 ToUserName 是联系人 ID，一旦可写就等于能以账号身份给任意好友发消息，
//     权限面太大，不做就不会出错。
package wechatweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/securebox"
	"github.com/jenvan/aiknow/pkg/wechatweb"
)

// Notifier 入库结果回执（实现在 channel/weixin，走 iLink Bot）。
//
// 文件传输助手通道本身只收不发，用户无从得知结果，所以回执统一交给 Bot。
type Notifier interface {
	Ingested(ctx context.Context, article *model.Article)
	Duplicate(ctx context.Context, url string)
}

// ArticleCreator 从 URL 入库（由 service.ArticleService 实现）。
type ArticleCreator interface {
	CreateFromURL(ctx context.Context, url string) (*model.Article, error)
}

// Runner 文件传输助手接收器。
type Runner struct {
	repo    repository.WechatWebRepository
	box     *securebox.Box
	creator ArticleCreator
	logger  *slog.Logger

	// notifier 可选：入库/重复时回执到微信（为 nil 时只记日志）
	notifier Notifier

	// lastPoll 最近一次 synccheck 成功的时间（UnixNano）。
	// last_sync_at 只在有新消息时更新，空闲时看起来像"卡住了"，故单独记心跳。
	lastPoll atomic.Int64

	mu    sync.Mutex
	seen  map[string]time.Time
	onKey func(wechatweb.SyncKey)
}

// seenCap 消息去重表容量（MsgId 级，防同一批消息重复投递）。
const seenCap = 500

// NewRunner 创建接收器。
func NewRunner(repo repository.WechatWebRepository, box *securebox.Box, creator ArticleCreator, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{repo: repo, box: box, creator: creator, logger: logger, seen: make(map[string]time.Time, seenCap)}
}

// SetNotifier 注入回执发送器。
func (r *Runner) SetNotifier(n Notifier) { r.notifier = n }

// Run 启动长轮询，阻塞到 ctx 取消或会话失效。
//
// 会话失效（ret 1100/1101/1102）直接返回 ErrSessionExpired，**不做自动重试**：
// 反复重登是这个协议里最容易触发风控的动作，必须人工扫码。
func (r *Runner) Run(ctx context.Context) error {
	if r.box == nil {
		return errors.New("wechatweb: 未配置 SESSION_SECRET，无法解密会话凭据")
	}
	sess, err := r.repo.Get(ctx)
	if err != nil {
		return err
	}
	if sess == nil {
		return errors.New("wechatweb: 尚未登录，先执行 `aiknow-server wechatweb login`")
	}

	tok, key, err := r.decode(sess)
	if err != nil {
		return fmt.Errorf("wechatweb: 会话凭据解密失败（SESSION_SECRET 变了？）: %w", err)
	}

	client := wechatweb.NewClient(tok,
		wechatweb.WithLogger(r.logger),
		wechatweb.WithSyncKeySink(func(k wechatweb.SyncKey) { r.persistKey(k) }),
	)
	r.onKey = func(k wechatweb.SyncKey) { r.persistKey(k) }

	r.logger.Info("wechatweb: 开始接收（只收不发）", "user", sess.UserName, "skey_items", len(key.List))

	// 每次长轮询成功都刷新心跳，便于后台判断"到底还活着吗"
	client.SetPollObserver(func(time.Time) { r.lastPoll.Store(time.Now().UnixNano()) })

	err = client.Run(ctx, key, r.handle)
	switch {
	case errors.Is(err, wechatweb.ErrSessionExpired):
		r.logger.Error("wechatweb: 会话已失效，需要重新扫码（不会自动重试）")
		return err
	case errors.Is(err, context.Canceled):
		return nil
	}
	return err
}

// LastPollAt 最近一次 synccheck 成功的时刻；从未成功则为零值。
func (r *Runner) LastPollAt() time.Time {
	ns := r.lastPoll.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// decode 解密凭据与游标。
func (r *Runner) decode(sess *model.WechatWebSession) (wechatweb.Token, wechatweb.SyncKey, error) {
	var tok wechatweb.Token
	plain, err := r.box.Decrypt(sess.CredsEnc)
	if err != nil {
		return tok, wechatweb.SyncKey{}, err
	}
	if err := json.Unmarshal(plain, &tok); err != nil {
		return tok, wechatweb.SyncKey{}, err
	}
	var key wechatweb.SyncKey
	if len(sess.SyncKey) > 0 {
		if err := json.Unmarshal(sess.SyncKey, &key); err != nil {
			return tok, key, fmt.Errorf("解析 sync_key: %w", err)
		}
	}
	return tok, key, nil
}

// persistKey 游标落库。用独立 ctx：调用方 ctx 可能已随关闭被取消，
// 而游标恰恰是「下次启动接着收」的关键，不能跟着一起丢。
func (r *Runner) persistKey(k wechatweb.SyncKey) {
	b, err := json.Marshal(k)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.repo.UpdateSyncKey(ctx, b); err != nil {
		r.logger.Error("wechatweb: 保存同步游标失败", "err", err)
	}
}

// handle 处理一条入站消息。
func (r *Runner) handle(ctx context.Context, m *wechatweb.Message) {
	if m == nil || m.MsgID == "" {
		return
	}
	if r.duplicate(m.MsgID) {
		return
	}

	rawURL := r.extractURL(m)
	if rawURL == "" {
		r.logger.Debug("wechatweb: 忽略非文章消息", "msg_type", m.MsgType, "app_msg", m.AppMsg)
		return
	}

	article, err := r.creator.CreateFromURL(ctx, rawURL)
	if err != nil {
		if errors.Is(err, ErrDuplicate) {
			r.logger.Info("wechatweb: 文章已存在，跳过", "url", shorten(rawURL))
			if r.notifier != nil {
				r.notifier.Duplicate(ctx, rawURL)
			}
			return
		}
		r.logger.Error("wechatweb: 入库失败", "url", shorten(rawURL), "err", err)
		return
	}
	r.logger.Info("wechatweb: 已入库", "title", article.Title, "status", article.ProcessingStatus)
	if r.notifier != nil {
		r.notifier.Ingested(ctx, article)
	}
}

// extractURL 取出可入库的文章链接。
//
// 只认公众号文章：转发卡片是 MsgType=49/AppMsgType=5，url 在 XML 里；
// 用户直接粘贴短链时是 MsgType=1 的纯文本。
func (r *Runner) extractURL(m *wechatweb.Message) string {
	candidate := ""
	switch {
	case m.MsgType == 49 && m.URL != "":
		candidate = m.URL
	case m.MsgType == 1:
		for _, f := range strings.Fields(m.Text) {
			if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
				candidate = f
				break
			}
		}
	}
	if candidate == "" {
		return ""
	}
	u, err := url.Parse(candidate)
	if err != nil {
		return ""
	}
	if u.Host != "mp.weixin.qq.com" {
		return ""
	}
	return candidate
}

// duplicate 按 MsgId 去重（内存表，进程重启后靠 URL 去重兜底）。
func (r *Runner) duplicate(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.seen[id]; ok {
		return true
	}
	if len(r.seen) >= seenCap {
		r.seen = make(map[string]time.Time, seenCap)
	}
	r.seen[id] = time.Now()
	return false
}

// ErrDuplicate 文章已存在（由适配层把 service.ErrDuplicateArticle 翻译过来）。
var ErrDuplicate = errors.New("duplicate article")

func shorten(u string) string {
	if len(u) > 80 {
		return u[:80] + "…"
	}
	return u
}
