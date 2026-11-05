// Package wechatweb 实现「微信文件传输助手网页版」协议的最小客户端。
//
// 用途：从手机微信把公众号文章转发给「文件传输助手」，服务端收下链接入库。
// 之所以不用官方 iLink Bot，是因为 ClawBot 收不到「转发」的卡片；文件传输助手
// 是普通联系人，转发是微信原生能力。
//
// **只实现收，不实现发**：发送（webwxsendmsg / webwxsendmsgimg / webwxuploadmedia）
// 既没有业务价值，又是风控面最大、权限面最危险的部分（ToUserName 是联系人 ID，
// 一旦可写，就等于能以账号身份给任意好友发消息）。这里刻意不写。
//
// 协议细节全部来自真机抓包（2026-11），与网上流传的 wx.qq.com 版文档有多处差异，
// 关键差异见各方法的注释，踩坑点集中在：
//  1. appid 是 wx_webfilehelper，业务域名是 szfilehelper.weixin.qq.com；
//  2. 每个请求必须带 header mmweb_appid: wx_webfilehelper，漏了返回 ret=1102 且无错误信息；
//  3. 消息 Content 是「两层 HTML 转义 + 真实 <br/> 折行」，必须先剥 br 再解两遍实体。
package wechatweb

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// appID 网页版文件传输助手的 appid（非通用网页版微信的 wx782c26e4c19acffb）
	appID = "wx_webfilehelper"
	// apiHost 登录后的业务域名。登录前是 filehelper.weixin.qq.com，
	// 登录后页面会切到 szfilehelper，接口全部在这个域上。
	apiHost = "https://szfilehelper.weixin.qq.com"
	// redirectURIPath 扫码确认后回调的凭证下发地址
	redirectURIPath = "/cgi-bin/mmwebwx-bin/webwxnewloginpage"

	// appHeader 是绝对不能省的请求头，缺了 API 返回 ret=1102 且不带任何错误信息
	appHeader = "mmweb_appid"

	// pollHold 一次 synccheck 的挂起时长上限（服务端 hold ~25s）
	defaultPollTimeout = 35 * time.Second
)

// ErrSessionExpired 会话失效，必须重新扫码。
var ErrSessionExpired = errors.New("wechatweb: 会话已失效，需要重新扫码登录")

// Token 是登录后拿到的全部凭据。
//
// 这些合起来等价于「账号的网页登录态」，必须加密存储、不进日志。
type Token struct {
	Uin        string            `json:"uin"`
	Sid        string            `json:"sid"`
	SKey       string            `json:"skey"`
	PassTicket string            `json:"pass_ticket"`
	UserName   string            `json:"user_name"` // 自己的 UserName（发送时才需要，这里仅留档）
	Cookies    map[string]string `json:"cookies"`   // wxuin/wxsid/webwx_data_ticket/... 需要回传的 cookie
}

// SyncKey 同步游标。SyncCheckKey 与它内容通常一致，但两者都保存以防将来分化。
type SyncKey struct {
	Count int   `json:"Count"`
	List  []Key `json:"List"`
}

// Key SyncKey 的一项。
type Key struct {
	Key int `json:"Key"`
	Val int `json:"Val"`
}

// String 拼成 synccheck 需要的 `Key_Val|Key_Val` 形式。
func (s SyncKey) String() string {
	parts := make([]string, 0, len(s.List))
	for _, k := range s.List {
		parts = append(parts, fmt.Sprintf("%d_%d", k.Key, k.Val))
	}
	return strings.Join(parts, "|")
}

// Message 是一条入站消息（只保留本项目关心的字段）。
type Message struct {
	MsgID    string
	MsgType  int
	AppMsg   int
	FromUser string
	ToUser   string
	Title    string
	URL      string
	Source   string // 公众号名（sourcedisplayname）
	Text     string // MsgType=1 的纯文本
}

// Client 只读的文件传输助手客户端。
type Client struct {
	token  Token
	http   *http.Client
	logger *slog.Logger
	// apiBase 便于测试注入 mock（默认真实域名）
	apiBase string
	// onKey 游标推进后回调，供上层持久化（丢了就要重新登录才能收新消息）
	onKey func(SyncKey)
}

// Option 定制 Client。
type Option func(*Client)

// WithHTTPClient 替换 HTTP 客户端。
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.http = hc } }

