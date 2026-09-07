# AiKnow 变更日志
## [v0.4.0-rc6] - 2026-09-07

### DOC-7-01：项目文档（README / API / MCP）

**对应任务**：DOC-7-01

**变更**：
- 新增 `README.md`：项目介绍、特性列表、技术栈、快速开始、API 文档、部署方式、开发命令、项目结构
- 新增 `docs/MCP.md`：MCP 协议版本、端点、认证、4 个工具说明（3 readonly + 1 readwrite）、Claude Desktop 配置示例
- 更新 `docs/API.md`：补全认证方式、错误响应、健康检查、认证、文章 CRUD、检索接口文档

**状态**：文档齐全，可按文档一键部署。

---

## [v0.4.0-rc5] - 2026-09-07

### W6：提示词管理 Handler 与 Service（HND-6-04）

**对应任务**：HND-6-04

**变更**：
- 新增 `internal/service/prompt.go`：提示词服务
  - `GetPrompt`：从默认 map 获取提示词模板，key 不存在时返回错误
  - `UpdatePrompt`：校验模板必须包含 `{{.Content}}` 且含 "json" 字样（Qwen JSON 模式要求）
  - `TestPrompt`：返回渲染后的提示词（TODO：后续接入 LLM 调用）
  - 定义 `PromptResult` 结构体用于测试结果返回
- 新增 `internal/handler/admin/prompt.go`：提示词管理 Handler
  - `GetPrompt`：GET `/api/v1/admin/prompts/:key` 获取提示词
  - `UpdatePrompt`：PUT `/api/v1/admin/prompts/:key` 更新提示词
  - `TestPrompt`：POST `/api/v1/admin/prompts/test` 测试提示词渲染

**编译状态**：
- `go build ./...` ✅ 通过
- `go vet ./...` ✅ 通过

---

## [v0.4.0-rc3] - 2026-09-07

### W6：MCP 调试代理 Handler（HND-6-02）

**对应任务**：HND-6-02

**变更**：
- 新增 `internal/handler/admin/mcp_debug.go`：MCP 调试代理 Handler
  - `Invoke` 方法：支持 Authorization Header 和 Session Cookie 双鉴权
  - 读取请求体并解析 JSON，返回请求内容 + 占位响应
  - 预留 TODO 用于后续转发到 MCP 服务

**编译状态**：
- `go build ./...` ✅ 通过

---

## [v0.4.0-rc2] - 2026-09-07

### W6：Admin Handler 实现（HND-6-03/6-05/6-06）

**对应任务**：HND-6-03（setting）、HND-6-05（apikey）、HND-6-06（log）

**变更**：
- 新增 ： 系统配置 Handler
  - ：返回 L2 默认配置（从 model.DefaultL2Config）
  - ：按 key 获取单个配置（TODO）
  - ：更新配置（含参数校验）
  -  / ：恢复默认配置（TODO）
  - ：配置变更历史（TODO）
- 新增 ： MCP 日志 Handler
  - ：日志列表，含分页参数边界修正（Page/PageSize）
  - ：按 trace_id 获取单条日志
  - ：清理过期日志（TODO，预留 before 参数）
- 新增 ： API Key 管理 Handler
  -  /  /  /  / ：API Key CRUD（TODO）
- 修复 ：
  - 修正  包引用（重新导入 hashpwd 包）
  - 修正  多返回值问题（改为 ）
  - 修正  多返回值问题（改为 ）
  - 修正  参数类型（改为先开事务再传入）

**编译状态**：
-  ✅ 通过
-  ✅ 通过

---



## [v0.4.0-rc1] - 2026-09-07

### W5：MCP Server Handler（HND-5-01）

**对应任务**：HND-5-01

**变更**：
- 新增 `internal/handler/mcp/server.go`：MCP Server Handler 实现，支持 JSON-RPC 2.0 协议
  - `Handle` 方法：统一处理 MCP 请求，包含 API Key 鉴权、请求体读取、JSON-RPC 解析
  - `handleRequest`：路由分发，支持 `initialize`、`tools/list`、`tools/call` 三个方法
  - `handleInitialize`：处理 MCP 初始化请求，返回协议版本和服务端信息
  - `handleToolsList`：按 API Key scope 过滤返回可用工具列表
  - `handleToolsCall`：调用对应工具，错误时返回 JSON-RPC 标准错误格式
- 新增 `internal/model/search.go` 中 MCP JSON-RPC 类型：
  - `MCPJSONRPCRequest` / `MCPJSONRPCResponse`：JSON-RPC 2.0 请求/响应结构
  - `MCPError`：JSON-RPC 错误结构（code + message）
  - `MCPCallToolRequest`：tools/call 请求参数结构
- 更新 `internal/handler/web/handler.go`：`NewWebHandler` 支持可选注入 MCP handler，`RegisterRoutes` 注册 `/mcp` 的 GET/POST 路由
- 更新 `internal/service/interfaces.go`：已有 `MCPService` 接口定义（Initialize/ListTools/CallTool）

**编译状态**：
- `go build ./...` ✅ 通过
- `go vet ./...` ✅ 通过

---

## [v0.3.0-rc1] - 2026-09-07

### W3：检索服务层实现（SRC-3-01~3-03）

**对应任务**：SRC-3-01（检索仓库）、SRC-3-02（混合检索服务）、SRC-3-03（关键词检索服务）

