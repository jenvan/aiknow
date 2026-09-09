package apikey

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/jenvan/aiknow/internal/config"
	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/internal/service"
)

// Main 处理 apikey 子命令。
//
// 后台 UI 也能管理 Key，但那需要先能登录；无头部署、自动化脚本与救急场景
// 需要一条不依赖浏览器的通路。
func Main(args []string) {
	if len(args) == 0 {
		printUsage()
		return
	}

	switch args[0] {
	case "create":
		create(args[1:])
	case "list":
		list()
	case "revoke":
		revoke(args[1:])
	default:
		fmt.Printf("未知的 apikey 子命令: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`用法:
  aiknow-server apikey create --desc <描述> [--scope readonly|readwrite]
  aiknow-server apikey list
  aiknow-server apikey revoke <id>

说明:
  create   创建 API Key，明文仅在此刻打印一次，服务端只存 SHA-256 哈希
  list     列出所有 Key（不含明文与哈希）
  revoke   删除指定 Key

示例:
  aiknow-server apikey create --desc "Claude Desktop" --scope readonly
`)
}

// newAuthService 复用与 serve 相同的装配路径，避免两处初始化逻辑漂移
func newAuthService(ctx context.Context) (service.AuthService, func(), error) {
	cfg := config.NewConfig()
	dsn, _, err := cfg.DatabaseURL()
	if err != nil {
		return nil, nil, err
	}

	db, err := repository.NewPostgresDB(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	adminUser, _ := cfg.Get("ADMIN_USERNAME")
	adminHash, _ := cfg.Get("ADMIN_PASSWORD_HASH")
	adminPwd, _ := cfg.Get("ADMIN_PASSWORD")

	auth, err := service.NewAuthService(service.AuthConfig{
		Username:     adminUser,
		PasswordHash: adminHash,
		Password:     adminPwd,
		APIKeyRepo:   db.GetAPIKeyRepo(),
	})
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("初始化认证服务失败: %w", err)
	}

	return auth, db.Close, nil
}

func create(args []string) {
	fs := flag.NewFlagSet("apikey create", flag.ExitOnError)
	desc := fs.String("desc", "", "用途描述（必填）")
	scope := fs.String("scope", service.ScopeReadOnly, "权限范围：readonly | readwrite")
	_ = fs.Parse(args)

	if *desc == "" {
		fmt.Println("错误: --desc 不能为空")
		fs.Usage()
		os.Exit(1)
	}

	ctx := context.Background()
	auth, closeDB, err := newAuthService(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()

	created, err := auth.CreateKey(ctx, model.APIKeyCreateInput{
		Description: *desc,
		Scope:       *scope,
	}, "cli")
	if err != nil {
		fmt.Printf("创建失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(`API Key 创建成功

  ID     %s
  描述   %s
  权限   %s

  明文   %s

请立即保存上面的明文，它不会再次显示——服务端只存哈希。
用法: Authorization: Bearer %s
`, created.ID, created.Description, created.Scope, created.Key, created.Key)
}

func list() {
	ctx := context.Background()
	auth, closeDB, err := newAuthService(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()

	keys, err := auth.ListKeys(ctx)
	if err != nil {
		fmt.Printf("查询失败: %v\n", err)
		os.Exit(1)
	}
	if len(keys) == 0 {
		fmt.Println("尚无 API Key。用 apikey create 创建。")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t前缀\t权限\t状态\t最近使用\t描述")
	for _, k := range keys {
		status := "启用"
		if !k.IsActive {
			status = "停用"
		}
		lastUsed := "从未"
		if k.LastUsedAt != nil {
			lastUsed = k.LastUsedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			k.ID, k.KeyPrefix, k.Scope, status, lastUsed, k.Description)
	}
	_ = w.Flush()
}

func revoke(args []string) {
	if len(args) == 0 {
		fmt.Println("错误: 需要提供 Key ID")
		fmt.Println("用法: aiknow-server apikey revoke <id>")
		os.Exit(1)
	}

	ctx := context.Background()
	auth, closeDB, err := newAuthService(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer closeDB()

	if err := auth.DeleteKey(ctx, args[0]); err != nil {
		fmt.Printf("删除失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已删除 API Key %s\n", args[0])
}
