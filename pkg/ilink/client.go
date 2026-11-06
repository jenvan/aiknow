package ilink

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	// DefaultBaseURL iLink 服务端默认域名。
	DefaultBaseURL = "https://ilinkai.weixin.qq.com"

	// channelVersion 上报给服务端的渠道版本号，与官方插件 2.x 对齐。
	channelVersion = "2.0.0"

	// errcodeSessionExpired 会话过期，需要重新扫码登录。
	errcodeSessionExpired = -14
)

// ErrSessionExpired 表示 bot 会话已过期，必须重新扫码登录（不可自动恢复）。
var ErrSessionExpired = errors.New("ilink: session expired, re-login required")

// APIError 是服务端返回的非零 ret / errcode。
type APIError struct {
	Op      string
	Ret     int
	ErrCode int
	ErrMsg  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ilink: %s failed (ret=%d errcode=%d msg=%s)", e.Op, e.Ret, e.ErrCode, e.ErrMsg)
}

// Client 是 iLink Bot 客户端，可并发使用。
type Client struct {
	token Token
	base  string
	// 断网时的日志节流：网络中断期间每次轮询都失败，逐条打 ERROR 会刷屏
	// （实测一小时 145 行）。改为"首条立即打，之后每 2 分钟一条，并带累计次数"。
	errLogMu sync.Mutex
	errLogAt time.Time
	errLogN  int

	// cdnBase 媒体 CDN 基座，测试可替换
	cdnBase string
	http    *http.Client
	logger  *slog.Logger
}

// logGetUpdatesError 对连续失败做日志节流。
func (c *Client) logGetUpdatesError(ctx context.Context, err error) {
	c.errLogMu.Lock()
	c.errLogN++ // 累计连续失败次数（不能在这里清零，否则每次调用都像"首条"）
	n := c.errLogN
	now := time.Now()
	// 首条、或距上次输出超过 2 分钟才写日志
	if c.errLogAt.IsZero() || now.Sub(c.errLogAt) >= 2*time.Minute {
		c.errLogAt = now
		c.errLogMu.Unlock()
		c.logger.ErrorContext(ctx, "ilink: getupdates failed（网络或服务端不可达，将持续重试）",
			slog.Any("err", err), slog.Int("consecutive", n))
		return
	}
	c.errLogMu.Unlock()
}

// resetErrLog 成功后复位节流状态。
func (c *Client) resetErrLog() {
	c.errLogMu.Lock()
	c.errLogN = 0
	c.errLogAt = time.Time{}
	c.errLogMu.Unlock()
}

// Option 用于定制 Client。
type Option func(*Client)

// WithHTTPClient 替换底层 HTTP 客户端（测试注入 mock、或走代理）。
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.http = hc } }

// WithLogger 设置日志器。
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.logger = l } }

// WithBaseURL 覆盖服务端域名（测试注入 mock 服务时使用）。
func WithBaseURL(u string) Option {
	return func(c *Client) {
		if u != "" {
			c.base = u
		}
	}
}

// NewClient 用已持久化的 Token 构造客户端。
func NewClient(tok Token, opts ...Option) *Client {
	base := DefaultBaseURL
	if tok.BaseURL != "" {
		base = tok.BaseURL
	}
	c := &Client{
		token:   tok,
		base:    base,
		cdnBase: cdnBaseURL,
		// 长轮询服务端 hold 35s，超时必须大于它，否则每轮都误判超时
		http:   &http.Client{Timeout: 50 * time.Second},
		logger: slog.Default(),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// randomUIN 生成 X-WECHAT-UIN：随机 uint32 的十进制字符串再 base64。
// 每次请求都要变（服务端用它防重放）。
func randomUIN() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		binary.BigEndian.PutUint32(b[:], uint32(time.Now().UnixNano()))
	}
	n := binary.BigEndian.Uint32(b[:])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprint(n)))
}

func (c *Client) newRequest(ctx context.Context, method, path string, payload any) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal %s payload: %w", path, err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("X-WECHAT-UIN", randomUIN())
	if c.token.BotToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.token.BotToken)
	}
	return req, nil
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return body, fmt.Errorf("ilink: %s %s", req.URL.Path, resp.Status)
	}
	return body, nil
}