**变更**：
- 新增 `internal/model/search.go`：补充 `SearchChunk`（检索分块结果）、`SearchTimings`（各阶段耗时）模型；`SearchResult` 重构为包含 Query/Results/Timings
- 新增 `internal/repository/search_repo.go`：`SearchRepositoryImpl` 完整实现，包含：
  - `VectorSearch`：基于 pgvector ` <-> ` 操作符的余弦距离检索
  - `BM25Search`：基于 PostgreSQL `ts_rank` + `plainto_tsquery` 的全文检索
  - `KeywordSearch`：占位实现，待接入 zhparser 分词
  - `GetArticleByID` / `GetCategories` / `GetTags`：委托现有仓库实现
- 新增 `internal/service/search.go`：`SearchServiceImpl` 完整混合检索实现（向量 + BM25 + RRF + Rerank），包含：
  - `Search` 方法：顺序执行 Embed → VectorSearch → BM25Search → RRF 融合 → Rerank 精排，记录各阶段耗时
  - `SearchWithConcurrent` 辅助方法：并发执行向量与 BM25 检索以提升性能
  - `rrfFuse` 函数：倒数排名融合（Reciprocal Rank Fusion），k=60
- 新增 `internal/service/search_keyword.go`：`KeywordSearchServiceImpl` 关键词检索服务实现（占位，待接入 zhparser）
- 更新 `internal/repository/interfaces.go`：新增 `SearchRepository` 接口（VectorSearch/BM25Search/KeywordSearch/GetArticleByID/GetCategories/GetTags）
- 更新 `internal/service/interfaces.go`：新增 `KeywordSearchService` 接口

**编译状态**：
- `go build ./internal/...` ✅ 通过
- `go vet ./internal/...` ✅ 通过

---

## [v0.2.0-rc1] - 2026-09-07

### W2：仓库层实现（REP-2-01~2-03）

**对应任务**：REP-2-01（文章仓库）、REP-2-02（Chunk 仓库）、REP-2-03（分类/标签仓库）

**变更**：
- 新增 `internal/repository/postgres.go`：`PostgresDB` 数据库封装（pgxpool + Tx 事务 + DB() 底层 sql.DB 适配 pgqueue）
- 新增 `internal/repository/article_repo.go`：`ArticleRepositoryImpl` 完整实现（Create/GetByID/List/Update/SoftDelete/Restore/CountByStatus/FindByURL/FindByContentHash/BulkUpdateStatus/BulkSoftDelete/GetArticleStatus/IncrementViewCount）
- 新增 `internal/repository/chunk_repo.go`：`ChunkRepositoryImpl` 完整实现（Create/BulkCreate/ListByArticle/DeleteByArticle/UpdateEmbedding/CountByArticle/GetTotalCount/GetByID），使用 `pgvector-go` 处理向量类型
- 新增 `internal/repository/category_repo.go`：`CategoryRepositoryImpl` 完整实现（Create/GetByID/GetByName/ListAll/GetOrCreateByName）
- 新增 `internal/repository/tag_repo.go`：`TagRepositoryImpl` 完整实现（Create/GetByID/GetByName/ListAll/BulkCreate/GetOrCreateByName/LinkTagsToArticle）
- 新增 `GenerateUUID()` 辅助函数（基于 `github.com/google/uuid`）
- 更新 `migrations/001_initial_schema.sql`：articles 表新增 `content_hash VARCHAR(64)` 字段及 `idx_articles_content_hash` 部分索引
- 更新 `go.mod`：新增 `github.com/google/uuid`、`github.com/pgvector/pgvector-go`、`github.com/jackc/pgx/v5/stdlib` 依赖

**编译状态**：
- `go build ./internal/repository/...` ✅ 通过
- `go vet ./internal/repository/...` ✅ 通过

---

## [v0.1.0-rc1] - 2026-09-07

### W1 基础框架初始化（里程碑 M1 达成）

**对应任务**：FND-1-01（项目骨架）、FND-1-03（数据库迁移）、FND-1-04（数据模型）、FND-1-05（L1 配置加载）、FND-1-06（接口契约）、AIP-1-01~1-03（AI Provider 接口）、PKG-1-01~1-03（公共库）、WEB-1-01（前端骨架）

**变更**：
- 新增 `go.mod`，Go 1.25 模块，依赖 pgx/v5、godotenv、yaml.v3、crypto
- 新增 `Makefile`，包含 build/run/test/vet/fmt/clean target
- 新增 `cmd/aiknow-server/main.go`，支持 hashpwd/version/help 子命令
- 新增 `cmd/aiknow-server/hashpwd/main.go`，密码哈希生成（占位实现）
- 新增 `migrations/001-005.sql`，完整数据库 Schema：articles、chunks、categories、tags、article_tags、settings、setting_history、api_keys、mcp_request_logs
- 新增 `internal/repository/migrations.go`，迁移执行器（embed + pgx 事务 + schema_migrations 版本管理）
- 新增 `internal/model/` 包：Article、Chunk、Category、Tag、Setting、APIKey、MCPRequestLog、SearchParams 等完整数据模型
- 新增 `internal/config/config.go`，L1 配置加载（环境变量 > .env > YAML > 默认值，来源日志，必填项校验）
- 新增 `internal/repository/interfaces.go`，定义 6 个 Repository 接口（Article/Chunk/Category/Tag/Setting/APIKey/MCPLog）
- 新增 `internal/service/interfaces.go`，定义 3 个 Service 接口（Article/Search/MCP）
- 新增 `internal/service/provider/` 包：LLM/Embedding/Reranker 三接口 + Qwen/BAAI/Mock 实现 + 工厂模式
- 新增 `pkg/ssrf/`：SSRF 防护库（内网 IP 拦截、协议白名单、端口限制），测试 7 项全部通过
- 新增 `pkg/htmlclean/`：HTML 清洗库（XSS 防护），测试 7 项全部通过
- 新增 `pkg/pgqueue/`：PGMQ 队列封装（send/pop/archive/深度查询），测试跳过（需 DB）
- 新增 `web/` 前端骨架：index.html、login.html、app.js（Alpine.js 路由）、main.css（Tailwind）

