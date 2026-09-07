# AiKnow 按周开发 Ticket 清单（v1 MVP）

**版本**：v1.0  
**日期**：2026-09-07  
**关联**：[DEVELOPMENT_PLAN.md](./DEVELOPMENT_PLAN.md)（轨道与 WBS）、[PRD.md](./PRD.md)（需求）、[MILESTONES.md](./MILESTONES.md)（里程碑验收）  
**状态**：待执行

> 本文档把 7 周计划拆成**可分配、可估点、带优先级和 DoD（完成定义）**的具体任务卡，作为每日站会与 Sprint Planning 的直接输入。
>
> **DoD（Definition of Done）约定**：每个 `P0` 任务须同时满足——① 代码通过 `go vet` + 相关单测；② 关键行为有可验证产物（可编译/可 curl/可 UI 操作）；③ 无遗留 `TODO` 阻塞项。`P1`/`P2` 任务可延后，见 PRD 2.x 优先级列。

---

## 使用说明

- 任务编号 `${轨道代号}-${周}-${序号}`，如 `FND-1-02` 表示基础轨道第 1 周第 2 个任务。
- **点数**：1 点 ≈ 半天工作量，2 点 = 1 人天（中等水平工程师）。
- **优先级**：`P0`=本周必须完成（关键路径 / 阻塞下游）；`P1`=本周尽量完成，可顺延至下周缓冲；`P2`=可选打磨项。

---

## Week 1：基础框架（目标：可编译骨架 + Schema + 配置 + AI Provider 接口）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| FND-1-01 | 项目骨架初始化（go.mod、Makefile、`.gitignore` 含 `.env`、`cmd/aiknow-server/main.go` 空服务、`hashpwd` 子命令框架） | 2 | P0 | `go build` 通过，`./aiknow-server --help` 可显示子命令 |
| FND-1-02 | 数据库连接池（`internal/repository/postgres.go`，pgx pool + ping + 优雅关闭） | 1 | P0 | 可连本地/远端 PG 并 ping 通 |
| FND-1-03 | 数据库迁移 001–005 + 迁移执行器 | 4 | P0 | `migrations` 全部执行成功，含 vector/BM25/PGMQ/zhparser 扩展可用性验证 |
| FND-1-04 | 数据模型定义（`model/*.go`：article/chunk/tag/setting/apikey/mcplog） | 2 | P0 | 全部 struct + db/json tag 与三份 PRD SQL 对齐 |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| FND-1-05 | L1 配置加载（`config.go`：env > .env > yaml > 默认值，godotenv，来源日志不打印值） | 4 | P0 | 三种来源优先级单测通过；缺必填项启动报错 |
| FND-1-06 | repository/service 接口契约定义 + mock 约定 | 2 | P0 | 接口冻结并写入 DEV 文档，供所有轨道 rebase |
| FND-1-07 | L2 配置缓存 + 热重载（`defaults.go`/`cache.go`，LISTEN/NOTIFY） | 4 | P1 | 缺键回落默认值；改配置后缓存刷新有单测 |
| AIP-1-01 | `LLMProvider` 接口 + Qwen 实现（JSON mode + 重试） | 4 | P1 | mock 模式可返回合法 JSON；真实调用需 Qwen Key |
| AIP-1-02 | `EmbeddingProvider` 接口 + BAAI 实现（1024 维 + 批量） | 3 | P1 | 返回 1024 维向量，维度与 `VECTOR(1024)` 一致 |
| AIP-1-03 | `RerankerProvider` 接口 + BAAI 实现 | 2 | P2 | 接口可用，真实联调可后置 |
| AIP-1-04 | 提示词引擎 + JSON 校验（`prompt.go`：template 渲染 + 分级重试策略） | 3 | P1 | 模板可编译校验 + 结构错误重试/内容越界兜底有单测 |
| PKG-1-01 | `pkg/ssrf`：URL 校验/内网黑名单/DNS 重绑定/端口限制/重定向 | 4 | P1 | 内网 IP 拦截 + 协议白名单单测通过 |
| PKG-1-02 | `pkg/htmlclean`：bluemonday 封装 | 1 | P2 | XSS 清洗单测通过 |
| PKG-1-03 | `pkg/pgqueue`：PGMQ 封装（send/pop/archive/深度查询） | 3 | P1 | 消息收发 + DLQ 单测通过 |
| WEB-1-01 | 前端骨架（`index.html`、Tailwind 构建、`app.js` 路由、布局、登录态守卫） | 4 | P1 | 页面壳可导航，未登录跳转 `/ui/login` |

**本周 P0 交付 = 里程碑 M1**（见 MILESTONES.md）。

---

