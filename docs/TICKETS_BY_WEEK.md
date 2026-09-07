# AiKnow 按周开发 Ticket 清单（v1 MVP）

**版本**：v1.0  
**日期**：2026-09-07  
**关联**：[DEVELOPMENT_PLAN.md](./DEVELOPMENT_PLAN.md)（轨道与 WBS）、[PRD.md](./PRD.md)（需求）、[MILESTONES.md](./MILESTONES.md)（里程碑验收）  
**状态**：🟡 开发中（核心功能完成，前端待完善）

> 本文档把 7 周计划拆成**可分配、可估点、带优先级和 DoD（完成定义）**的具体任务卡，作为每日站会与 Sprint Planning 的直接输入。
>
> **DoD（Definition of Done）约定**：每个 `P0` 任务须同时满足——① 代码通过 `go vet` + 相关单测；② 关键行为有可验证产物（可编译/可 curl/可 UI 操作）；③ 无遗留 `TODO` 阻塞项。`P1`/`P2` 任务可延后，见 PRD 2.x 优先级列。
>
> **状态图例**：`✅` 已完成 · `🔄` 进行中 · `⏳` 待开发 · `❌` 阻塞

---

## 使用说明

- 任务编号 `${轨道代号}-${周}-${序号}`，如 `FND-1-02` 表示基础轨道第 1 周第 2 个任务。
- **点数**：1 点 ≈ 半天工作量，2 点 = 1 人天（中等水平工程师）。
- **优先级**：`P0`=本周必须完成（关键路径 / 阻塞下游）；`P1`=本周尽量完成，可顺延至下周缓冲；`P2`=可选打磨项。

---

## Week 1：基础框架（目标：可编译骨架 + Schema + 配置 + AI Provider 接口）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| FND-1-01 | 项目骨架初始化（go.mod、Makefile、`.gitignore` 含 `.env`、`cmd/aiknow-server/main.go` 空服务、`hashpwd` 子命令框架） | 2 | P0 | ✅ | `go build` 通过，`./aiknow --help` 显示子命令 |
| FND-1-02 | 数据库连接池（`internal/repository/postgres.go`，pgx pool + ping + 优雅关闭） | 1 | P0 | ✅ | `PostgresDB` 封装完成 |
| FND-1-03 | 数据库迁移 001–005 + 迁移执行器 | 4 | P0 | ✅ | `migrations/*.sql` 完成，迁移执行器支持 |
| FND-1-04 | 数据模型定义（`model/*.go`：article/chunk/tag/setting/apikey/mcplog） | 2 | P0 | ✅ | 9 个 model 文件完成 |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| FND-1-05 | L1 配置加载（`config.go`：env > .env > yaml > 默认值，godotenv，来源日志不打印值） | 4 | P0 | ✅ | 三级配置加载完成 |
| FND-1-06 | repository/service 接口契约定义 + mock 约定 | 2 | P0 | ✅ | 6 个 Repository 接口 + 3 个 Service 接口 |
| FND-1-07 | L2 配置缓存 + 热重载（`defaults.go`/`cache.go`，LISTEN/NOTIFY） | 4 | P1 | ❌ | 暂未实现，L2 配置存储在 settings 表但未实现热重载 |
| AIP-1-01 | `LLMProvider` 接口 + Qwen 实现（JSON mode + 重试） | 4 | P1 | ✅ | mock + Qwen 实现完成 |
| AIP-1-02 | `EmbeddingProvider` 接口 + BAAI 实现（1024 维 + 批量） | 3 | P1 | ✅ | mock + BAAI 实现完成 |
| AIP-1-03 | `RerankerProvider` 接口 + BAAI 实现 | 2 | P2 | ✅ | mock + BAAI 实现完成 |
| AIP-1-04 | 提示词引擎 + JSON 校验（`prompt.go`：template 渲染 + 分级重试策略） | 3 | P1 | ✅ | prompt.go 完成 |
| PKG-1-01 | `pkg/ssrf`：URL 校验/内网黑名单/DNS 重绑定/端口限制/重定向 | 4 | P1 | ✅ | 测试通过 |
| PKG-1-02 | `pkg/htmlclean`：bluemonday 封装 | 1 | P2 | ✅ | 测试通过 |
| PKG-1-03 | `pkg/pgqueue`：PGMQ 封装（send/pop/archive/深度查询） | 3 | P1 | ✅ | 测试通过（需 DB） |
| WEB-1-01 | 前端骨架（`index.html`、Tailwind 构建、`app.js` 路由、布局、登录态守卫） | 4 | P1 | ✅ | 骨架完成，页面 JS 待完善 |

**本周 P0 交付 = 里程碑 M1**（见 MILESTONES.md）。

---