**编译状态**：
- `go build ./...` ✅ 通过
- `go vet ./...` ✅ 通过
- `go test ./pkg/...` ✅ 通过（ssrf 7 项、htmlclean 7 项）
- `./aiknow-server --help` ✅ 正常
- `./aiknow-server version` ✅ 输出 0.1.0

**代码量**：27 个 Go 文件，2668 行代码

**里程碑 M1 验收状态**：
| # | 验收项 | 状态 |
|---|--------|------|
| 1 | go build 通过，./aiknow-server --help 显示子命令 | ✅ |
| 2 | 数据库迁移 001-005 文件创建完成 | ✅ |
| 3 | pgvector/pg_textsearch/zhparser/PGMQ 扩展 SQL 已就绪 | ✅ |
| 4 | HNSW/BM25/GIN 索引定义在迁移文件中 | ✅ |
| 5 | 环境变量 > .env > config.yaml > 默认值 优先级已实现 | ✅ |
| 6 | .env 不存在时静默跳过 | ✅ |
| 7 | 启动日志打印配置来源 | ✅ |
| 8 | 缺必填项启动失败报错 | ✅ |
| 9 | L2 配置缺键回落内置默认值 | ✅ |
| 10 | LLMProvider/EmbeddingProvider/RerankerProvider 三接口定义 | ✅ |
| 11 | Embedding mock 返回 1024 维 | ✅ |
| 12 | repository/service 接口契约已定义 | ✅ |
| 13 | 前端页面壳可导航 | ✅ |

**下一步**：
- FND-1-02：数据库连接池实现
- AIP-1-04：提示词引擎 + JSON 校验
- W2：仓库层实现（R1-R6）、服务层实现（S1-S4）

---

## [v3.5.2] - 2026-09-07

### 新增 AI Provider 层接口与基础实现（W1 关键任务）

**对应任务**：AIP-1-01~1-03

**变更**：
- 新增 `internal/service/provider/llm.go`：定义 `LLMProvider` 接口（Generate、GenerateJSON、Name）及 `MockLLM` 实现
- 新增 `internal/service/provider/embedding.go`：定义 `EmbeddingProvider` 接口（Embed、EmbedBatch、Name、Dimension）及 `MockEmbedding` 实现
- 新增 `internal/service/provider/reranker.go`：定义 `RerankerProvider` 接口（Rerank、Name）及 `MockReranker` 实现
- 新增 `internal/service/provider/factory.go`：`ProviderFactory` 工厂模式，按配置字符串创建对应 Provider
- 新增 `internal/service/provider/llm_qwen.go`：Qwen LLM Provider 实现，兼容 DashScope ChatGPT 格式 API
- 新增 `internal/service/provider/embedding_baai.go`：BAAI Embedding Provider 实现，兼容 DeepInfra API
- 新增 `internal/service/provider/reranker_baai.go`：BAAI Reranker Provider 实现，兼容 DeepInfra API

**Mock 实现**支持开发/测试无需真实 API Key，默认维度 1024。

---

## [v3.5.1] - 2026-09-07

### 新增公共库包（W1 PKG 任务）

**对应任务**：PKG-1-01（ssrf）、PKG-1-02（htmlclean）、PKG-1-03（pgqueue）

**变更**：
- 新增 `pkg/ssrf/ssrf.go`：SSRF 防护库，支持协议白名单、端口限制、私有 IP 段拦截（含 AWS metadata、RFC 1918 等），提供 `SafeError` 友好错误信息
- 新增 `pkg/ssrf/ssrf_test.go`：覆盖允许/拦截场景及辅助函数（IsBlocked、IsPrivateIPError、IsSSRFResult）
- 新增 `pkg/htmlclean/htmlclean.go`：HTML 清洗器，支持标签移除与常见实体解码（预留 bluemonday 扩展点）
- 新增 `pkg/htmlclean/htmlclean_test.go`：覆盖标签清洗与实体解码
- 新增 `pkg/pgqueue/pgqueue.go`：PGMQ 队列封装（Send/Pop/Archive/Delete/Depth/OldestMessageAge）
- 新增 `pkg/pgqueue/pgqueue_test.go`：跳过依赖数据库的集成测试

---

## [v3.5.0] - 2026-09-07

### W1 关键路径：数据模型与配置加载骨架

**对应任务**：FND-1-04（数据模型）、FND-1-05（L1 配置加载）、FND-1-06（repo/service 接口）、WEB-1-01（前端骨架）

**变更**：
- 新增 `internal/model/` 包，定义完整数据模型：Article、Chunk、Category、Tag、Setting、APIKey、MCPRequestLog 及检索/MCP 相关 DTO
- 新增 `internal/config/config.go`，实现三层配置加载（默认值 < .env < YAML < 环境变量），带来源追踪与必填项校验
- 新增 `internal/repository/interfaces.go`，定义 6 个 Repository 接口契约（Article/Chunk/Category/Tag/Setting/APIKey/MCPLog）
- 新增 `internal/service/interfaces.go`，定义 3 个 Service 接口契约（Article/Search/MCP）
- 新增 `cmd/aiknow-server/main.go`，子命令模式（hashpwd/version/help），占位实现
- 新增 `web/` 前端骨架：SPA 主页面、登录页、Alpine.js 路由、Tailwind CSS 样式
- 新增 `internal/model/search.go`，定义检索参数与结果结构（SearchParams、SearchResult 等占位类型）