## Week 2：采集 + AI 处理（目标：完整录入流程 + 异步状态可查）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| REP-2-01 | `article_repo.go`：CRUD / 分页筛选 / 去重查询 / 软删除 / 批量 | 4 | P0 | 文章增删改查 + 去重查询单测通过 |
| REP-2-02 | `chunk_repo.go`：chunk 增删查、按 article 分组 | 2 | P0 | chunk 读写 + 分组的单测通过 |
| SVC-2-01 | `article.go`：文章 CRUD、分类/标签自动创建、内容 hash 去重 | 4 | P0 | 录入/去重/软删除接口可用 |
| SVC-2-02 | `chunk.go`：文本切分（200–500 字 + overlap） | 2 | P0 | 切分长度与 overlap 边界单测通过 |
| SVC-2-03 | AI 处理流水线（编排 LLM → chunk → embedding → 状态更新） | 4 | P0 | 单篇文章从 `pending` 走到 `completed`，三项结果写入 |
| SVC-2-04 | `worker.go`：PGMQ 消费、并发、重试、状态字段更新 | 4 | P0 | 异步消费 + 失败重试 + `retry_count`/`last_error` 更新 |
| AIP-2-01 | 补全 4.3 全部 Provider 的真实联调（替换 mock） | 2 | P1 | Qwen/BAAI 真实调用返回符合预期 |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| COL-2-01 | `collector.go`：URL 采集 + 正文提取 + 去重 hash | 4 | P0 | 有效 URL 提取标题/纯文本，超时/重定向受控 |
| REP-2-03 | `setting_repo.go` / `apikey_repo.go` / `mcplog_repo.go`（R4/R5/R6） | 4 | P1 | 三个仓库读写单测通过（可顺延 W3） |
| WEB-2-01 | 文章录入页雏形（`articles/new`，对接 H4 契约，mock 先行） | 2 | P2 | 表单可提交（mock 数据） |

**本周 P0 交付 = 里程碑 M2**。

---

## Week 3：检索核心（目标：混合检索 + 关键词检索可调用）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| SRC-3-01 | `search_repo.go`：pgvector HNSW 查询 + pg_textsearch BM25 查询 | 4 | P0 | 向量召回 Top 20 + BM25 召回 Top 20 正确 |
| SRC-3-02 | `search.go`：混合检索（向量 + BM25 + RRF + Rerank + 上下文补全） | 6 | P0 | `/api/v1/search` 返回精排 Top 5 含证据块+分数 |
| SRC-3-03 | `search_keyword.go`：关键词加权检索（5 档加权 + matched 字段） | 4 | P0 | `/api/v1/search/keywords` 返回加权排序的文章卡片 |
| HND-3-01 | `handler/api/article.go` + `search.go`：对外 REST 初版 | 4 | P1 | `/api/v1/articles` 与 `/search` 可 curl |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| SVC-3-01 | `auth.go`：API Key 校验（SHA-256）+ 限流 + 管理员认证 | 4 | P1 | Key 鉴权 + 读写限流单测通过 |
| SVC-3-02 | `setting.go`：L2 配置读写 + 校验 + history | 3 | P1 | 配置校验（权重和/范围）单测通过 |
| REP-3-01 | W2 顺延的 R4/R5/R6 收尾 | 2 | P1 | 仓库层全部就绪 |
| HND-3-02 | `handler/api/middleware.go`：API Key 鉴权 + 限流中间件 | 2 | P1 | 无 Key 返回 401，超限返回 429 |

**本周 P0 交付 = 里程碑 M3**。

---

## Week 4：精排 + 后台基础页（目标：可管理文章的 Web 后台）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| HND-4-01 | `handler/admin/auth.go`：login/logout/session/me + 失败锁定 | 3 | P0 | 登录成功返回 HttpOnly Cookie；5 次失败锁定 15 分钟 |
| HND-4-02 | `handler/admin/middleware.go`：Session 校验 + CSRF | 2 | P0 | 未登录访问 admin 接口返回 401 |
| HND-4-03 | `handler/admin/dashboard.go`：统计 + 队列 + 活动 + 健康 | 2 | P0 | 仪表盘聚合接口返回各项数据 |
| HND-4-04 | `handler/admin/article.go`：批量操作 + Chunk 查看 | 3 | P0 | 批量重处理/删除 + chunk 列表接口可用 |
| HND-4-05 | `handler/web/handler.go`：embed 静态资源 + SPA 回退 | 1 | P0 | `/ui/*` 可访问并回退到 index.html |
| WEB-4-01 | 登录页（`login`） | 2 | P0 | 登录/锁定提示/登出可用 |
| WEB-4-02 | 文章列表页（`articles`）：筛选/搜索/排序/批量 | 4 | P0 | 列表 + 筛选 + 批量操作可用 |
| WEB-4-03 | 文章编辑页（`articles/:id`）：正文/AI 结果/状态面板/Chunk | 4 | P0 | 改正文重置 pending、仅改 AI 结果不重置 |
| WEB-4-04 | 仪表盘页（`dashboard`） | 3 | P1 | 展示统计卡片 + 状态分布 + 队列深度 |
| WEB-4-05 | 文章录入页（`articles/new` 完整版） | 3 | P1 | 手动 + URL 采集（含预览）可用 |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| SVC-4-01 | `mcplog.go`：MCP 日志异步写入 + 查询 | 3 | P1 | 日志异步写 + 查询单测通过 |