// WithLogger 设置日志器。
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.logger = l } }

// WithSyncKeySink 注册游标变更回调。
func WithSyncKeySink(fn func(SyncKey)) Option { return func(c *Client) { c.onKey = fn } }

// WithAPIBase 覆盖业务域名（测试用）。
func WithAPIBase(base string) Option {
	return func(c *Client) { c.apiBase = strings.TrimRight(base, "/") }
}

// NewClient 用已保存的凭据构造客户端。
func NewClient(tok Token, opts ...Option) *Client {
	c := &Client{
		token:  tok,
		http:   &http.Client{Timeout: defaultPollTimeout},
		logger: slog.Default(),
		// 长轮询与普通请求共用一个 client；synccheck 的 ctx 超时单独控制
		apiBase: apiHost,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// newRequest 构造带会话 cookie 与 App 头的请求。
func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	u := c.apiBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set(appHeader, appID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	}
	for k, v := range c.token.Cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	return req, nil
}

// baseRequest 是所有业务接口共用的鉴权体。
//
// 注意 Uin 的类型历史上有坑：webwxinit 用**字符串**，webwxsendmsg 用**数字**；
// 这里统一按 init 的字符串形式（实测可用）。
func (c *Client) baseRequest() map[string]any {
	return map[string]any{
		"Uin":      c.token.Uin,
		"Sid":      c.token.Sid,
		"SKey":     c.token.SKey,
		"DeviceID": deviceID(),
	}
}

// deviceID 生成 15 位纯数字的设备 ID（网页版实测无 "e" 前缀）。
func deviceID() string {
	n, err := rand.Int(rand.Reader, big.NewInt(900000000000000))
	if err != nil {
		return "123456789012345"
	}
	return fmt.Sprintf("%015d", n.Int64()+100000000000000)
}

// InitResult webwxinit 的结果。
type InitResult struct {
	UserName     string
	NickName     string
	SyncKey      SyncKey
	SyncCheckKey SyncKey
	Ret          int
}

// Init 拉取初始 SyncKey 与自己的 UserName。
//
// 这是每次登录后的必做步骤，且**只在登录后做一次**：页面刷新会重新 Init 并重置
// SyncKey，历史消息就补不回来了。服务端要长期挂着，绝不反复 Init。
func (c *Client) Init(ctx context.Context) (*InitResult, error) {
	q := url.Values{}
	q.Set("r", fmt.Sprint(time.Now().UnixMilli()))
	q.Set("lang", "zh_CN")
	q.Set("pass_ticket", c.token.PassTicket)

	req, err := c.newRequest(ctx, http.MethodPost, "/cgi-bin/mmwebwx-bin/webwxinit", q,
		map[string]any{"BaseRequest": c.baseRequest()})
	if err != nil {
		return nil, err
	}
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		BaseResponse struct {
			Ret    int    `json:"Ret"`
			ErrMsg string `json:"ErrMsg"`
		} `json:"BaseResponse"`
		User struct {
			UserName string `json:"UserName"`
			NickName string `json:"NickName"`
		} `json:"User"`
		SyncKey      SyncKey `json:"SyncKey"`
		SyncCheckKey SyncKey `json:"SyncCheckKey"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析 webwxinit: %w", err)
	}
	if resp.BaseResponse.Ret != 0 {
		return nil, c.retError("webwxinit", resp.BaseResponse.Ret, resp.BaseResponse.ErrMsg)
	}
	return &InitResult{
		UserName:     resp.User.UserName,
		NickName:     resp.User.NickName,
		SyncKey:      resp.SyncKey,
		SyncCheckKey: resp.SyncCheckKey,
		Ret:          resp.BaseResponse.Ret,
	}, nil
}

// SyncCheckResult 一次长轮询的结果。
type SyncCheckResult struct {
	RetCode  int
	Selector int
}

// SyncCheck 长轮询（服务端 hold 约 25 秒才返回 selector=0）。
//
// 注意：网页版是用 <script> 标签（JSONP）发起这个请求的，返回的是
// `window.synccheck={retcode:"0",selector:"2"}` 这样的 JS 文本。
// 服务端直接 GET 并正则/截取解析即可，不需要执行 JS。
func (c *Client) SyncCheck(ctx context.Context, key SyncKey) (*SyncCheckResult, error) {
	q := url.Values{}
	q.Set("r", fmt.Sprint(time.Now().UnixMilli()))
	q.Set("skey", c.token.SKey)
	q.Set("sid", c.token.Sid)
	q.Set("uin", c.token.Uin)
	q.Set("deviceid", deviceID())
	q.Set("synckey", key.String())
	q.Set(appHeader, appID)

	req, err := c.newRequest(ctx, http.MethodGet, "/cgi-bin/mmwebwx-bin/synccheck", q, nil)
	if err != nil {
		return nil, err
	}
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	return parseSyncCheck(string(body))
}

// Sync 拉取增量消息。仅在 Selector != 0 时调用（selector=0 表示没有新消息）。
func (c *Client) Sync(ctx context.Context, key SyncKey) ([]*Message, SyncKey, error) {
	q := url.Values{}
	q.Set("sid", c.token.Sid)
	q.Set("skey", c.token.SKey)
	q.Set("pass_ticket", c.token.PassTicket)

	req, err := c.newRequest(ctx, http.MethodPost, "/cgi-bin/mmwebwx-bin/webwxsync", q,
		map[string]any{
			"BaseRequest": c.baseRequest(),
			"SyncKey":     key,
			"rr":          -time.Now().Unix(),
		})
	if err != nil {
		return nil, key, err
	}
	body, err := c.do(req)
	if err != nil {
		return nil, key, err
	}

	var resp struct {
		BaseResponse struct {
			Ret    int    `json:"Ret"`
			ErrMsg string `json:"ErrMsg"`
		} `json:"BaseResponse"`
		AddMsgList   []rawMessage `json:"AddMsgList"`
		SyncKey      SyncKey      `json:"SyncKey"`
		SyncCheckKey SyncKey      `json:"SyncCheckKey"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, key, fmt.Errorf("解析 webwxsync: %w", err)
	}
	if resp.BaseResponse.Ret != 0 {
		return nil, key, c.retError("webwxsync", resp.BaseResponse.Ret, resp.BaseResponse.ErrMsg)
	}

	next := resp.SyncKey
	if next.Count == 0 {
		next = key // 服务端没给新游标时保持原值，绝不置空
	}

	msgs := make([]*Message, 0, len(resp.AddMsgList))
	for i := range resp.AddMsgList {
		if m := convertMessage(&resp.AddMsgList[i]); m != nil {
			msgs = append(msgs, m)
		}
	}
	return msgs, next, nil
}