**依赖**：`github.com/joho/godotenv`、`gopkg.in/yaml.v3`

---

## [v3.4.1] - 2026-09-07

### 移除关键词检索的「标签部分匹配」档

**评审意见**：标签本来就是一个词语，只能精确命中，不存在「部分匹配」。

**问题分析**：v3.4 设计的 6 档权重中，「标签部分匹配」（权重 4.0，`tags.name LIKE '%kw%'`）是错误的设计：

1. **假阳性且权重过高**。`Java` 会命中标签 `JavaScript`，`云` 会命中 `云原生`、`阿里云`、`云计算`、`私有云`——几乎等于不筛选。而 4.0 的权重比正文命中（1.0）高 4 倍，一个误命中就能把无关文章推到前面。

2. **与已有信号重复**。标签词通常也出现在标题或正文中，而标题/摘要/正文三档走的是 zhparser 分词的全文检索，已经覆盖了这类关联。部分匹配不提供新信息，只是给假阳性开了一个高权重通道。

**变更**：
- 权重档从 6 档减为 5 档，删除 `weight_tag_partial`
- 标签与分类**只做精确匹配**（`lower(name)` 相等）
- 删除 `pg_trgm` 扩展依赖和 `idx_tags_name_trgm` 索引

**最终权重表**：

| 命中位置 | 权重 | 匹配方式 |
|---------|------|---------|
| 标签精确 | 10.0 | `lower(tags.name)` 完全相等 |
| 分类精确 | 8.0 | `lower(categories.name)` 完全相等 |
| 标题 | 3.0 | 全文检索（zhparser） |
| 摘要 | 1.5 | 全文检索（zhparser） |
| 正文 | 1.0 | BM25 归一化后乘权重 |

**连带简化**：去掉 `pg_trgm` 后，关键词检索所需的额外索引减少到三个（标签/分类的 `lower(name)`、摘要 GIN、`article_tags.tag_id`），部署依赖也少了一个扩展。

**影响范围**：PRD 5.2.5 评分模型、4.2.3 索引、7.3.3 L2 配置表、5.3.5 调试接口、2.5.5 检索测试页

---

## [v3.4] - 2026-09-07

### MCP 工具扩展：2 个 → 4 个

| Tool | 功能 | scope | 状态 |
|------|------|-------|------|
| `search_knowledge` | 语义检索，返回 chunk 证据块 | readonly | 已有 |
| `find_articles_by_keywords` | 关键词检索，返回文章卡片 | readonly | **新增** |
| `get_article_detail` | 获取文章全文 | readonly | 已有 |
| `add_article_by_url` | 提交 URL 采集入库 | readwrite | **新增**（原 P2 提前至 v1） |

#### 1. 新增 `find_articles_by_keywords`

输入一个或多个关键词，返回最相关的 Top N 文章卡片（标题、摘要、分类、标签、链接）。

**与 `search_knowledge` 的分工**（两者返回粒度不同，非重复功能）：

| | `search_knowledge` | `find_articles_by_keywords` |
|---|---|---|
| 输入 | 自然语言问句 | 关键词数组 |
| 返回粒度 | Chunk 片段 | 整篇文章卡片 |
| 检索方式 | 向量 + BM25 + RRF + Rerank | 标签/分类精确匹配 + 全文检索加权 |
| 用途 | RAG 证据供给 | 文章发现、主题浏览 |

工具 `description` 中写明该区别，引导调用方大模型正确选择。

**评分模型**（标签/分类命中权重高于正文——用户给关键词时期望的是「打了这个标签的文章」，而非「正文恰好提到该词的文章」）：

| 命中位置 | 权重 | 匹配方式 |
|---------|------|---------|
| 标签精确匹配 | 10.0 | `tags.name` 完全相等（忽略大小写） |
| 分类精确匹配 | 8.0 | `categories.name` 完全相等（忽略大小写） |
| 标题命中 | 3.0 | 标题全文检索 |
| 摘要命中 | 1.5 | 摘要全文检索 |
| 正文命中 | 1.0 | BM25 分数归一化后乘权重 |

> 标签与分类**只做精确匹配**，不做 substring 匹配。理由见 v3.4.1 条目。

- 最终得分 = Σ(每个关键词的各项命中权重)
- 多关键词不做平均：命中数量本身是信号，命中 3 个关键词的文章自然高于命中 1 个的
- `match_mode`：`any`（默认）/ `all`（先过滤未命中全部关键词的文章再排序）
- 5 项权重放入 L2 配置 `keyword_search`，后台可调

**参数**：`keywords`（必填，1-10 个）、`limit`（默认 5，1-20）、`match_mode`（默认 `any`）

**返回**：`content` 为文章卡片 resource；`annotations.articles` 提供结构化字段，含 `matched` 说明每个关键词的命中方式（让大模型判断相关性强度——「标签精确匹配」比「正文提及」是更强信号）。

**无结果时** `isError` 为 `false`——检索无结果是正常业务情况，不是错误。

#### 2. 新增 `add_article_by_url`（v1 唯一写入工具）

原计划 P2，本次提前至 v1。行为与 `POST /api/v1/articles` 完全一致，复用同一套采集入库逻辑（SSRF 防护、去重、异步处理），立即返回不等待 AI 处理。

**参数**：`url`（必填）、`category_name`（可选）、`tags`（可选，与 AI 生成的合并去重）

**关键设计决策**：

- **去重命中不算错误**：返回 `isError: false` + 已存在提示。若返回 `true`，大模型会误以为操作失败而重试。
- **SSRF 拦截错误信息统一措辞**，不回显 DNS 解析结果或具体拦截原因。否则该工具会变成内网探测器——调用方可通过错误信息差异推断内网哪些 IP 存活。
- **写限流与 REST API 共享计数器**（按 API Key 维度，默认 20 req/min），避免绕过限流。