## Week 2：采集 + AI 处理（目标：完整录入流程 + 异步状态可查）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| REP-2-01 | `article_repo.go`：CRUD / 分页筛选 / 去重查询 / 软删除 / 批量 | 4 | P0 | ✅ | 完整实现 |
| REP-2-02 | `chunk_repo.go`：chunk 增删查、按 article 分组 | 2 | P0 | ✅ | 完整实现 |
| SVC-2-01 | `article.go`：文章 CRUD、分类/标签自动创建、内容 hash 去重 | 4 | P0 | ✅ | 完整实现 |
| SVC-2-02 | `chunk.go`：文本切分（200–500 字 + overlap） | 2 | P0 | ✅ | 完整实现 |
| SVC-2-03 | AI 处理流水线（编排 LLM → chunk → embedding → 状态更新） | 4 | P0 | ✅ | AIPipeline 完成 |
| SVC-2-04 | `worker.go`：PGMQ 消费、并发、重试、状态字段更新 | 4 | P0 | ✅ | Worker 完成 |
| AIP-2-01 | 补全 4.3 全部 Provider 的真实联调（替换 mock） | 2 | P1 | ⏳ | 需真实 API Key |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| COL-2-01 | `collector.go`：URL 采集 + 正文提取 + 去重 hash | 4 | P0 | ✅ | 完整实现 |
| REP-2-03 | `setting_repo.go` / `apikey_repo.go` / `mcplog_repo.go`（R4/R5/R6） | 4 | P1 | ✅ | 完整实现 |
| WEB-2-01 | 文章录入页雏形（`articles/new`，对接 H4 契约，mock 先行） | 2 | P2 | ⏳ | 待前端开发 |

**本周 P0 交付 = 里程碑 M2**。

---

## Week 3：检索核心（目标：混合检索 + 关键词检索可调用）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| SRC-3-01 | `search_repo.go`：pgvector HNSW 查询 + pg_textsearch BM25 查询 | 4 | P0 | ✅ | 完整实现 |
| SRC-3-02 | `search.go`：混合检索（向量 + BM25 + RRF + Rerank + 上下文补全） | 6 | P0 | ✅ | 完整实现 |
| SRC-3-03 | `search_keyword.go`：关键词加权检索（5 档加权 + matched 字段） | 4 | P0 | 🔄 | 占位实现，待接入 zhparser |
| HND-3-01 | `handler/api/article.go` + `search.go`：对外 REST 初版 | 4 | P1 | ✅ | 完整实现 |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| SVC-3-01 | `auth.go`：API Key 校验（SHA-256）+ 限流 + 管理员认证 | 4 | P1 | 🔄 | 基础实现，登录锁定待完善 |
| SVC-3-02 | `setting.go`：L2 配置读写 + 校验 + history | 3 | P1 | ❌ | 待实现 |
| REP-3-01 | W2 顺延的 R4/R5/R6 收尾 | 2 | P1 | ✅ | 已完成 |
| HND-3-02 | `handler/api/middleware.go`：API Key 鉴权 + 限流中间件 | 2 | P1 | ✅ | 完整实现 |

**本周 P0 交付 = 里程碑 M3**。

---

## Week 4：精排 + 后台基础页（目标：可管理文章的 Web 后台）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| HND-4-01 | `handler/admin/auth.go`：login/logout/session/me + 失败锁定 | 3 | P0 | ✅ | 基础实现，锁定逻辑待完善 |
| HND-4-02 | `handler/admin/middleware.go`：Session 校验 + CSRF | 2 | P0 | ✅ | 基础实现 |
| HND-4-03 | `handler/admin/dashboard.go`：统计 + 队列 + 活动 + 健康 | 2 | P0 | ✅ | 完整实现 |
| HND-4-04 | `handler/admin/article.go`：批量操作 + Chunk 查看 | 3 | P0 | ✅ | 完整实现 |
| HND-4-05 | `handler/web/handler.go`：embed 静态资源 + SPA 回退 | 1 | P0 | ✅ | 完整实现 |
| WEB-4-01 | 登录页（`login`） | 2 | P0 | ⏳ | 有骨架，待完善 |
| WEB-4-02 | 文章列表页（`articles`）：筛选/搜索/排序/批量 | 4 | P0 | ❌ | 待前端开发 |
| WEB-4-03 | 文章编辑页（`articles/:id`）：正文/AI 结果/状态面板/Chunk | 4 | P0 | ❌ | 待前端开发 |
| WEB-4-04 | 仪表盘页（`dashboard`） | 3 | P1 | ❌ | 待前端开发 |
| WEB-4-05 | 文章录入页（`articles/new` 完整版） | 3 | P1 | ❌ | 待前端开发 |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| SVC-4-01 | `mcplog.go`：MCP 日志异步写入 + 查询 | 3 | P1 | ✅ | 完整实现 |

**本周 P0 交付 = 里程碑 M4**。

---

## Week 5：MCP + API（目标：Claude Desktop 可挂载读写）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| SVC-5-01 | `service/mcp`：4 工具实现 + scope 过滤 + 协议封装 | 4 | P0 | ✅ | 完整实现 |
| HND-5-01 | `handler/mcp/server.go`：Streamable HTTP / initialize / tools/list / tools/call / scope / Origin / trace_id / SSE | 6 | P0 | ✅ | 完整实现 |
| HND-5-02 | 路由装配：注册全部路由 + 中间件链 + 启动 worker | 2 | P0 | ✅ | main.go 完成 |
| HND-5-03 | REST API 补齐：限流 + 全接口 | 2 | P1 | ⏳ | 部分完成 |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| QAL-5-01 | MCP 端到端脚本骨架（Claude Desktop 配置示例 + 4 工具冒烟） | 2 | P2 | ⏳ | 需真实环境测试 |

