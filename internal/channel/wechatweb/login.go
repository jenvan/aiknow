package wechatweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/securebox"
	"github.com/jenvan/aiknow/pkg/wechatweb"
)

// Login 完成一次扫码登录并把凭据落库。
//
// 流程：申请二维码 → 等扫码 → **只做一次 webwxinit** 拿初始 SyncKey 与 UserName
// → 加密存库。之后接收器不再调用 Init：重复 Init 会重置 SyncKey，历史消息就补不回来。
func Login(ctx context.Context, repo repository.WechatWebRepository, box *securebox.Box, out io.Writer) error {
	if box == nil {
		return errors.New("wechatweb: 缺少 SESSION_SECRET，无法加密凭据")
	}

	for attempt := 1; attempt <= 20; attempt++ {
		sess, err := wechatweb.StartLogin(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "\n请用手机微信扫码（或直接打开下面的链接）：\n  %s\n", sess.QRURL)
		fmt.Fprintf(out, "等待扫码…（二维码约 4 分钟过期，过期会自动换新码）\n")

		tok, err := sess.Wait(ctx)
		if errors.Is(err, wechatweb.ErrQRCodeExpired) {
			fmt.Fprintln(out, "二维码已过期，重新取码…")
			continue
		}
		if err != nil {
			return err
		}

		// 只在这里 Init 一次
		client := wechatweb.NewClient(*tok)
		init, err := client.Init(ctx)
		if err != nil {
			return fmt.Errorf("webwxinit 失败: %w", err)
		}
		tok.UserName = init.UserName

		creds, err := json.Marshal(tok)
		if err != nil {
			return err
		}
		enc, err := box.Encrypt(creds)
		if err != nil {
			return err
		}
		keyJSON, err := json.Marshal(init.SyncKey)
		if err != nil {
			return err
		}
		if err := repo.Save(ctx, enc, tok.UserName, keyJSON); err != nil {
			return err
		}

		fmt.Fprintf(out, "\n登录成功\n  账号: %s (%s)\n  同步游标: %d 项\n",
			init.NickName, init.UserName, len(init.SyncKey.List))
		fmt.Fprintln(out, "\n下一步：设 WECHATWEB_ENABLED=true 后重启 serve，即可开始接收。")
		return nil
	}
	return errors.New("多次换码仍未完成扫码，已放弃")
}

// Status 输出当前会话状态（不解密凭据）。
func Status(ctx context.Context, repo repository.WechatWebRepository, out io.Writer) error {
	sess, err := repo.Get(ctx)
	if err != nil {
		return err
	}
	if sess == nil {
		fmt.Fprintln(out, "尚未登录。执行：aiknow-server wechatweb login")
		return nil
	}
	var key wechatweb.SyncKey
	if len(sess.SyncKey) > 0 {
		_ = json.Unmarshal(sess.SyncKey, &key)
	}
	last := "-"
	if sess.LastSyncAt != nil {
		last = sess.LastSyncAt.Format(time.DateTime)
	}
	fmt.Fprintf(out, "账号: %s\n同步游标: %d 项\n最近同步: %s\n创建时间: %s\n",
		sess.UserName, len(key.List), last, sess.CreatedAt.Format(time.DateTime))
	return nil
}
