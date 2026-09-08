// Package main 入口文件 - AiKnow 服务器主程序
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jenvan/aiknow/cmd/aiknow-server/hashpwd"
	"github.com/jenvan/aiknow/internal/config"
	"github.com/jenvan/aiknow/internal/handler/admin"
	"github.com/jenvan/aiknow/internal/handler/api"
	"github.com/jenvan/aiknow/internal/handler/mcp"
	"github.com/jenvan/aiknow/internal/handler/web"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/internal/service"
	"github.com/jenvan/aiknow/internal/service/provider"
	"github.com/jenvan/aiknow/pkg/pgqueue"
)

// 版本信息，由 ldflags 注入（必须是 var，const 无法被 -X 覆盖）
var (
	version   = "0.1.0"
	gitCommit = "unknown"
)

func main() {
	// 解析命令行参数
	var showHelp, showVersion bool
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&showVersion, "version", false, "显示版本信息")
	flag.Parse()

	if showVersion {
		fmt.Printf("%s (commit %s)\n", version, gitCommit)
		return
	}

	if showHelp || len(os.Args) == 1 {
		printHelp()
		return
	}

	// hashpwd 子命令
	if len(os.Args) > 1 && os.Args[1] == "hashpwd" {
		hashpwd.Main(os.Args[2:])
		return
	}

	// migrate 子命令
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		migrate()
		return
	}

	// serve 子命令
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		serve()
		return
	}

	printHelp()
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Printf(`AiKnow - AI 知识库管理系统 v%s

用法:
  aiknow-server [选项] [子命令] [参数]

选项:
  --help      显示此帮助信息
  --version   显示版本信息

子命令:
  hashpwd [密码]    生成 bcrypt 密码哈希
  migrate           执行数据库迁移后退出（用于开发期初始化/重建）
  serve             启动 HTTP 服务（启动时会自动执行迁移）

示例:
  aiknow-server --help
  aiknow-server version
  aiknow-server hashpwd mysecretpassword
  aiknow-server migrate
  aiknow-server serve
`, version)
}

// runMigrations 在单个事务中执行所有待执行的迁移并提交。
//
// 迁移须整体提交：DDL 停留在未提交事务中时，连接池内其他连接无法看到新建的表。
func runMigrations(ctx context.Context, db *repository.PostgresDB) error {
	tx, err := db.GetPool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启迁移事务: %w", err)
	}
	// Commit 成功后 Rollback 为空操作，失败路径则由它回滚
	defer func() { _ = tx.Rollback(ctx) }()

	if err := repository.RunMigrations(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交迁移事务: %w", err)
	}
	return nil
}