**本周 P0 交付 = 里程碑 M5**。

---

## Week 6：调试与配置页（目标：完整后台管理与调试能力）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| HND-6-01 | `handler/admin/search_debug.go`：分阶段结果 + 关键词 debug | 4 | P0 | ✅ | 完整实现 |
| HND-6-02 | `handler/admin/mcp_debug.go`：`mcp/invoke` 代理转发 | 4 | P0 | ✅ | 完整实现 |
| HND-6-03 | `handler/admin/setting.go`：读写/reset/history | 3 | P0 | ✅ | 完整实现 |
| HND-6-04 | `handler/admin/prompt.go`：提示词试跑 | 2 | P0 | ✅ | 完整实现 |
| HND-6-05 | `handler/admin/apikey.go`：CRUD（明文仅一次） | 2 | P0 | ✅ | 完整实现 |
| HND-6-06 | `handler/admin/log.go`：分页/筛选/全文检索/trace_id | 3 | P0 | ✅ | 完整实现 |
| WEB-6-01 | 检索测试页（`search`） | 4 | P0 | ❌ | 待前端开发 |
| WEB-6-02 | MCP 调试页（`mcp-debug`） | 4 | P0 | ❌ | 待前端开发 |
| WEB-6-03 | MCP 日志页（`mcp-logs`） | 3 | P0 | ❌ | 待前端开发 |
| WEB-6-04 | 系统配置页（`settings`） | 3 | P0 | ❌ | 待前端开发 |
| WEB-6-05 | API Key 管理页（`settings/api-keys`） | 2 | P1 | ❌ | 待前端开发 |

**本周 P0 交付 = 里程碑 M6**。

---

## Week 7：测试与发布（目标：v1.0.0 交付）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | 状态 | 备注 |
| :--- | :--- | :---: | :---: | :---: | :--- |
| QAL-7-01 | 集成测试（API 端到端） | 3 | P0 | ✅ | 3 个测试文件完成 |
| QAL-7-02 | 性能测试（压测 + P95 测量，见 PRD 9.1） | 4 | P0 | ❌ | 待实现 |
| QAL-7-03 | 安全测试（SSRF/XSS/鉴权/限流，PRD 6） | 4 | P0 | ❌ | 待实现 |
| QAL-7-04 | MCP 端到端测试（Claude Desktop 挂载 + 4 工具） | 4 | P0 | ❌ | 需真实环境 |
| QAL-7-05 | 检索评测（标准问题集 + TopK 命中率/引用准确率） | 4 | P1 | ❌ | 待实现 |
| QAL-7-06 | 发布准备（Bug 修复 + 验收逐项核对 + v1.0.0 标签） | 4 | P0 | 🔄 | 进行中 |
| DOC-7-01 | README + 部署文档 | 2 | P1 | ✅ | 完成 |
| DOC-7-02 | 配置与 API 文档 | 2 | P1 | ✅ | 完成 |
| DOC-7-03 | MCP 使用文档（含 Claude Desktop 配置示例） | 2 | P1 | ✅ | 完成 |

**本周完成 = 里程碑 M7（终验）**。

---

## 点数汇总（供排期参考）

| 周 | P0 点数 | P1/P2 点数 | 总点数 | 关键路径风险 |
| :--- | :---: | :---: | :---: | :--- |
| W1 | 9 | 25 | 34 | 🟥 迁移 + 接口契约（阻塞全局） |
| W2 | 20 | 8 | 28 | 🟥 AI 流水线首次跑通 |
| W3 | 14 | 11 | 25 | 🟥 检索核心（R3/S5） |
| W4 | 21 | 10 | 31 | 🟥 后台基础页批量上线 |
| W5 | 12 | 2 | 14 | 🟥 MCP 端到端（最紧） |
| W6 | 33 | 2 | 35 | 🟥 调试/配置页最重，建议预留缓冲 |
| W7 | 23 | 10 | 33 | 🟥 全面验收 |

> **提示**：W6 是 P0 点数最高（33），与 PRD「Phase 6 交互较重」的提示一致，团队需在 W5 提前预演 MCP 调试代理的接口契约，避免 W6 集中爆发。

---

## 快速开始（如何用这份清单）

1. **Sprint Planning**：每周从对应章节取 P0 任务排进 Sprint，P1 任务顺势填充。
2. **每日站会**：只追踪 🟥 关键路径任务是否如期；滞后当天升级。
3. **DoD 核对**：P0 任务未达 DoD 一律不算完成，不允许「假完成」滚入下周。
4. **验收对照**：每个里程碑完成后，打开 [MILESTONES.md](./MILESTONES.md) 逐项打勾。