#### 3. `tools/list` 按 scope 过滤

`readonly` Key 调用 `tools/list` 只返回 3 个只读工具，看不到 `add_article_by_url`。

**理由**：让只读客户端根本看不到写工具，比让它看到却调用失败更合理——大模型不会反复尝试注定 403 的工具，也避免在对话中暴露「存在但你不能用」的能力。

调用层面仍然强制校验 scope，返回 403。

#### 4. 新增 REST 接口 `/api/v1/search/keywords`

与 MCP 的 `find_articles_by_keywords` 共用同一套检索逻辑，仅返回格式不同（REST 返回原始 JSON，MCP 包装为 `content` + `annotations`）。

#### 5. 新增数据库索引

```sql
-- 标签/分类大小写不敏感精确匹配
CREATE INDEX idx_tags_name_lower ON tags (lower(name));
CREATE INDEX idx_categories_name_lower ON categories (lower(name));

-- 摘要全文检索（标题已有索引）
CREATE INDEX idx_articles_summary_gin ON articles
USING gin (to_tsvector('chinese', coalesce(summary, '')));

-- 反向查标签
CREATE INDEX idx_article_tags_tag_id ON article_tags (tag_id);
```

#### 6. 新增 L2 配置组 `keyword_search`

| 字段 | 默认值 | 范围 |
|------|-------|------|
| `default_limit` | 5 | 1-20 |
| `weight_tag_exact` | 10.0 | 0-100 |
| `weight_category_exact` | 8.0 | 0-100 |
| `weight_title` | 3.0 | 0-100 |
| `weight_summary` | 1.5 | 0-100 |
| `weight_content` | 1.0 | 0-100 |
| `max_keywords` | 10 | 1-20 |

#### 7. 后台检索测试页增加关键词检索 Tab

- 多值关键词输入，`match_mode`、`limit` 可调
- 5 项权重可**临时覆盖**（不写入配置），用于对比不同权重下的排序
- 每篇文章展开显示得分构成：各关键词在各位置的命中与贡献分值
- 新增调试接口 `POST /api/v1/admin/search/keywords/debug`，支持 `weights_override`

> `contribution` 与 `weight` 可能不等：正文命中的贡献值是 BM25 归一化后乘权重，其余位置命中即贡献满额。

#### 8. 项目结构

新增 `internal/service/search_keyword.go`。

#### 9. 验收标准

新增 2 组：
- 10.5 关键词检索验收（12 条）
- 10.6 MCP 写入工具验收（9 条）

10.4 MCP 验收扩充至 11 条（覆盖 4 个工具 + scope 过滤），后续章节编号顺延至 10.12。

### 工期

保持 7 周。关键词检索（Phase 3）和 MCP 写入工具（Phase 5）预计各增 1.5-2 天，两阶段本就相对宽松。若延期，优先将 `match_mode=all` 和 `matched` 字段降为 P1。

---

## [v3.3] - 2026-09-07

### 需求优化（评审意见处理）

#### 1. AI 处理合并为单次 LLM 调用

**问题**：原方案摘要、分类、标签分三次调用 LLM，同一篇正文被输入三次，token 成本和处理耗时都接近三倍。

**变更**：
- 合并为一次调用，用 `response_format: {"type": "json_object"}` 约束返回结构
- 三个提示词配置项 `prompt.summary` / `prompt.category` / `prompt.tags` 合并为 `prompt.analyze`
- 返回 JSON 含 `summary`、`category`、`tags` 三个字段

**收益**：输入 token 降至约 1/3；三项结果出自同一次推理，一致性更好。

**Qwen JSON 模式注意事项**：使用 `json_object` 时提示词中必须出现 "json" 字样，否则 API 报错。模板保存时校验这一点。

**结果校验分级**（只有结构性错误才重试，避免因内容越界重跑整次调用）：

| 校验项 | 规则 | 失败处理 |
|--------|------|---------|
| JSON 可解析 | 能反序列化 | 重试 |
| 字段完整 | 三字段均存在 | 重试 |
| 类型正确 | summary 为串，tags 为串数组 | 重试 |
| 摘要长度 | 50-500 字 | 截断，不重试 |
| 分类有效 | 在候选列表内或「未分类」 | 置「未分类」，不重试 |
| 标签数量 | 1-15 个 | 截取前 10，不重试 |
| 标签长度 | 单个 ≤ 20 字 | 丢弃超长项，不重试 |

**影响范围**：`internal/service/llm.go`、`prompt.go`；`settings` 表配置键；工作流程图

#### 2. 移除 admins 表与角色体系

**问题**：v3.2 设计了 `admins` 表（含 role、failed_attempts、locked_until），却没有用户管理和密码修改页面，表的存在意义不成立。MVP 阶段只有单一管理员，`admin` / `superadmin` 的角色区分也是空转。

**变更**：
- 删除 `admins` 表
- 管理员账号改由 L1 配置提供：`ADMIN_USERNAME` + `ADMIN_PASSWORD_HASH`
- 登录失败锁定状态改为内存维护
- 后台 API 移除所有 `superadmin` 权限标注，API Key 管理页不再做角色判断
- 新增 `aiknow-server hashpwd` 子命令用于生成 bcrypt 哈希

**密码配置策略**：
- 生产环境只提供 `ADMIN_PASSWORD_HASH`
- `ADMIN_PASSWORD`（明文）为开发便利项，启动时在内存哈希，不落盘不写日志
- 两者都缺失时**启动失败并报错**，不再生成随机密码——随机密码只打印一次的做法在容器重启场景下容易丢失