// migrate 执行数据库迁移后退出
//
// 迁移逻辑与 serve 共用 runMigrations，避免开发期与生产环境的表结构产生漂移。
// 扩展（vector / pg_search）不由迁移创建，需预先执行
// docker-compose-initdb.sql，详见 docs/DEPLOY.md。
func migrate() {
	ctx := context.Background()

	cfg := config.NewConfig()
	dsn, _ := cfg.Get("DATABASE_URL")
	if dsn == "" {
		fmt.Println("缺少 DATABASE_URL 配置（可通过环境变量或 .env 提供）")
		os.Exit(1)
	}

	db, err := repository.NewPostgresDB(ctx, dsn)
	if err != nil {
		fmt.Printf("数据库连接失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := runMigrations(ctx, db); err != nil {
		fmt.Printf("迁移失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("数据库迁移完成")
}

// serve 启动 HTTP 服务 - 装配所有路由和服务
func serve() {
	ctx := context.Background()

	// 加载配置
	cfg := config.NewConfig()
	if err := cfg.Validate(); err != nil {
		fmt.Printf("配置验证失败: %v\n", err)
		os.Exit(1)
	}
	cfg.LogSources()

	// 初始化数据库连接
	dsn, _ := cfg.Get("DATABASE_URL")
	db, err := repository.NewPostgresDB(ctx, dsn)
	if err != nil {
		fmt.Printf("数据库连接失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	fmt.Println("数据库连接成功")

	// 执行迁移
	if err := runMigrations(ctx, db); err != nil {
		fmt.Printf("迁移失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("数据库迁移完成")

	// 初始化 AI Provider
	//
	// 三个 Provider 各自独立选择，缺省为 mock，便于无凭据时本地开发。
	// 选择 qwen / baai 时，对应的 API Key 必填，否则构造函数会报错。
	qwenKey, _ := cfg.Get("QWEN_API_KEY")
	qwenBaseURL, _ := cfg.Get("QWEN_BASE_URL")
	baaiKey, _ := cfg.Get("BAAI_API_KEY")
	baaiBaseURL, _ := cfg.Get("BAAI_BASE_URL")

	factory := provider.NewProviderFactory(provider.L1ProviderConfig{
		LLMProvider: cfg.GetOr("LLM_PROVIDER", "mock"),
		LLMAPIKey:   qwenKey,
		LLMBaseURL:  qwenBaseURL,

		EmbeddingProvider: cfg.GetOr("EMBEDDING_PROVIDER", "mock"),
		EmbeddingAPIKey:   baaiKey,
		EmbeddingBaseURL:  baaiBaseURL,

		RerankerProvider: cfg.GetOr("RERANKER_PROVIDER", "mock"),
		RerankerAPIKey:   baaiKey,
		RerankerBaseURL:  baaiBaseURL,
	})

	// 构造失败不能忽略：此前以 _ 丢弃错误，会导致 provider 为 nil，
	// 直到首次调用才 panic
	llmProvider, err := factory.CreateLLM(ctx)
	if err != nil {
		fmt.Printf("初始化 LLM Provider 失败: %v\n", err)
		os.Exit(1)
	}
	embeddingProvider, err := factory.CreateEmbedding(ctx)
	if err != nil {
		fmt.Printf("初始化 Embedding Provider 失败: %v\n", err)
		os.Exit(1)
	}
	rerankerProvider, err := factory.CreateReranker(ctx)
	if err != nil {
		fmt.Printf("初始化 Reranker Provider 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("AI Provider 初始化完成: LLM=%s, Embedding=%s, Reranker=%s\n",
		llmProvider.Name(), embeddingProvider.Name(), rerankerProvider.Name())

	// 初始化服务
	articleService := service.NewArticleService(
		db.GetArticleRepo(),
		db.GetChunkRepo(),
		db.GetCategoryRepo(),
		db.GetTagRepo(),
	)

	searchService := service.NewSearchService(
		db.GetSearchRepo(),
		embeddingProvider,
		rerankerProvider,
		db.GetChunkRepo(),
		db.GetCategoryRepo(),
		db.GetTagRepo(),
	)

	// 管理员凭据来自 L1 配置；此前 Login 内硬编码 admin/admin，
	// 配置中的用户名与密码从未生效
	adminUser, _ := cfg.Get("ADMIN_USERNAME")
	adminHash, _ := cfg.Get("ADMIN_PASSWORD_HASH")
	adminPwd, _ := cfg.Get("ADMIN_PASSWORD")

	authService, err := service.NewAuthService(service.AuthConfig{
		Username:        adminUser,
		PasswordHash:    adminHash,
		Password:        adminPwd,
		SessionLifetime: cfg.GetDuration("SESSION_LIFETIME", service.DefaultSessionLifetime),
	})
	if err != nil {
		fmt.Printf("初始化认证服务失败: %v\n", err)
		os.Exit(1)
	}
	if adminHash == "" {
		fmt.Println("警告: 正在使用明文 ADMIN_PASSWORD（仅限开发环境），" +
			"生产环境请改用 ADMIN_PASSWORD_HASH")
	}

	// 初始化 Gin 路由器
	r := gin.Default()

	// API v1 路由组
	apiGroup := r.Group("/api/v1")
	{
		// 健康检查
		apiGroup.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "version": version})
		})

		// 认证路由
		// Cookie 的 Secure 由环境决定：本地开发走 HTTP，置 true 浏览器不会回传
		authHandler := admin.NewAuthHandler(authService, admin.CookieConfig{
			MaxAge: cfg.GetDuration("SESSION_LIFETIME", service.DefaultSessionLifetime),
			Secure: cfg.IsProduction(),
		})
		apiGroup.POST("/admin/auth/login", authHandler.Login)
		apiGroup.POST("/admin/auth/logout", authHandler.Logout)
		apiGroup.GET("/admin/auth/session", authHandler.GetSession)

		// 文章路由
		articleHandler := api.NewArticleHandler(articleService)
		apiGroup.GET("/articles", articleHandler.List)
		apiGroup.GET("/articles/:id", articleHandler.GetByID)
		apiGroup.POST("/articles", articleHandler.Create)
		apiGroup.DELETE("/articles/:id", articleHandler.SoftDelete)

		// 检索路由
		searchHandler := api.NewSearchHandler(searchService)
		apiGroup.POST("/search", searchHandler.Search)
		apiGroup.POST("/search/keywords", searchHandler.SearchKeywords)
	}

	// MCP 路由（需要 API Key 鉴权）
	mcpService := service.NewMCPService(
		searchService,
		service.NewKeywordSearchService(db.GetSearchRepo(), db.GetCategoryRepo(), db.GetTagRepo()),
		articleService,
		authService,
		service.NewMCPLogService(nil), // TODO: 接入 MCPLogRepository
	)
	mcpHandler := mcp.NewServer(mcpService, authService)
	r.GET("/mcp", mcpHandler.Handle)
	r.POST("/mcp", mcpHandler.Handle)

	// Web UI 路由（SPA 静态文件服务）
	webHandler := web.NewWebHandler()
	webHandler.RegisterRoutes(r)

	// 启动 Worker（AI 处理流水线）
	queue := pgqueue.NewQueue(db.DB(), "article_processing")
	pipeline := service.NewAIPipeline(
		llmProvider,
		embeddingProvider,
		db.GetArticleRepo(),
		db.GetChunkRepo(),
		db.GetCategoryRepo(),
		db.GetTagRepo(),
	)

	worker := service.NewWorker(db, pipeline, queue, service.DefaultWorkerConfig())
	go func() {
		if err := worker.Start(); err != nil {
			fmt.Printf("Worker 启动失败: %v\n", err)
		}
	}()
	defer worker.Stop()

	// 启动 HTTP 服务器
	port, _ := cfg.Get("PORT")
	addr := fmt.Sprintf(":%s", port)
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		fmt.Printf("服务器启动在 %s\n", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("服务器启动失败: %v\n", err)
			os.Exit(1)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭服务器...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Printf("服务器强制关闭: %v\n", err)
	}

	fmt.Println("服务器已关闭")
}
