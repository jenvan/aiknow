// Package main 入口文件 - AiKnow 服务器主程序
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jenvan/aiknow/cmd/aiknow-server/apikey"
	"github.com/jenvan/aiknow/cmd/aiknow-server/hashpwd"
	wechatwebcmd "github.com/jenvan/aiknow/cmd/aiknow-server/wechatweb"
	weixincmd "github.com/jenvan/aiknow/cmd/aiknow-server/weixin"
	channelwechatweb "github.com/jenvan/aiknow/internal/channel/wechatweb"
	channelweixin "github.com/jenvan/aiknow/internal/channel/weixin"
	"github.com/jenvan/aiknow/internal/config"
	"github.com/jenvan/aiknow/internal/handler/admin"
	"github.com/jenvan/aiknow/internal/handler/api"
	"github.com/jenvan/aiknow/internal/handler/mcp"
	"github.com/jenvan/aiknow/internal/handler/web"
	"github.com/jenvan/aiknow/internal/repository"
	"github.com/jenvan/aiknow/internal/service"
	"github.com/jenvan/aiknow/internal/service/provider"
	"github.com/jenvan/aiknow/pkg/cos"
	"github.com/jenvan/aiknow/pkg/pgqueue"
	"github.com/jenvan/aiknow/pkg/securebox"
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

	// apikey 子命令
	if len(os.Args) > 1 && os.Args[1] == "apikey" {
		apikey.Main(os.Args[2:])
		return
	}

	// wechatweb 子命令（文件传输助手：扫码登录 / 状态 / 登出）
	if len(os.Args) > 1 && os.Args[1] == "wechatweb" {
		wechatwebcmd.Main(os.Args[2:])
		return
	}

	// weixin 子命令（扫码登录 / 状态 / 登出）
	if len(os.Args) > 1 && os.Args[1] == "weixin" {
		weixincmd.Main(os.Args[2:])
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
  apikey            管理对外 API / MCP 所用的 API Key（create / list / revoke）
  weixin            微信 iLink Bot（login / status / logout）
  wechatweb         文件传输助手（login / status / logout，只收不发）
  serve             启动 HTTP 服务（启动时会自动执行迁移）

示例:
  aiknow-server --help
  aiknow-server version
  aiknow-server hashpwd mysecretpassword
  aiknow-server migrate
  aiknow-server weixin login
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
	dsn, _, err := cfg.DatabaseURL()
	if err != nil {
		fmt.Println(err)
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
	//
	// 优先 DATABASE_URL；为空时由 POSTGRES_* 分项拼装（compose 部署走这条路径）
	dsn, dsnSrc, err := cfg.DatabaseURL()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("数据库配置来源: %s\n", dsnSrc)

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
	qwenModel, _ := cfg.Get("QWEN_MODEL")
	baaiKey, _ := cfg.Get("BAAI_API_KEY")
	baaiBaseURL, _ := cfg.Get("BAAI_BASE_URL")
	baaiEmbeddingModel, _ := cfg.Get("BAAI_EMBEDDING_MODEL")
	baaiRerankerModel, _ := cfg.Get("BAAI_RERANKER_MODEL")

	factory := provider.NewProviderFactory(provider.L1ProviderConfig{
		LLMProvider: cfg.GetOr("LLM_PROVIDER", "mock"),
		LLMAPIKey:   qwenKey,
		LLMBaseURL:  qwenBaseURL,
		LLMModel:    qwenModel,

		EmbeddingProvider: cfg.GetOr("EMBEDDING_PROVIDER", "mock"),
		EmbeddingAPIKey:   baaiKey,
		EmbeddingBaseURL:  baaiBaseURL,
		EmbeddingModel:    baaiEmbeddingModel,

		RerankerProvider: cfg.GetOr("RERANKER_PROVIDER", "mock"),
		RerankerAPIKey:   baaiKey,
		RerankerBaseURL:  baaiBaseURL,
		RerankerModel:    baaiRerankerModel,
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

	// L2 业务配置缓存（settings 表）。
	//
	// 后台系统配置页的修改通过 SettingHandler.onChanged 回调触发 Reload，
	// 使 L2 配置保持热生效且优先级最高。
	configCache := config.NewConfigCache(db.DB())
	if err := configCache.Reload(ctx); err != nil {
		fmt.Printf("警告: 加载 L2 配置失败: %v\n", err)
	}

	// QwenProvider 支持运行时模型名来源：后台设置 llm.model 后覆盖 L1 QWEN_MODEL
	if s, ok := llmProvider.(provider.ModelSourceSetter); ok {
		s.SetModelSource(configCache)
	}

	// AI 处理任务队列。
	// 需在 articleService 之前创建：文章录入后由它投递异步处理任务。
	queue := pgqueue.NewQueue(db.DB(), "article_processing")

	// 初始化服务
	articleService := service.NewArticleService(
		db.GetArticleRepo(),
		db.GetChunkRepo(),
		db.GetCategoryRepo(),
		db.GetTagRepo(),
		queue,
	)

	// 文件传输助手通道的加密箱与生命周期管理器。
	// 必须在路由注册前创建：后台「微信入口」页的扫码接口要用到它们。
	// 开关 WECHATWEB_ENABLED 只决定**启动时是否立即拉起**接收器，
	// 页面扫码成功也会拉起（Manager.Start），无需重启。
	webSecret, _ := cfg.Get("SESSION_SECRET")
	webBox, webBoxErr := securebox.New(webSecret)
	if webBoxErr != nil {
		fmt.Printf("警告: SESSION_SECRET 不可用，文件传输助手通道无法使用: %v\n", webBoxErr)
		webBox = nil
	}
	webMgr := channelwechatweb.NewManager(
		db.GetWechatWebRepo(), webBox,
		channelwechatweb.ArticleAdapter{Svc: articleService},
		slog.Default(),
	)

	chunkService := service.NewChunkService(db.GetChunkRepo())

	searchService := service.NewSearchService(
		db.GetSearchRepo(),
		embeddingProvider,
		rerankerProvider,
		db.GetChunkRepo(),
		db.GetCategoryRepo(),
		db.GetTagRepo(),
		// L2 配置：检索参数（top_k / candidate_k / rrf_k / 权重）后台可改
		configCache,
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
		// 缺少仓库时 ValidateKey 一律拒绝，避免以"任何 Key 都无效"的静默状态运行
		APIKeyRepo: db.GetAPIKeyRepo(),
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

	// MCP 服务在路由注册前构造：后台调试页与对外 /mcp 共用同一实例
	keywordService := service.NewKeywordSearchService(
		db.GetSearchRepo(), db.GetCategoryRepo(), db.GetTagRepo(),
		// L2 配置：关键词权重（tag/category/title/summary/content）后台可改
		configCache,
	)
	mcpLogService := service.NewMCPLogService(db.GetMCPLogRepo())
	mcpService := service.NewMCPService(
		searchService,
		keywordService,
		articleService,
		authService,
		mcpLogService,
	)
	collectorConfig := service.DefaultCollectorConfig()
	collectorConfig.Timeout = cfg.GetDuration("COLLECTOR_TIMEOUT", collectorConfig.Timeout)
	if v, _ := cfg.Get("COLLECTOR_MAX_REDIRECTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			collectorConfig.MaxRedirects = n
		}
	}
	if v, _ := cfg.Get("COLLECTOR_MIN_CONTENT_LENGTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			collectorConfig.MinContentLength = n
		}
	}
	collectorConfig.UserAgent = cfg.GetOr("COLLECTOR_USER_AGENT", collectorConfig.UserAgent)

	// 无头浏览器渲染：需要 COLLECTOR_BROWSER_ENABLED=true 且 endpoint 非空
	// 才真正走浏览器；否则采集退化为纯 http。
	collectorConfig.RenderEnabled = cfg.GetBool("COLLECTOR_BROWSER_ENABLED", collectorConfig.RenderEnabled)
	collectorConfig.BrowserEndpoint = cfg.GetOr("COLLECTOR_BROWSER_ENDPOINT", collectorConfig.BrowserEndpoint)
	collectorConfig.BrowserToken = cfg.GetOr("COLLECTOR_BROWSER_TOKEN", collectorConfig.BrowserToken)
	collectorConfig.BrowserTimeout = cfg.GetDuration("COLLECTOR_BROWSER_TIMEOUT", collectorConfig.BrowserTimeout)

	collector := service.NewCollector(collectorConfig)
	// 让「给 URL 就入库」成为服务层能力：微信 bot、浏览器扩展、快捷指令、
	// MCP add_article_by_url 全部复用同一条采集链路
	articleService.SetCollector(collector)
	// 后台「系统配置 → collector.user_agent」可覆盖启动配置并热生效
	collector.SetUserAgentSource(db.GetSettingRepo())

	// 腾讯云 COS：采集文章截图归档。配置不全时 Enabled()==false，跳过上传。
	cosClient := cos.New(cos.Config{
		SecretID:   cfg.GetOr("COS_SECRET_ID", ""),
		SecretKey:  cfg.GetOr("COS_SECRET_KEY", ""),
		Bucket:     cfg.GetOr("COS_BUCKET", ""),
		Region:     cfg.GetOr("COS_REGION", ""),
		Endpoint:   cfg.GetOr("COS_ENDPOINT", ""),
		PathPrefix: cfg.GetOr("COS_PATH_PREFIX", "screenshots"),
	})
	if cosClient.Enabled() {
		fmt.Println("COS 截图归档已启用")
	}

	promptService := service.NewPromptService(
		db.GetSettingRepo(), db.GetArticleRepo(), db.GetCategoryRepo(), llmProvider)

	// Cookie 的 Secure 由环境决定：本地开发走 HTTP，置 true 浏览器不会回传
	authHandler := admin.NewAuthHandler(authService, admin.CookieConfig{
		MaxAge: cfg.GetDuration("SESSION_LIFETIME", service.DefaultSessionLifetime),
		Secure: cfg.IsProduction(),
	})

	// ---- 公开路由（无鉴权）----
	//
	// 仅限：健康探针、登录、以及登录态查询（自身即以 Cookie 为凭据）。
	// 其余一律落入下面两个受保护分组。
	public := r.Group("/api/v1")
	{
		public.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "version": version})
		})
		public.POST("/admin/auth/login", authHandler.Login)
		public.POST("/admin/auth/logout", authHandler.Logout)
		public.GET("/admin/auth/session", authHandler.GetSession)
	}

	// ---- 后台管理路由（Session Cookie + CSRF）----
	//
	// 管理后台不复用对外 API：后者已锁 API Key，且 admin 侧接口更完整
	// （批量操作、Chunk 预览、重新处理等）。
	adminGroup := r.Group("/api/v1/admin")
	adminGroup.Use(
		admin.SessionMiddleware(authService),
		admin.CSRFMiddleware(authService),
	)
	{
		adminArticle := admin.NewArticleHandler(articleService, chunkService, collector)
		adminGroup.GET("/articles", adminArticle.List)
		adminGroup.POST("/articles", adminArticle.Create)
		adminGroup.POST("/articles/preview", adminArticle.Preview)
		// 静态段须先于 :id 注册，否则 batch-delete 会被当作文章 ID
		adminGroup.POST("/articles/batch-delete", adminArticle.BatchDelete)
		adminGroup.POST("/articles/batch-reprocess", adminArticle.BatchReprocess)
		adminGroup.GET("/articles/:id", adminArticle.Get)
		adminGroup.PUT("/articles/:id", adminArticle.Update)
		adminGroup.DELETE("/articles/:id", adminArticle.Delete)
		adminGroup.GET("/articles/:id/chunks", adminArticle.GetChunks)
		adminGroup.POST("/articles/:id/reprocess", adminArticle.Reprocess)
		adminGroup.GET("/categories", func(c *gin.Context) {
			cats, err := db.GetCategoryRepo().ListAll(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, cats)
		})
		adminGroup.GET("/tags", func(c *gin.Context) {
			tags, err := db.GetTagRepo().ListAll(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, tags)
		})

		apiKeyHandler := admin.NewAPIKeyHandler(authService)
		adminGroup.GET("/api-keys", apiKeyHandler.List)
		adminGroup.POST("/api-keys", apiKeyHandler.Create)
		adminGroup.POST("/api-keys/:id/enable", apiKeyHandler.Enable)
		adminGroup.POST("/api-keys/:id/disable", apiKeyHandler.Disable)
		adminGroup.DELETE("/api-keys/:id", apiKeyHandler.Delete)

		dashboardHandler := admin.NewDashboardHandler(db, queue, admin.ProviderInfo{
			LLM:       llmProvider.Name(),
			Embedding: embeddingProvider.Name(),
			Reranker:  rerankerProvider.Name(),
		})
		adminGroup.GET("/dashboard/stats", dashboardHandler.GetStats)
		adminGroup.GET("/dashboard/queue", dashboardHandler.GetQueueStatus)
		adminGroup.GET("/dashboard/activity", dashboardHandler.GetRecentActivity)
		adminGroup.GET("/dashboard/health", dashboardHandler.GetHealth)

		searchDebugHandler := admin.NewSearchDebugHandler(searchService, keywordService)
		adminGroup.POST("/search/debug", searchDebugHandler.Debug)
		adminGroup.POST("/search/debug/keywords", searchDebugHandler.DebugKeywords)

		weixinHandler := admin.NewWeixinHandler(
			db.GetWeixinRepo(),
			cfg.GetBool("WEIXIN_BOT_ENABLED", false),
			cfg.GetOr("WEIXIN_BOT_BASE_URL", ""),
		)
		adminGroup.GET("/weixin/status", weixinHandler.GetStatus)
		adminGroup.POST("/weixin/login/start", weixinHandler.StartLogin)
		adminGroup.GET("/weixin/login/poll", weixinHandler.PollLogin)
		adminGroup.POST("/weixin/logout", weixinHandler.Logout)

		wechatWebHandler := admin.NewWechatWebHandler(db.GetWechatWebRepo(), webBox, webMgr)
		adminGroup.GET("/wechatweb/status", wechatWebHandler.GetStatus)
		adminGroup.POST("/wechatweb/login/start", wechatWebHandler.StartLogin)
		adminGroup.GET("/wechatweb/login/poll", wechatWebHandler.PollLogin)
		adminGroup.POST("/wechatweb/logout", wechatWebHandler.Logout)

		mcpDebugHandler := admin.NewMCPDebugHandler(mcpService)
		adminGroup.POST("/mcp/debug", mcpDebugHandler.Invoke)

		logHandler := admin.NewLogHandler(mcpLogService, configCache)
		adminGroup.GET("/logs", logHandler.List)
		adminGroup.DELETE("/logs", logHandler.DeleteExpired)
		adminGroup.GET("/logs/:trace_id", logHandler.Get)

		settingHandler := admin.NewSettingHandler(cfg, db.GetSettingRepo(), func() {
			if err := configCache.Reload(context.Background()); err != nil {
				fmt.Printf("警告: 刷新 L2 配置失败: %v\n", err)
			}
		})
		adminGroup.GET("/settings", settingHandler.GetSettings)
		adminGroup.POST("/settings/reset", settingHandler.ResetAllSettings)
		adminGroup.GET("/settings/:key", settingHandler.GetSetting)
		adminGroup.PUT("/settings/:key", settingHandler.UpdateSetting)
		adminGroup.POST("/settings/:key/reset", settingHandler.ResetSetting)
		adminGroup.GET("/settings/:key/history", settingHandler.GetHistory)

		promptHandler := admin.NewPromptHandler(promptService)
		// 参数名须与 handler 中的 c.Param("key") 一致
		adminGroup.GET("/prompts/:key", promptHandler.GetPrompt)
		adminGroup.PUT("/prompts/:key", promptHandler.UpdatePrompt)
		adminGroup.POST("/prompts/test", promptHandler.TestPrompt)
	}

	// ---- 对外 API 路由（API Key + scope）----
	//
	// scope 语义：读操作需 readonly，写操作需 readwrite；
	// readwrite 隐含 readonly（见 service.ScopeAllows）。
	apiGroup := r.Group("/api/v1")
	apiGroup.Use(api.AuthMiddleware(authService))
	{
		articleHandler := api.NewArticleHandler(articleService)
		apiGroup.GET("/articles", api.RequireScope(service.ScopeReadOnly), articleHandler.List)
		apiGroup.GET("/articles/:id", api.RequireScope(service.ScopeReadOnly), articleHandler.GetByID)
		apiGroup.POST("/articles", api.RequireScope(service.ScopeReadWrite), articleHandler.Create)
		// 只给 URL 的录入入口：浏览器扩展 / 快捷指令 / 脚本
		apiGroup.POST("/articles/from-url", api.RequireScope(service.ScopeReadWrite), articleHandler.CreateFromURL)
		apiGroup.DELETE("/articles/:id", api.RequireScope(service.ScopeReadWrite), articleHandler.SoftDelete)

		searchHandler := api.NewSearchHandler(searchService)
		// 检索虽用 POST，但语义是读，故只需 readonly
		apiGroup.POST("/search", api.RequireScope(service.ScopeReadOnly), searchHandler.Search)
		apiGroup.POST("/search/keywords", api.RequireScope(service.ScopeReadOnly), searchHandler.SearchKeywords)
	}

	// MCP 路由（需要 API Key 鉴权）
	mcpHandler := mcp.NewServer(mcpService, authService)
	r.GET("/mcp", mcpHandler.Handle)
	r.POST("/mcp", mcpHandler.Handle)

	// Web UI 路由（SPA 静态文件服务）
	webHandler := web.NewWebHandler()
	webHandler.RegisterRoutes(r)

	// 启动 Worker（AI 处理流水线）。队列已在服务初始化前创建。
	pipeline := service.NewAIPipeline(
		llmProvider,
		embeddingProvider,
		db.GetArticleRepo(),
		db.GetChunkRepo(),
		db.GetCategoryRepo(),
		db.GetTagRepo(),
		// 传入 promptService，使后台修改的提示词对实际处理生效
		promptService,
	)
	// L2 配置：分块大小与 LLM 参数（model / temperature / max_tokens）后台可调
	pipeline.SetConfigSource(configCache)
	// URL 文章处理时重采集（渲染 + 截图），截图上传 COS 归档
	pipeline.SetCollector(collector)
	pipeline.SetScreenshotStore(cosClient)
	// 正文图片归档：公众号图床有防盗链，不归档的话后台预览全是坏图。
	// 依赖 COS（未配置时 Archive 内部直接透传）。
	if cfg.GetBool("COLLECTOR_ARCHIVE_IMAGES", true) && cosClient.Enabled() {
		pipeline.SetImageArchiver(service.NewImageArchiver(cosClient, collectorConfig.UserAgent))
		fmt.Println("正文图片归档已启用（下载至 COS 并替换链接）")
	}

	worker := service.NewWorker(db, pipeline, queue, service.DefaultWorkerConfig())
	go func() {
		if err := worker.Start(); err != nil {
			fmt.Printf("Worker 启动失败: %v\n", err)
		}
	}()
	defer worker.Stop()

	// ---- 微信通道公共依赖 ----
	//
	// 两条通道共用同一个 ArticleAdapter 与 Pusher：
	//   - 文件传输助手只收，入库后**通过 Bot 发回执**（它自己不发消息）；
	//   - Bot 既收发，也负责处理完成推送。
	// Pusher 只有 Bot 启用时才有发送端（Attach 在 Bot.Run 里做），
	// 因此 SetOnProcessed 也只在 Bot 启用时注册，避免空转告警。
	articleAdapter := channelweixin.ArticleAdapter{Svc: articleService}
	wxPusher := channelweixin.NewPusher(articleAdapter, slog.Default())
	if cfg.GetBool("WEIXIN_BOT_ENABLED", false) {
		worker.SetOnProcessed(wxPusher.Deliver)
	}

	// ---- 文件传输助手（可选通道：只收转发来的公众号文章）----
	//
	// 与微信 Bot 是两条独立通道：这条走网页版协议（逆向），能收到「转发」卡片；
	// Bot 走官方 iLink，收不到转发但零风险。两者都默认关闭。
	// 回执统一走 Bot：本通道只收不发（发送是风控面最大的动作）
	if cfg.GetBool("WEIXIN_BOT_ENABLED", false) {
		webMgr.SetNotifier(channelweixin.NewNotifier(db.GetWeixinRepo(), wxPusher, slog.Default()))
	}
	if cfg.GetBool("WECHATWEB_ENABLED", false) && webBox != nil {
		if err := webMgr.Start(context.Background()); err != nil {
			fmt.Printf("文件传输助手通道未启动: %v\n", err)
		} else {
			fmt.Println("文件传输助手通道已启用（只收不发）")
		}
	}
	defer webMgr.Stop()

	// ---- 微信 iLink Bot（可选通道：单聊 + 采集 + 检索）----
	//
	// 与主服务同进程：直接调 service 层（不需要给 bot 发 API Key，也不走 HTTP 回环）。
	// 开关关闭或未扫码时完全不启动；bot 出错只影响自身，不阻塞 HTTP 主链路。
	if cfg.GetBool("WEIXIN_BOT_ENABLED", false) {
		botCtx, botCancel := context.WithCancel(context.Background())
		defer botCancel()

		botRouter := channelweixin.NewRouter(
			articleAdapter,
			keywordService,
			searchService,
			channelweixin.CollectorAdapter{Svc: collector},
			slog.Default(),
		)
		// 处理完成回推用共享的 wxPusher：只推「登记过来源」的采集，
		// 后台/API 录入的文章不会被推到微信。
		bot := channelweixin.NewBot(db.GetWeixinRepo(), botRouter, slog.Default(),
			channelweixin.WithPusher(wxPusher))
		go func() {
			err := bot.Run(botCtx)
			switch {
			case err == nil:
			case errors.Is(err, context.Canceled):
				// 正常关闭
			default:
				fmt.Printf("微信 bot 已停止: %v\n", err)
			}
		}()
		fmt.Println("微信 Bot 已启用")
	}

	// 启动 HTTP 服务器
	port, _ := cfg.Get("PORT")
	addr := fmt.Sprintf(":%s", port)

	// WriteTimeout 必须容纳最慢的同步请求：URL 采集 Preview 会走无头浏览器渲染
	// （含全页截图），单页可达 20s+。取「浏览器渲染超时 + 20s 余量」，
	// 渲染关闭时退回 15s 基线，避免慢连接长期占用。
	writeTimeout := 15 * time.Second
	if collectorConfig.RenderEnabled && collectorConfig.BrowserEndpoint != "" {
		writeTimeout = collectorConfig.BrowserTimeout + 20*time.Second
	}
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: writeTimeout,
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