**留待 v2**：多用户、角色区分、密码自助修改。

**影响范围**：删除 `internal/repository/admin_repo.go`；`internal/service/auth.go` 改为读配置

#### 3. audit_logs 替换为 mcp_request_logs

**问题**：通用审计日志在单管理员场景下信息量有限，而真正需要回溯的是 MCP 调用情况（排查「Claude 那边为什么没搜到」）。

**变更**：删除 `audit_logs` 表，新增 `mcp_request_logs` 表。

**新表核心字段**：
| 字段 | 用途 |
|------|------|
| `trace_id` | 优先取请求头 `X-Trace-Id`（不强制），未传则服务端生成 UUID；响应头回写，便于关联调用方日志 |
| `request_body` / `response_body` | 完整 JSON-RPC，支持一键重放 |
| `result_summary` | 检索类调用的命中 chunk 与分数，回答「为什么这条没被搜到」 |
| `is_error` | 覆盖 JSON-RPC 层 `isError=true` 和 HTTP 层非 2xx |
| `elapsed_ms` | 耗时 |

**新增页面** `/ui/mcp-logs`（替代原 `/ui/logs`）：
- 按时间/方法/工具名/调用方/错误状态/耗时区间筛选
- 按请求参数内容全文检索（GIN 索引）
- trace_id 精确查询
- 一键重放：跳转 MCP 调试页并回填参数
- 检索结果回溯

**写入方式**：响应返回后异步写入，不阻塞 MCP 主链路；写入失败仅记服务端日志。

**保留策略**：默认 30 天，`pg_cron` 每日清理，天数由 `mcp_log.retention_days` 配置。

**删除追溯的补偿方案**：换掉通用审计日志后，「谁删了这篇文章」会失去记录。因此在 `articles` 表补 `deleted_at` / `deleted_by` 两个字段——本来就是软删除，成本极低，避免删除操作完全无迹可循。所有列表查询默认附加 `WHERE deleted_at IS NULL`，并加部分索引 `idx_articles_not_deleted`。

**影响范围**：新增 `internal/service/mcplog.go`、`repository/mcplog_repo.go`、`model/mcplog.go`、`web/js/pages/mcp-logs.js`；删除 audit 相关文件

#### 4. 配置加载优先级明确

**变更**：L1 配置来源改为四级，先命中者生效：

```
1. 环境变量        （最高，适合容器/CI 注入）
2. .env 文件       （本地开发，不提交版本库）
3. config.yaml     （默认值与非敏感配置，可提交）
4. 代码内置默认值   （兜底）
```

**实现要点**：
- 用 `godotenv` 加载 `.env`，其默认行为是**不覆盖已存在的环境变量**，与上述优先级天然一致
- `.env` 不存在时静默跳过，不报错（生产通常只用环境变量）
- `.env` 写入 `.gitignore`，仓库只提供 `.env.example`
- 启动后打印各关键配置的**来源**（如 `DATABASE_URL from: env`），但不打印值

**新增文件**：`.env.example`、`.gitignore`

**新增环境变量**：`ADMIN_PASSWORD_HASH`（必填）、`ENV_FILE`（可选，默认 `./.env`）

**影响范围**：`internal/config/config.go`

### 连带调整

- `settings` 表移除 `updated_by` 字段，`setting_history` 移除 `changed_by`（单管理员下无意义）
- `api_keys` 表移除 `created_by` 字段
- 新增配置项 `mcp_log.retention_days`（默认 30，范围 1-365）
- 后台页面从 10 个调整为 9 个（移除审计日志页，新增 MCP 请求日志页）
- 迁移文件重命名：`005_admin_settings.sql` → `005_settings_apikeys_mcplogs.sql`
- 验收标准新增 10.8（MCP 请求日志，11 条）、10.10（配置加载，7 条），10.2 补充 4 条 AI 处理项
- 修正架构图中遗留的「768 维」

### 路线图

保持 7 周不变。v3.3 移除管理员管理相关工作量，新增 MCP 日志页，两者大致抵消。

---

## [v3.2] - 2026-09-07

### 需求完善

#### 1. Web 管理后台功能细化（新增 PRD 2.5 节）

明确后台共 10 个页面，采用左侧菜单 + 右侧内容区的单页应用布局，使用 Session Cookie 鉴权（区别于对外 API 的 API Key 鉴权）。

| 页面 | 路径 | 核心内容 |
|------|------|---------|
| 登录页 | `/ui/login` | 用户名密码登录，失败次数限制 |
| 仪表盘 | `/ui/dashboard` | 统计卡片、处理状态分布、队列状态、最近活动、服务健康 |
| 文章列表 | `/ui/articles` | 筛选、搜索、排序、行内操作、批量操作 |
| 文章编辑 | `/ui/articles/:id` | 正文编辑、AI 结果修正、处理状态面板、Chunk 预览 |
| 文章录入 | `/ui/articles/new` | 手动录入 / URL 采集 |
| 检索测试 | `/ui/search` | 分阶段结果、耗时明细、分词结果 |
| MCP 调试 | `/ui/mcp-debug` | 参数输入、原始请求响应、SSE、鉴权切换 |
| 系统配置 | `/ui/settings` | 模型参数、提示词、检索参数、采集参数 |
| API Key 管理 | `/ui/settings/api-keys` | 创建、禁用、删除（仅 superadmin） |
| 审计日志 | `/ui/logs` | 查询与筛选 |

**关键行为约定**：
- 修改正文保存 → `processing_status` 重置为 `pending` 并重新入队
- 仅修改 AI 结果（摘要/分类/标签）→ 不触发重新处理

