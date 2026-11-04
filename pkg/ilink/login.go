package ilink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrQRExpired 表示二维码过期，需要重新取码。
var ErrQRExpired = errors.New("ilink: QR code expired")

// ErrLoginCancelled 表示用户在手机上取消了授权。
var ErrLoginCancelled = errors.New("ilink: login cancelled")

// LoginSession 是一次扫码登录会话。
type LoginSession struct {
	client *Client
	QRCode string
	// URL 是二维码内容（微信内可直接打开的链接），交给用户扫码/点击。
	URL string
}

// StartLogin 申请一个新的登录二维码。
//
// 调用方负责把 URL 呈现给用户（终端打印 / 渲染成图片 / 后台页面），
// 然后调用 Wait 等待确认。二维码几分钟即过期，过期后应重新 StartLogin。
func StartLogin(ctx context.Context, opts ...Option) (*LoginSession, error) {
	c := NewClient(Token{}, opts...)
	req, err := c.newRequest(ctx, http.MethodGet, "/ilink/bot/get_bot_qrcode?bot_type=3", nil)
	if err != nil {
		return nil, err
	}
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		QRCode string `json:"qrcode"`
		Img    string `json:"qrcode_img_content"`
		Ret    int    `json:"ret"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse get_bot_qrcode: %w (body=%s)", err, truncate(string(body), 200))
	}
	if resp.Ret != 0 || resp.QRCode == "" {
		return nil, fmt.Errorf("ilink: get_bot_qrcode returned ret=%d body=%s", resp.Ret, truncate(string(body), 200))
	}
	return &LoginSession{client: c, QRCode: resp.QRCode, URL: resp.Img}, nil
}

// Wait 阻塞等待用户扫码确认。二维码过期返回 ErrQRExpired，取消返回 ErrLoginCancelled。
func (l *LoginSession) Wait(ctx context.Context) (*Token, error) {
	// 二维码有效期约 4 分钟，留 30s 余量
	deadline := time.Now().Add(4*time.Minute + 30*time.Second)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrQRExpired
		}

		status, tok, err := l.PollOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// 网络抖动不应终止登录，继续轮询
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		switch status {
		case "confirmed":
			return tok, nil
		case "expired":
			return nil, ErrQRExpired
		case "cancel", "cancelled":
			return nil, ErrLoginCancelled
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

// PollOnce 查一次扫码状态（非阻塞），供「在后台页面轮询扫码结果」使用。
//
// status 取值：wait / scaned / confirmed / expired（服务端可能新增其它值，
// 调用方按「非 confirmed 即继续等」处理即可）。confirmed 时返回 Token。
func (l *LoginSession) PollOnce(ctx context.Context) (string, *Token, error) {
	c := l.client
	req, err := c.newRequest(ctx, http.MethodGet, "/ilink/bot/get_qrcode_status?qrcode="+l.QRCode, nil)
	if err != nil {
		return "", nil, err
	}
	// 登录接口要求显式上报客户端版本，缺了会被拒
	req.Header.Set("iLink-App-ClientVersion", "1")
	body, err := c.do(req)
	if err != nil {
		return "", nil, err
	}
	var resp struct {
		Status   string `json:"status"`
		BotToken string `json:"bot_token"`
		BotID    string `json:"ilink_bot_id"`
		UserID   string `json:"ilink_user_id"`
		BaseURL  string `json:"baseurl"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", nil, fmt.Errorf("parse get_qrcode_status: %w", err)
	}
	if resp.Status != "confirmed" {
		return resp.Status, nil, nil
	}
	if resp.BotToken == "" {
		return "", nil, fmt.Errorf("ilink: confirmed without bot_token")
	}
	return resp.Status, &Token{
		BotToken: resp.BotToken,
		BotID:    resp.BotID,
		UserID:   resp.UserID,
		BaseURL:  resp.BaseURL,
	}, nil
}