// Run 长轮询循环：synccheck → selector≠0 时 sync → 回调 handler。
//
// 节奏完全复刻网页版：空闲时每个 synccheck 挂 25s，几乎没有额外请求，
// 这是「不触发风控」的关键。任何出错都退避，绝不快速重试。
func (c *Client) Run(ctx context.Context, key SyncKey, h func(context.Context, *Message)) error {
	backoff := 5 * time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		sc, err := c.SyncCheck(ctx, key)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, ErrSessionExpired) {
				return err
			}
			c.logger.Warn("wechatweb: synccheck 失败，退避重试", "err", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < time.Minute {
				backoff *= 2
			}
			continue
		}
		backoff = 5 * time.Second

		if sc.Selector == 0 {
			continue // 没有新消息，立刻进入下一次长轮询
		}

		msgs, next, err := c.Sync(ctx, key)
		if err != nil {
			if errors.Is(err, ErrSessionExpired) {
				return err
			}
			c.logger.Warn("wechatweb: webwxsync 失败", "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
			continue
		}
		key = next
		if c.onKey != nil {
			c.onKey(next)
		}
		for _, m := range msgs {
			h(ctx, m)
		}
	}
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// synccheck 的响应很小，其它接口最大 ~200KB；限 1MB 防御异常响应
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return body, fmt.Errorf("%s %s: %s", req.Method, req.URL.Path, resp.Status)
	}
	return body, nil
}

// retError 把服务端 ret 映射成哨兵错误。
//
// 1100/1101/1102 都表示会话不可用（未登录/被踢/凭证失效），必须重新扫码；
// 其它 ret 当作普通错误，由调用方退避。
func (c *Client) retError(op string, ret int, msg string) error {
	switch ret {
	case 1100, 1101, 1102:
		return fmt.Errorf("%w (op=%s ret=%d %s)", ErrSessionExpired, op, ret, msg)
	}
	return fmt.Errorf("wechatweb: %s 返回 ret=%d %s", op, ret, msg)
}