#### 2. MCP 调试页面（新增）

新增 `/ui/mcp-debug` 页面，无需依赖 Claude Desktop 等外部客户端即可验证 MCP 接口。

**核心能力**：
- 按 `inputSchema` 动态渲染参数表单，支持表单/JSON 双模式
- 展示完整 JSON-RPC 请求体与原始响应
- `content` 中 resource 类型渲染为可跳转卡片
- 支持 `initialize`、`tools/list`、`tools/call` 三种操作
- 支持 SSE 模式，逐条追加显示事件流
- 可切换用 Session 或指定 API Key 调用（验证 scope 与 401/403 行为）
- 展示 `MCP-Protocol-Version`、`Mcp-Session-Id` 响应头

**设计决策**：页面通过后端 `/api/v1/admin/mcp/invoke` 代理接口转发，而非浏览器直连 `/mcp`。
- **原因 1**：避免在前端暴露 API Key
- **原因 2**：绕开浏览器对 MCP 端点 `Origin` 校验的限制

#### 3. 检索测试页面（新增）

新增 `/ui/search` 页面，用于调优 RRF 权重和 `ef_search`。

- 三栏展示向量召回、BM25 召回、RRF 融合候选
- 精排结果标注每条在 RRF 阶段的原始排名变化
- 各阶段耗时明细（embedding / vector_recall / bm25_recall / rrf / rerank / context_fill）
- 展示 zhparser 对 Query 的分词结果，便于排查中文检索问题

#### 4. 配置分层重构（重要变更）

将原本集中在 `config.yaml` 的参数按性质拆为三层：

| 层级 | 存储位置 | 内容 | 变更方式 |
|------|---------|------|---------|
| **L1 启动配置** | 环境变量 / `config.yaml` | 连接串、密钥、端口、连接池、provider、base_url | 改后需重启 |
| **L2 业务配置** | 数据库 `settings` 表 | 模型参数、提示词、检索参数、采集参数 | 后台修改，热生效 |
| **L3 凭据数据** | `api_keys`、`admins` 表 | API Key 哈希、管理员账号 | 后台管理 |

**分层原则**：
- 密钥类只走环境变量，不落到 `settings` 表或后台页面，避免明文凭据在数据库和 UI 中扩散
- 模型名、温度、提示词属于业务调优范畴，需频繁试错，放数据库
- `provider`、`base_url` 归 L1：切换服务商属部署级变更，通常伴随密钥更换
- `ai.embedding.dimension` 固定 1024 且归 L1：与 `chunks.embedding VECTOR(1024)` 表结构强绑定

**热生效机制**：
```
启动：环境变量 > config.yaml > 内置默认值 → L1（进程内不可变）
     settings 表 → 内存 ConfigCache
运行：后台保存 → 写表 + 写 history → 刷新 ConfigCache
     多实例：PostgreSQL LISTEN/NOTIFY 广播，各实例重载
```

`settings` 表缺失的键回落到内置默认值，首次部署无需灌数据；「恢复默认」即删除对应行。

#### 5. 提示词配置化

三类提示词（摘要、分类、标签）移至后台可编辑，使用 Go `text/template` 语法。

**可用变量**：
- `{{.Title}}` — 文章标题（全部提示词）
- `{{.Content}}` — 纯文本内容，超长截断至前 8000 字（全部提示词）
- `{{.Categories}}` — 当前全部分类名，逗号分隔（仅分类提示词）

**校验规则**：保存时校验模板可编译，且包含必需变量（缺少 `{{.Content}}` 拒绝保存）。

**试跑功能**：可选一篇已有文章，用未保存的模板调用 LLM，展示渲染后 Prompt 与模型返回，不写入文章数据。

#### 6. 新增数据表

| 表名 | 用途 |
|------|------|
| `settings` | L2 业务配置，key-value + JSONB |
| `setting_history` | 配置变更记录（操作者、时间、旧值、新值） |
| `api_keys` | API Key 管理，从 config.yaml 迁移至数据库 |
| `admins` | 管理员账号，含锁定状态与角色 |
| `audit_logs` | 审计日志 |

**API Key 存储决策**：仅存 SHA-256 哈希 + 前缀（用于列表展示），不存明文。
- 选 SHA-256 而非 bcrypt 的原因：API Key 是高熵随机串，不存在字典攻击风险；且每次请求都要验证，bcrypt 计算开销不可接受

对应新增迁移文件 `migrations/005_admin_settings.sql`。

#### 7. 新增后台管理 API（PRD 5.3 节）

前缀 `/api/v1/admin`，使用 Session Cookie 鉴权（不接受 API Key），要求 CSRF Token。共 19 个接口，其中关键接口：

- `POST /api/v1/admin/mcp/invoke` — MCP 调试代理，返回体同时含 `request` 和 `response`，便于复制到其他客户端复现
- `POST /api/v1/admin/search/debug` — 检索调试，返回 `query_tokens`、`stages`、`timings_ms`
- `PUT /api/v1/admin/settings/:key` — 配置更新，校验失败返回 400 并指明具体字段
- `POST /api/v1/admin/prompts/test` — 提示词试跑

#### 8. 项目结构调整

- 新增 `internal/handler/admin/` 包（10 个文件），与 `handler/api/` 分离，因两者鉴权方式不同
- 新增 `internal/service/` 下 `prompt.go`、`setting.go`、`auth.go`、`audit.go`
- 新增 `internal/repository/` 下 `setting_repo.go`、`apikey_repo.go`、`admin_repo.go`、`audit_repo.go`
- `internal/config/` 拆为 `config.go`（L1）、`defaults.go`（L2 默认值）、`cache.go`（缓存 + 热重载）
- `web/js/pages/` 按页面拆分为 8 个 JS 文件

