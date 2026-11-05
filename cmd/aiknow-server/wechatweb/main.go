// Package wechatweb 提供 `aiknow-server wechatweb` 子命令：
// 扫码登录、查看状态、登出。
//
// 这条通道用于「手机把公众号文章转发给文件传输助手 → 服务端收下链接入库」。
// 只收不发，协议细节见 pkg/wechatweb。
package wechatweb

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jenvan/aiknow/internal/channel/wechatweb"
	"github.com/jenvan/aiknow/internal/config"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/pkg/securebox"
)

// Main 处理 wechatweb 子命令。
func Main(args []string) {
	if len(args) == 0 {
		printUsage()
		return
	}
	switch args[0] {
	case "login":
		login()
	case "status":
		status()
	case "logout":
		logout(args[1:])
	default:
		fmt.Printf("未知的 wechatweb 子命令: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`用法:
  aiknow-server wechatweb login     扫码登录文件传输助手（只收不发）
  aiknow-server wechatweb status    查看会话与同步游标
  aiknow-server wechatweb logout    删除本地凭据

说明:
  - 登录后服务端会以「只读」方式长轮询，接收你从手机转发给文件传输助手的公众号文章；
  - 不实现任何发送接口，权限面最小；
  - 会话凭据加密存储（密钥来自 SESSION_SECRET）；
  - **不要同时开着网页版**：同一账号两个消费者会抢消息、重复投递。
`)
}

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

func newBox(cfg *config.Config) (*securebox.Box, error) {
	secret, _ := cfg.Get("SESSION_SECRET")
	return securebox.New(secret)
}

func login() {
	ctx := context.Background()
	db, cfg, closeDB, err := openDB(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()

	box, err := newBox(cfg)
	if err != nil {
		fmt.Printf("无法初始化加密（SESSION_SECRET 必须非空）: %v\n", err)
		os.Exit(1)
	}

	if err := wechatweb.Login(ctx, db.GetWechatWebRepo(), box, os.Stdout); err != nil {
		fmt.Printf("登录失败: %v\n", err)
		os.Exit(1)
	}
}

func status() {
	ctx := context.Background()
	db, _, closeDB, err := openDB(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()
	if err := wechatweb.Status(ctx, db.GetWechatWebRepo(), os.Stdout); err != nil {
		fmt.Printf("读取状态失败: %v\n", err)
		os.Exit(1)
	}
}

func logout(args []string) {
	fs := flag.NewFlagSet("wechatweb logout", flag.ExitOnError)
	yes := fs.Bool("yes", false, "跳过确认")
	_ = fs.Parse(args)

	ctx := context.Background()
	db, _, closeDB, err := openDB(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()

	repo := db.GetWechatWebRepo()
	sess, err := repo.Get(ctx)
	if err != nil {
		fmt.Printf("读取会话失败: %v\n", err)
		os.Exit(1)
	}
	if sess == nil {
		fmt.Println("没有已保存的会话。")
		return
	}
	if !*yes {
		fmt.Printf("确认删除 %s 的本地凭据？重新启用需再次扫码 [y/N]: ", sess.UserName)
		var ans string
		fmt.Scanln(&ans)
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("已取消。")
			return
		}
	}
	if err := repo.Delete(ctx); err != nil {
		fmt.Printf("删除失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("已删除本地凭据。")
}
