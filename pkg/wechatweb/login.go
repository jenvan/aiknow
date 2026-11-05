package wechatweb

import (
	"context"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 登录相关域名。登录前在 filehelper，重定向后业务在 szfilehelper。
const (
	loginHost     = "https://login.wx.qq.com"
	qrImageHost   = "https://login.weixin.qq.com"
	newLoginPage  = apiHost + redirectURIPath
	pollTimeout   = 30 * time.Second
	wantedCookies = "wxuin,wxsid,webwx_data_ticket,webwx_auth_ticket,webwxuvid,wxpluginkey,wxloadtime,mm_lang"
)

// LoginSession 一次扫码登录会话。
type LoginSession struct {
	UUID string
	// QRURL 二维码图片地址（可在手机上长按识别，或渲染成图）
	QRURL string
	// OpenURL 微信内可直接打开的登录链接
	OpenURL string

	http *http.Client
}

var (
	qrLoginRe  = regexp.MustCompile(`window\.QRLogin\.uuid\s*=\s*"([^"]+)"`)
	loginCode  = regexp.MustCompile(`window\.code\s*=\s*(\d+)`)
	redirectRE = regexp.MustCompile(`window\.redirect_uri\s*=\s*"([^"]+)"`)
)

// StartLogin 申请登录二维码。
func StartLogin(ctx context.Context, opts ...Option) (*LoginSession, error) {
	c := &Client{http: &http.Client{Timeout: 20 * time.Second}, logger: slog.Default()}
	for _, o := range opts {
		o(c)
	}

	// redirect_uri 需要双重编码（实测页面就是双重编码）
	redirect := url.QueryEscape(url.QueryEscape(newLoginPage))
	u := fmt.Sprintf("%s/jslogin?appid=%s&redirect_uri=%s&fun=new&lang=zh_CN&_=%d",
		loginHost, appID, redirect, time.Now().UnixMilli())

	body, err := simpleGet(ctx, c.http, u)
	if err != nil {
		return nil, fmt.Errorf("wechatweb: jslogin 失败: %w", err)
	}
	m := qrLoginRe.FindStringSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("wechatweb: jslogin 响应无法解析: %.120s", body)
	}
	uuid := m[1]
	return &LoginSession{
		UUID:    uuid,
		QRURL:   qrImageHost + "/qrcode/" + uuid,
		OpenURL: qrImageHost + "/l/" + uuid,
		http:    c.http,
	}, nil
}

// PollOnce 查一次扫码状态（非阻塞），供后台页面按需轮询。
//
// status 取值：wait（未扫码）/ scaned（已扫码待确认）/ confirmed（成功，tok 非空）。
func (l *LoginSession) PollOnce(ctx context.Context) (string, *Token, error) {
	u := fmt.Sprintf("%s/cgi-bin/mmwebwx-bin/login?loginicon=true&uuid=%s&tip=1&r=%d&_=%d&appid=%s",
		loginHost, url.QueryEscape(l.UUID), 1400000000+randInt(900000), time.Now().UnixMilli(), appID)
	body, err := simpleGet(ctx, l.http, u)
	if err != nil {
		return "", nil, err
	}

	code := 0
	if m := loginCode.FindStringSubmatch(body); m != nil {
		fmt.Sscanf(m[1], "%d", &code)
	}
	switch code {
	case 200:
		m := redirectRE.FindStringSubmatch(body)
		if m == nil {
			return "", nil, fmt.Errorf("wechatweb: 登录成功但没有 redirect_uri: %.160s", body)
		}
		tok, err := l.fetchToken(ctx, m[1])
		if err != nil {
			return "", nil, err
		}
		return "confirmed", tok, nil
	case 201:
		return "scaned", nil, nil
	default:
		return "wait", nil, nil
	}
}

// Wait 轮询扫码状态，直到确认登录并返回凭据。
//
// 二维码有效期由服务端控制（实测约 4-5 分钟），过期返回 ErrQRCodeExpired，
// 调用方换新码即可。
func (l *LoginSession) Wait(ctx context.Context) (*Token, error) {
	deadline := time.Now().Add(4 * time.Minute)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrQRCodeExpired
		}

		status, tok, err := l.PollOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if status == "confirmed" {
			return tok, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

// ErrQRCodeExpired 二维码过期。
var ErrQRCodeExpired = fmt.Errorf("wechatweb: 二维码已过期")

// loginPageXML webwxnewloginpage 下发的凭证（XML）。
type loginPageXML struct {
	Ret        string `xml:"ret"`
	Message    string `xml:"message"`
	SKey       string `xml:"skey"`
	WxSid      string `xml:"wxsid"`
	WxUin      string `xml:"wxuin"`
	PassTicket string `xml:"pass_ticket"`
}

// fetchToken 用 redirect_uri 换凭据，并收集 cookie。
func (l *LoginSession) fetchToken(ctx context.Context, redirect string) (*Token, error) {
	if !strings.Contains(redirect, "fun=new") {
		redirect += "&fun=new"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, redirect, nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wechatweb: 获取凭据失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var lp loginPageXML
	if err := xml.Unmarshal(body, &lp); err != nil {
		return nil, fmt.Errorf("wechatweb: 凭据 XML 解析失败: %w (%.160s)", err, body)
	}
	if lp.Ret != "0" {
		return nil, fmt.Errorf("wechatweb: 凭据下发失败 ret=%s %s", lp.Ret, lp.Message)
	}
	if lp.SKey == "" || lp.WxSid == "" || lp.WxUin == "" {
		return nil, fmt.Errorf("wechatweb: 凭据不完整 (skey/sid/uin 有空值)")
	}

	cookies := map[string]string{}
	for _, c := range resp.Cookies() {
		cookies[c.Name] = c.Value
	}
	// 只保留后续请求需要的；其余（跟踪类）不落库
	need := strings.Split(wantedCookies, ",")
	kept := map[string]string{}
	for _, n := range need {
		if v, ok := cookies[strings.TrimSpace(n)]; ok {
			kept[strings.TrimSpace(n)] = v
		}
	}

	return &Token{
		Uin:        lp.WxUin,
		Sid:        lp.WxSid,
		SKey:       lp.SKey,
		PassTicket: lp.PassTicket,
		Cookies:    kept,
	}, nil
}

func simpleGet(ctx context.Context, hc *http.Client, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set(appHeader, appID)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}

func randInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 123456
	}
	return int(n.Int64())
}
