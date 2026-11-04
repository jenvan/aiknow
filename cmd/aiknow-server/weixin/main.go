// Package weixin 提供 `aiknow-server weixin` 子命令：扫码登录、查看状态、登出。
//
// 为什么扫码要放在 CLI 而不是后台页面：
//   - 二维码是几分钟就过期的临时凭证，CLI 打印链接后用户直接在手机微信里打开，
//     最简单也最少活动部件；
//   - bot_token 属凭据，落库即可，不需要经过浏览器/HTTP 层。
package weixin

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jenvan/aiknow/internal/config"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/ilink"
)

// Main 处理 weixin 子命令。
func Main(args []string) {
	if len(args) == 0 {
		printUsage()
		return
	}
	switch args[0] {
	case "login":
		login(args[1:])
	case "status":
		status()
	case "logout":
		logout(args[1:])
	case "send-image":
		sendImage(args[1:])
	default:
		fmt.Printf("未知的 weixin 子命令: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`用法:
  aiknow-server weixin login [--allow-from "a@im.wechat,b@im.wechat"]
  aiknow-server weixin status
  aiknow-server weixin logout [--yes]
  aiknow-server weixin send-image <图片路径>

说明:
  login    扫码登录微信 iLink Bot（等价于把「微信 ClawBot」插件授权给 AiKnow）
  status   查看当前账号、白名单、长轮询游标与最近活跃时间
  logout   删除本地保存的 bot 凭据
  send-image  往最近一次会话窗口发一张图片（用库里保存的 bot 凭据与 context_token）

提示:
  - 扫码入口：手机微信 → 我 → 设置 → 关于微信，确认版本 8.0.70+，
    再到 设置 → 插件 → 微信 ClawBot 授权；
  - 白名单默认只放行扫码者本人，无需额外配置即可自用；
  - 凭据与游标存数据库（weixin_bot_accounts），不写 .env。
`)
}

// openDB 复用与 serve 相同的配置/连接装配路径。
func openDB(ctx context.Context) (*repository.PostgresDB, *config.Config, func(), error) {
	cfg := config.NewConfig()
	dsn, _, err := cfg.DatabaseURL()
	if err != nil {
		return nil, nil, nil, err
	}
	db, err := repository.NewPostgresDB(ctx, dsn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("数据库连接失败: %w", err)
	}
	return db, cfg, db.Close, nil
}

func login(args []string) {
	fs := flag.NewFlagSet("weixin login", flag.ExitOnError)
	allowFrom := fs.String("allow-from", "", "允许使用 bot 的微信用户 ID（逗号分隔）；留空=仅扫码者本人")
	_ = fs.Parse(args)

	ctx := context.Background()
	db, cfg, closeDB, err := openDB(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()

	repo := db.GetWeixinRepo()
	existing, err := repo.Get(ctx)
	if err != nil {
		fmt.Printf("读取现有账号失败: %v\n", err)
		os.Exit(1)
	}
	if existing != nil {
		fmt.Printf("已存在 bot %s（扫码者 %s），本次扫码将覆盖凭据。\n", existing.BotID, existing.OwnerUserID)
	}

	base := cfg.GetOr("WEIXIN_BOT_BASE_URL", "")
	opts := []ilink.Option{ilink.WithBaseURL(base)}

	// 二维码几分钟过期，过期即换新码重来，直到用户扫码成功
	for attempt := 1; attempt <= 30; attempt++ {
		ls, err := ilink.StartLogin(ctx, opts...)
		if err != nil {
			fmt.Printf("获取二维码失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\n请用手机微信打开下面的链接（或扫码）授权：")
		fmt.Println("  " + ls.URL)
		fmt.Println("（等待扫码…）")

		tok, err := ls.Wait(ctx)
		switch {
		case errors.Is(err, ilink.ErrQRExpired):
			fmt.Println("二维码已过期，重新取码…")
			continue
		case err != nil:
			fmt.Printf("登录失败: %v\n", err)
			os.Exit(1)
		}

		af := *allowFrom
		if af == "" && existing != nil {
			af = existing.AllowFrom // 未显式指定时沿用旧白名单，避免扫码把白名单清空
		}
		acc := &model.WeixinBotAccount{
			BotID:       tok.BotID,
			BotToken:    tok.BotToken,
			BaseURL:     firstNonEmpty(tok.BaseURL, base),
			OwnerUserID: tok.UserID,
			AllowFrom:   af,
		}
		if err := repo.SaveLogin(ctx, acc); err != nil {
			fmt.Printf("保存凭据失败: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("\n登录成功\n  bot:   %s\n  扫码者: %s\n", tok.BotID, tok.UserID)
		if af == "" {
			fmt.Println("  白名单: 仅扫码者本人（如需他人使用，重跑并加 --allow-from）")
		} else {
			fmt.Printf("  白名单: %s\n", af)
		}
		fmt.Println("\n下一步：在 .env 里设 WEIXIN_BOT_ENABLED=true 后重启 serve。")
		return
	}
	fmt.Println("多次换码仍未完成扫码，已放弃。")
	os.Exit(1)
}

func status() {
	ctx := context.Background()
	db, _, closeDB, err := openDB(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()

	acc, err := db.GetWeixinRepo().Get(ctx)
	if err != nil {
		fmt.Printf("读取账号失败: %v\n", err)
		os.Exit(1)
	}
	if acc == nil {
		fmt.Println("尚未登录。执行：aiknow-server weixin login")
		return
	}

	allowFrom := acc.AllowFrom
	if strings.TrimSpace(allowFrom) == "" {
		allowFrom = "（空）仅扫码者本人"
	}
	lastSeen := "-"
	if acc.LastSeenAt != nil {
		lastSeen = acc.LastSeenAt.Format(time.DateTime)
	}
	cursor := "空（重启会从头拉取）"
	if acc.Cursor != "" {
		cursor = "已保存"
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "bot_id\t"+acc.BotID)
	fmt.Fprintln(w, "扫码者\t"+acc.OwnerUserID)
	fmt.Fprintln(w, "白名单\t"+allowFrom)
	fmt.Fprintln(w, "游标\t"+cursor)
	fmt.Fprintln(w, "最近活跃\t"+lastSeen)
	fmt.Fprintln(w, "更新时间\t"+acc.UpdatedAt.Format(time.DateTime))
	_ = w.Flush()
}

func logout(args []string) {
	fs := flag.NewFlagSet("weixin logout", flag.ExitOnError)
	yes := fs.Bool("yes", false, "跳过确认")
	_ = fs.Parse(args)

	ctx := context.Background()
	db, _, closeDB, err := openDB(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()

	repo := db.GetWeixinRepo()
	acc, err := repo.Get(ctx)
	if err != nil {
		fmt.Printf("读取账号失败: %v\n", err)
		os.Exit(1)
	}
	if acc == nil {
		fmt.Println("没有已保存的账号。")
		return
	}
	if !*yes {
		fmt.Printf("确认删除 bot %s 的本地凭据？重新启用需再次扫码 [y/N]: ", acc.BotID)
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("已取消。")
			return
		}
	}
	if err := repo.Delete(ctx, acc.BotID); err != nil {
		fmt.Printf("删除失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("已删除本地凭据。")
}

// sendImage 往最近一次会话发一张图片。
//
// 用途：把某个需要「长按识别二维码」的图（如文件传输助手网页版登录码）直接发到
// 微信里，省去"用另一个设备拍屏幕"。
//
// context_token 用的是库里最后一次 inbound 消息的——微信侧可能已过期，
// 失败时会明确报错，不会静默成功。
func sendImage(args []string) {
	fs := flag.NewFlagSet("weixin send-image", flag.ExitOnError)
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Println("用法: aiknow-server weixin send-image <图片路径>")
		os.Exit(1)
	}
	path := fs.Arg(0)

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("读取图片失败: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	db, _, closeDB, err := openDB(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()

	acc, err := db.GetWeixinRepo().Get(ctx)
	if err != nil || acc == nil {
		fmt.Println("尚未登录 bot，先执行 aiknow-server weixin login")
		os.Exit(1)
	}
	if acc.LastContextToken == "" {
		fmt.Println("没有可用的 context_token：请先在微信里给 bot 发一条消息")
		os.Exit(1)
	}

	client := ilink.NewClient(ilink.Token{
		BotToken: acc.BotToken,
		BotID:    acc.BotID,
		UserID:   acc.OwnerUserID,
		BaseURL:  acc.BaseURL,
	})
	if err := client.SendImage(ctx, acc.OwnerUserID, acc.LastContextToken, data); err != nil {
		fmt.Printf("发送失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已发送 %d 字节图片给 %s\n", len(data), acc.OwnerUserID)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