#### 9. 环境变量调整

新增必填项 `SESSION_SECRET`（后台 Session 签名密钥，32 字节以上随机串）。

新增可选项 `ADMIN_USERNAME`、`CONFIG_PATH`。

**首次启动逻辑**：`admins` 表为空时用 `ADMIN_USERNAME`/`ADMIN_PASSWORD` 创建 superadmin；未设 `ADMIN_PASSWORD` 则生成随机密码打印到启动日志（仅一次）。

#### 10. 验收标准补充

新增 4 组共 39 条验收项：
- 10.6 后台页面验收（11 条）
- 10.7 MCP 调试页面验收（10 条）
- 10.8 系统配置验收（9 条）
- 10.5 安全验收补充 3 条（API Key 哈希存储、401、403）

#### 11. 开发路线图调整：6 周 → 7 周

原 Phase 4 将「Reranker + 完整 Web 后台」压在同一周，细化后后台含 10 个页面（其中 MCP 调试、检索调试、系统配置三个交互较重），拆分为：
- **Phase 4**：精排 + 后台基础页（登录/仪表盘/文章列表/文章编辑）
- **Phase 6**：调试与配置页（检索测试/MCP 调试/系统配置/API Key/审计日志）

### 其他修正

- 修正术语表中遗留的「768 维」为「1024 维」（v3.1 遗漏项）

---

## [v3.1] - 2026-09-07

### 技术方案调整

#### 1. 向量维度调整
- **变更**：向量维度从 768 维调整为 1024 维
- **原因**：采用更大的模型 `bge-large-zh-v1.5` 以提升检索准确率
- **影响范围**：
  - `chunks.embedding` 字段：`VECTOR(768)` → `VECTOR(1024)`
  - `articles.embedding_dim` 默认值：768 → 1024
  - 配置文件中的模型：`bge-base-zh-v1.5` → `bge-large-zh-v1.5`

#### 2. API 接口规范明确
- **变更**：明确文章录入时分类和标签的创建逻辑
- **新增规范**：
  - `category_id` 和 `category_name` 二选一，优先使用 `category_id`
  - 分类通过 `category_name` 指定但不存在时，自动创建新分类
  - 标签不存在时自动创建，重复标签自动去重
- **影响范围**：`POST /api/v1/articles` 接口实现

#### 3. RRF 参数补充
- **新增配置参数**：
  - `rrf_k: 60`（RRF 倒数排名融合参数，标准值）
  - `vector_weight: 0.5`（向量检索权重）
  - `bm25_weight: 0.5`（BM25 检索权重）
- **影响范围**：
  - `POST /api/v1/search` 接口增加 `rrf_k` 参数
  - `config.yaml` 增加 RRF 相关配置
  - 检索服务实现需支持可配置的权重

#### 4. 中文分词配置完善
- **变更**：扩展中文分词词性映射
- **原配置**：`n,v,a,i,e,l`（名词、动词、形容词、成语、叹词、习用语）
- **新配置**：`n,v,a,i,e,l,j,r,d,m,q,t,z`
  - 新增：`j`(简称)、`r`(代词)、`d`(副词)、`m`(数词)、`q`(量词)、`t`(时间词)、`z`(状态词)
- **SQL 变更**：
  ```sql
  ALTER TEXT SEARCH CONFIGURATION chinese 
  ADD MAPPING FOR n,v,a,i,e,l,j,r,d,m,q,t,z WITH simple;
  ```
- **影响范围**：`migrations/003_pg_textsearch_bm25.sql`

#### 5. MCP 返回格式优化
- **变更**：`search_knowledge` 工具返回格式改为结构化 resource
- **原格式**：拼接字符串格式
  ```json
  {
    "content": [
      {"type": "text", "text": "找到以下相关文档片段：\n\n1. [标题]..."}
    ]
  }
  ```
- **新格式**：结构化 resource 格式
  ```json
  {
    "content": [
      {"type": "text", "text": "找到 2 个相关文档片段"},
      {
        "type": "resource",
        "resource": {
          "uri": "aiknow://article/uuid/chunk/index",
          "name": "标题 - 第X节",
          "description": "内容片段...",
          "mimeType": "text/plain"
        }
      }
    ]
  }
  ```
- **优势**：
  - 结构化数据便于大模型解析
  - 支持 MCP 资源引用协议
  - 便于追踪和调试
- **影响范围**：`internal/handler/mcp/server.go` 中的 `search_knowledge` 实现

#### 6. 配置参数补充
- **新增附录 C 参数**：
  - `RRF k: 60`（倒数排名融合参数）
  - `向量权重: 0.5`
  - `BM25 权重: 0.5`
  - `向量维度: 1024`
  - `中文分词词性: n,v,a,i,e,l,j,r,d,m,q,t,z`（含词性说明）

### 评审意见处理
- ✅ 确认 `pg_textsearch` 为 Timescale 官方扩展，已被主流云厂商支持
- ✅ 采纳向量维度调整建议（1024 维）
- ✅ 采纳 API 接口规范明确建议
- ✅ 采纳 RRF 参数补充建议
- ✅ 采纳中文分词配置完善建议
- ✅ 采纳 MCP 返回格式优化建议
- ⏭️ 性能指标、部署方案、开发路线图保持原方案

---

## [v3.0] - 2026-09-07

### 初始评审版本
- 统一命名为 AiKnow
- 修正数据模型（category_id + article_tags）
- 明确 pg_textsearch + zhparser
- 缩减 v1 范围
- 补充 MCP 规范细节
- 补充安全需求
- 调整性能指标