**本周 P0 交付 = 里程碑 M4**。

---

## Week 5：MCP + API（目标：Claude Desktop 可挂载读写）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| SVC-5-01 | `service/mcp（S10）`：4 工具实现 + scope 过滤 + 协议封装 | 4 | P0 | 4 工具可独立调用，readonly Key 看不到写工具 |
| HND-5-01 | `handler/mcp/server.go（H13）`：Streamable HTTP / initialize / tools/list / tools/call / scope / Origin / trace_id / SSE | 6 | P0 | `/mcp` 握手成功，SSE 可用，401/403 正确 |
| HND-5-02 | 路由装配 `H14`：注册全部路由 + 中间件链 + 启动 worker | 2 | P0 | `main.go` 可启动完整服务，全路由可达 |
| HND-5-03 | REST API 补齐：限流 + 全接口 | 2 | P1 | 写入工具与 REST 共享限流计数器，超限 429 |

### 🟨 可并行（本周尽量完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| QAL-5-01 | MCP 端到端脚本骨架（Claude Desktop 配置示例 + 4 工具冒烟） | 2 | P2 | 手动挂载验证记录（正式 Q6 在 W7） |

**本周 P0 交付 = 里程碑 M5**。

---

## Week 6：调试与配置页（目标：完整后台管理与调试能力）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| HND-6-01 | `handler/admin/search_debug.go`：分阶段结果 + 关键词 debug | 4 | P0 | 检索调试接口返回四阶段中间结果 + 得分构成 |
| HND-6-02 | `handler/admin/mcp_debug.go`：`mcp/invoke` 代理转发 | 4 | P0 | 后台代理 MCP 调用返回完整 request/response |
| HND-6-03 | `handler/admin/setting.go`：读写/reset/history | 3 | P0 | 配置读写 + 校验 + 恢复默认 + 变更记录可用 |
| HND-6-04 | `handler/admin/prompt.go`：提示词试跑 | 2 | P0 | 未保存模板可试跑并展示渲染后 Prompt + 模型返回 |
| HND-6-05 | `handler/admin/apikey.go`：CRUD（明文仅一次） | 2 | P0 | 创建返回一次明文，列表仅显示前缀 |
| HND-6-06 | `handler/admin/log.go`：分页/筛选/全文检索/trace_id | 3 | P0 | MCP 日志查询接口可用 |
| WEB-6-01 | 检索测试页（`search`） | 4 | P0 | 四阶段结果 + 耗时 + 分词展示 |
| WEB-6-02 | MCP 调试页（`mcp-debug`） | 4 | P0 | initialize/tools/list/tools/call + SSE 可用 |
| WEB-6-03 | MCP 日志页（`mcp-logs`） | 3 | P0 | 筛选/全文检索/trace_id/重放可用 |
| WEB-6-04 | 系统配置页（`settings`） | 3 | P0 | 参数配置 + 校验拒绝 + 试跑可用 |
| WEB-6-05 | API Key 管理页（`settings/api-keys`） | 2 | P1 | 创建/禁用/删除可用 |

**本周 P0 交付 = 里程碑 M6**。

---

## Week 7：测试与发布（目标：v1.0.0 交付）

### 🟥 关键路径（必须完成）

| 编号 | 任务 | 点 | 优先级 | DoD |
| :--- | :--- | :---: | :---: | :--- |
| QAL-7-01 | 集成测试（API 端到端） | 3 | P0 | 全 API 场景测试套件通过 |
| QAL-7-02 | 性能测试（压测 + P95 测量，见 PRD 9.1） | 4 | P0 | 性能报告：召回 P95<100ms、端到端 P95<1s |
| QAL-7-03 | 安全测试（SSRF/XSS/鉴权/限流，PRD 6） | 4 | P0 | 安全测试报告，全部控制点通过 |
| QAL-7-04 | MCP 端到端测试（Claude Desktop 挂载 + 4 工具） | 4 | P0 | 挂载验证记录 |
| QAL-7-05 | 检索评测（标准问题集 + TopK 命中率/引用准确率） | 4 | P1 | 评测报告 + 脚本 |
| QAL-7-06 | 发布准备（Bug 修复 + 验收逐项核对 + v1.0.0 标签） | 4 | P0 | 里程碑 M7 全部验收项打勾 + tag |
| DOC-7-01 | README + 部署文档 | 2 | P1 | 可按文档一键部署 |
| DOC-7-02 | 配置与 API 文档 | 2 | P1 | 接口文档完整 |
| DOC-7-03 | MCP 使用文档（含 Claude Desktop 配置示例） | 2 | P1 | MCP 接入说明 |

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