// ---- wire 类型：字段名严格对齐服务端，不要改 ----

type baseInfo struct {
	ChannelVersion string `json:"channel_version"`
}

type wireRefMsg struct {
	Title       string `json:"title,omitempty"`
	MessageItem *struct {
		Type     int `json:"type"`
		TextItem *struct {
			Text string `json:"text"`
		} `json:"text_item,omitempty"`
	} `json:"message_item,omitempty"`
}

type wireItem struct {
	Type     int `json:"type"`
	TextItem *struct {
		Text string `json:"text"`
	} `json:"text_item,omitempty"`
	RefMsg *wireRefMsg `json:"ref_msg,omitempty"`
}

type wireMessage struct {
	MessageID    int64      `json:"message_id"`
	FromUserID   string     `json:"from_user_id"`
	ToUserID     string     `json:"to_user_id"`
	GroupID      string     `json:"group_id"`
	MessageType  int        `json:"message_type"`
	ContextToken string     `json:"context_token"`
	CreateTimeMS int64      `json:"create_time_ms"`
	ItemList     []wireItem `json:"item_list"`
}

// Updates 是一次长轮询的结果。
type Updates struct {
	Messages []*Message
	Cursor   string
}

// GetUpdates 拉取一批消息。服务端会 hold 连接最多 35s，无消息时返回空列表，
// 仍会带上新的游标——游标必须原样保存并在下次请求带回，否则重复收消息。
func (c *Client) GetUpdates(ctx context.Context, cursor string) (*Updates, error) {
	payload := map[string]any{
		"get_updates_buf": cursor,
		"base_info":       baseInfo{ChannelVersion: channelVersion},
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/ilink/bot/getupdates", payload)
	if err != nil {
		return nil, err
	}
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Ret     int           `json:"ret"`
		Msgs    []wireMessage `json:"msgs"`
		Buf     string        `json:"get_updates_buf"`
		ErrCode *int          `json:"errcode"`
		ErrMsg  string        `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse getupdates: %w (body=%s)", err, truncate(string(body), 200))
	}
	if resp.ErrCode != nil && *resp.ErrCode == errcodeSessionExpired {
		return nil, ErrSessionExpired
	}
	if resp.Ret != 0 || (resp.ErrCode != nil && *resp.ErrCode != 0) {
		code := 0
		if resp.ErrCode != nil {
			code = *resp.ErrCode
		}
		return nil, &APIError{Op: "getupdates", Ret: resp.Ret, ErrCode: code, ErrMsg: resp.ErrMsg}
	}

	out := &Updates{Cursor: resp.Buf}
	for i := range resp.Msgs {
		out.Messages = append(out.Messages, convertMessage(&resp.Msgs[i]))
	}
	return out, nil
}

func convertMessage(w *wireMessage) *Message {
	m := &Message{
		MessageID:    w.MessageID,
		FromUserID:   w.FromUserID,
		ToUserID:     w.ToUserID,
		GroupID:      w.GroupID,
		MessageType:  MessageType(w.MessageType),
		ContextToken: w.ContextToken,
		CreateTimeMs: w.CreateTimeMS,
		Items:        make([]Item, 0, len(w.ItemList)),
	}
	for _, it := range w.ItemList {
		item := Item{Type: ItemType(it.Type), Raw: rawOf(it)}
		if it.TextItem != nil {
			item.Text = it.TextItem.Text
		}
		if it.RefMsg != nil {
			ref := &RefMessage{Title: it.RefMsg.Title}
			if it.RefMsg.MessageItem != nil && it.RefMsg.MessageItem.TextItem != nil {
				ref.Text = it.RefMsg.MessageItem.TextItem.Text
			}
			item.RefMsg = ref
		}
		m.Items = append(m.Items, item)
	}
	return m
}

func rawOf(it wireItem) json.RawMessage {
	b, err := json.Marshal(it)
	if err != nil {
		return nil
	}
	return b
}

// SendText 回复一条文本消息。
//
// contextToken **必须**是该用户最近一条 inbound 消息里的 context_token，
// 否则消息不会落到对应会话窗口。
//
// 微信对单条文本长度有限制，超长需要由调用方切分（见 channel 层的 splitText）。
func (c *Client) SendText(ctx context.Context, toUserID, contextToken, text string) error {
	payload := map[string]any{
		"msg": map[string]any{
			"to_user_id":    toUserID,
			"client_id":     newClientID(),
			"message_type":  int(MessageTypeBot),
			"message_state": 2, // FINISH
			"context_token": contextToken,
			"item_list": []map[string]any{
				{"type": int(ItemTypeText), "text_item": map[string]string{"text": text}},
			},
		},
		"base_info": baseInfo{ChannelVersion: channelVersion},
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/ilink/bot/sendmessage", payload)
	if err != nil {
		return err
	}
	body, err := c.do(req)
	if err != nil {
		return err
	}
	var resp struct {
		Ret       int    `json:"ret"`
		MessageID int64  `json:"message_id"`
		ErrCode   *int   `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse sendmessage: %w (body=%s)", err, truncate(string(body), 200))
	}
	// 成功时记录服务端分配的 message_id：微信对「发出去了但没投递」不会报错，
	// 有 id 才能和用户端「到底看没看到」对得上账。
	if resp.MessageID != 0 {
		c.logger.InfoContext(ctx, "ilink: reply accepted",
			slog.Int64("message_id", resp.MessageID), slog.String("to", toUserID))
	}
	if resp.ErrCode != nil && *resp.ErrCode == errcodeSessionExpired {
		return ErrSessionExpired
	}
	if resp.Ret != 0 || (resp.ErrCode != nil && *resp.ErrCode != 0) {
		code := 0
		if resp.ErrCode != nil {
			code = *resp.ErrCode
		}
		return &APIError{Op: "sendmessage", Ret: resp.Ret, ErrCode: code, ErrMsg: resp.ErrMsg}
	}
	return nil
}

// Handler 处理一条入站消息。返回 error 只记日志，不中断轮询。
type Handler func(ctx context.Context, msg *Message) error

// CursorSink 在游标推进后回调，用于持久化（重启后接着拉取，不重复消费）。
type CursorSink func(ctx context.Context, cursor string)

// Run 启动长轮询循环，阻塞直到 ctx 取消（返回 ctx.Err()）或会话过期
// （返回 ErrSessionExpired）。
//
// 语义与取舍：
//   - **先处理消息、再落游标**（at-least-once）。顺序反过来在进程崩溃时会
//     直接丢消息；重复投递由上层按 message_id 去重。
//   - 单次轮询失败不退出，退避 2s 后重试；只有会话过期（-14）才向上抛。
//   - handler panic 被捕获并记日志，单条消息的 bug 不能拖死整个 bot。
func (c *Client) Run(ctx context.Context, cursor string, h Handler, sink CursorSink) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		updates, err := c.GetUpdates(ctx, cursor)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, ErrSessionExpired) {
				return ErrSessionExpired
			}
			c.logGetUpdatesError(ctx, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		c.resetErrLog()

		for _, msg := range updates.Messages {
			c.handleOne(ctx, h, msg)
		}

		if updates.Cursor != "" && updates.Cursor != cursor {
			cursor = updates.Cursor
			if sink != nil {
				sink(ctx, cursor)
			}
		}
	}
}

func (c *Client) handleOne(ctx context.Context, h Handler, msg *Message) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.ErrorContext(ctx, "ilink: handler panic",
				slog.Any("panic", r), slog.Int64("message_id", msg.MessageID))
		}
	}()
	if err := h(ctx, msg); err != nil {
		c.logger.ErrorContext(ctx, "ilink: handler error",
			slog.Any("err", err), slog.Int64("message_id", msg.MessageID))
	}
}

// newClientID 生成发送侧的唯一 id（32 位 hex）。每条消息都应不同，
// 服务端用它做幂等。
func newClientID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		binary.BigEndian.PutUint64(b[:], uint64(time.Now().UnixNano()))
	}
	return hex.EncodeToString(b[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
