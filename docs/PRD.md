# 爱脑 AiKnow：AI 知识库管理工具需求与技术方案

**版本**：v3.4.1（MCP 工具扩展版）  
**日期**：2026-09-07  
**状态**：已评审，待开发


## 一、项目概述

### 1.1 项目名称

- **中文名称**：爱脑
- **英文名称**：AiKnow
- **名称含义**：AI 大脑，爱护你的大脑，英文 AI Knowledge Base 的缩写
- **工程命名**：
  - 产品名：`AiKnow`
  - 仓库/二进制：`aiknow`
  - API 服务名：`aiknow-server`
  - Go Module：`github.com/your-org/aiknow`

> **注**：本文档及后续所有技术资料统一使用上述命名，不再混用 `knowledge-base`、`ai-knowledge-base` 等旧称。

### 1.2 项目定位

爱脑 AiKnow 是一个面向企业内部的知识库管理工具，定位为**内部知识管理 + RAG 检索 + API/MCP 接入**的基础设施。核心价值在于：

- 帮助团队沉淀、整理和检索知识文档
- 通过 AI 能力自动化知识加工（摘要、分类、标签）
- 对外提供标准化接口，供 AI 智能体（如 Claude Desktop）作为 RAG 数据源使用

### 1.3 v1 目标

**v1 定位**：一个可用的内部知识库 + 稳定检索 + MCP 接入（含一个写入工具）。先跑通核心闭环，再扩展高级功能。

**v1 交付承诺**：
- ✅ 文章的增删改查和手动/URL 采集录入
- ✅ AI 异步处理（摘要、分类、标签），状态可查询
- ✅ 基于向量 + 关键词的混合检索（含中文分词）
- ✅ RESTful API + API Key 鉴权
- ✅ MCP 四个工具：`search_knowledge`、`get_article_detail`、`find_articles_by_keywords`、`add_article_by_url`
- ✅ 基础的防 SSRF、防 XSS 安全措施

**v1 明确不包含**：
- ❌ 批量导入/导出
- ❌ 标签/分类管理界面（后台预留数据表，管理员可手动 SQL 维护）
- ❌ 版本历史与回滚
- ❌ 回收站
- ❌ MCP 其他写入工具（更新、删除文章）
- ❌ 搜索建议/高级筛选


## 二、功能需求

### 2.1 内容采集模块

| 功能 | 描述 | 优先级 |
| :--- | :--- | :--- |
| **手动录入** | 支持 Markdown 格式，手动输入标题、正文、分类、标签 | P0 |
| **网址采集** | 输入 URL，自动解析网页正文与标题。渲染优先（无头浏览器，失败降级 http）；微信走专用规则、其余走 Readability，正文以 Markdown 保留结构 | P0 |
| **截图归档** | URL 文章处理时截取全页图上传 COS，写回 `screenshot_url`，防源站失效 | P1 |
| **防 SSRF** | URL 采集时限制内网 IP 访问，仅允许 HTTP/HTTPS 协议 | P0 |
| **内容去重** | 同 URL 或同内容 hash 自动去重，避免重复入库 | P0 |
| **采集预览** | 采集后返回预览信息（标题、Markdown 正文、是否渲染），管理员确认后再正式录入 | P1 |
| **批量导入** | 支持从本地文件（Markdown、TXT）批量导入 | P2 |

### 2.2 AI 智能处理模块

| 功能 | 描述 | 优先级 |
| :--- | :--- | :--- |
| **单次调用产出三项** | 一次 LLM 调用同时返回摘要、分类、标签（JSON 结构化输出） | P0 |
| **自动摘要** | 生成 100-300 字的核心概要 | P0 |
| **自动分类** | 归入预设分类体系 | P0 |
| **自动打标签** | 提取 3-10 个关键词作为标签 | P0 |
| **JSON 输出约束** | 使用 `response_format: json_object` 强制结构化返回 | P0 |
| **结果校验** | 对返回 JSON 做 schema 校验与业务校验，不合法则重试 | P0 |
| **异步处理** | 提交后立即返回，通过消息队列异步完成 | P0 |
| **状态查询** | 提供 `processing_status`、`retry_count`、`last_error` 等状态字段 | P0 |
| **人工修正** | 支持管理员在 Web 界面编辑摘要、分类、标签 | P0 |
| **可重试** | AI 处理失败时支持手动或自动重试 | P0 |
| **重新处理** | 支持对已有文章重新触发 AI 处理（如模型升级） | P1 |

> **设计说明**：摘要、分类、标签三项任务都建立在对同一篇文章的理解之上，拆成三次调用会让同一份正文被重复输入三次，token 成本和处理耗时都接近三倍。合并为单次调用后，输入 token 降至约 1/3，且三项结果基于同一次推理，一致性更好。

### 2.3 知识管理模块

| 功能 | 描述 | 优先级 |
| :--- | :--- | :--- |
| **文章 CRUD** | 完整的增删改查操作 | P0 |
| **分页列表** | 支持按时间、分类、标签筛选和排序 | P0 |
| **关键词检索** | 基于 PostgreSQL 全文检索（含中文分词） | P0 |
| **语义检索** | 基于向量嵌入的语义相似度检索 | P0 |
| **混合检索** | 语义 + 关键词检索结果融合（RRF） | P0 |
| **关键词找文章** | 输入多个关键词，标签/分类加权，返回 Top N 文章卡片 | P0 |
| **精排（Rerank）** | 使用 BAAI Reranker 对候选结果进行深度打分 | P0 |
| **父子文档结构** | 长文档拆分为 Chunk 索引，检索返回完整上下文 | P0 |
| **标签统一管理** | 管理员可合并、重命名、删除标签 | P1 |
| **分类树管理** | 自定义树形分类体系 | P1 |
| **版本历史** | 记录文章修改历史，支持回滚 | P2 |
| **回收站** | 软删除 + 恢复/彻底删除 | P2 |

### 2.4 对外接口模块

| 功能 | 描述 | 优先级 |
| :--- | :--- | :--- |
| **RESTful API** | 完整 CRUD + 检索接口，供内部系统调用 | P0 |
| **API Key 鉴权** | 基于 API Key 的身份认证，支持 scope 限定 | P0 |
| **速率限制** | 按 API Key 进行请求限流 | P0 |
| **MCP 请求日志** | 记录每次 MCP 调用的完整请求/响应，支持检索与重放 | P0 |
| **MCP Streamable HTTP** | 符合 MCP 2025-06-18 规范的 Streamable HTTP 传输 | P0 |
| **MCP 检索 Tools** | `search_knowledge`（语义证据块）、`find_articles_by_keywords`（关键词找文章） | P0 |
| **MCP 读取 Tools** | `get_article_detail` | P0 |
| **MCP 写入 Tools** | `add_article_by_url`（需 readwrite scope） | P0 |
| **MCP 更新/删除 Tools** | `update_article`、`delete_article` 等 | P2 |

### 2.5 Web 管理后台模块

后台采用左侧菜单 + 右侧内容区的单页应用布局，共 6 个一级菜单。所有页面通过 `/api/v1/*` 接口与后端交互，使用 Session Cookie 鉴权（非 API Key）。

#### 2.5.1 页面清单

| 页面 | 路径 | 功能 | 优先级 |
| :--- | :--- | :--- | :--- |
| **登录页** | `/ui/login` | 用户名密码登录，失败次数限制提示 | P0 |
| **仪表盘** | `/ui/dashboard` | 数据概览 + 队列状态 + 最近活动 | P0 |
| **文章列表** | `/ui/articles` | 分页、筛选、搜索、批量操作；标题进预览、操作列进编辑 | P0 |
| **文章预览** | `/ui/articles/:id/preview` | 阅读态 Markdown 正文、AI 摘要、网页截图灯箱 | P1 |
| **文章编辑** | `/ui/articles/:id` | 正文 Markdown 分屏编辑与实时预览，编辑 AI 结果，查看 Chunks | P0 |
| **文章录入** | `/ui/articles/new` | 手动录入 / URL 采集（含预览） | P0 |
| **检索测试** | `/ui/search` | 混合检索调试，展示各阶段中间结果 | P0 |
| **MCP 调试** | `/ui/mcp-debug` | 输入参数测试 MCP 工具输出 | P0 |
| **MCP 请求日志** | `/ui/mcp-logs` | 检索历史 MCP 调用，支持回放 | P0 |
| **系统配置** | `/ui/settings` | 业务参数与提示词配置 | P0 |
| **API Key 管理** | `/ui/settings/api-keys` | 创建、查看、禁用、删除 API Key | P0 |

#### 2.5.2 仪表盘（`/ui/dashboard`）

| 区块 | 内容 |
| :--- | :--- |
| **统计卡片** | 文章总数、Chunk 总数、标签数、分类数 |
| **处理状态分布** | 按 `processing_status` 分组计数（pending / processing / completed / failed），点击可跳转到已筛选的文章列表 |
| **队列状态** | PGMQ 队列深度、最早消息等待时长、DLQ 消息数 |
| **最近活动** | 最近 10 条录入记录和 10 条失败任务（含 `last_error` 摘要） |
| **服务健康** | 数据库连通性、LLM / Embedding / Reranker 三个 Provider 的探活结果 |

#### 2.5.3 文章列表（`/ui/articles`）

| 能力 | 说明 |
| :--- | :--- |
| **列表字段** | 标题、分类、标签、处理状态、来源（手动 / URL）、创建时间、更新时间 |
| **筛选** | 分类、标签（多选）、处理状态、发布状态、时间范围 |
| **搜索** | 标题关键词搜索（走 `idx_articles_title_gin`） |
| **排序** | 创建时间、更新时间、浏览数，升序 / 降序 |
| **行内操作** | 编辑、重新处理（reprocess）、删除（软删除，二次确认） |
| **批量操作** | 批量重新处理、批量删除（选中项计数 + 二次确认） |
| **状态提示** | `failed` 状态行标红并悬浮显示 `last_error`，提供「重试」按钮 |

#### 2.5.4 文章编辑（`/ui/articles/:id`）

| 区块 | 说明 |
| :--- | :--- |
| **基础信息** | 标题、作者、来源 URL、发布状态 |
| **正文编辑** | Markdown 分屏编辑器 + 实时预览（marked 渲染，DOMPurify 前端清洗防 XSS） |
| **AI 结果修正** | 摘要（多行文本）、分类（下拉选择 / 新建）、标签（可增删的 tag 输入框） |
| **处理状态面板** | `processing_status`、`retry_count`、`last_error`、`processed_at`、`embedding_model`、`embedding_dim`、`indexed_at` |
| **Chunk 预览** | 按 `chunk_index` 列出所有 Chunk 的内容与字数，标识是否已生成向量 |
| **操作** | 保存、保存并重新处理、仅重新生成向量、删除 |

> **说明**：正文变更后保存会将 `processing_status` 重置为 `pending` 并重新入队；仅修改 AI 结果（摘要 / 分类 / 标签）不触发重新处理。

#### 2.5.5 检索测试（`/ui/search`）

用于验证混合检索效果，是调优 RRF 权重和 `ef_search` 的主要工具。页面分「语义检索」和「关键词检索」两个 Tab。

**语义检索 Tab**：

| 能力 | 说明 |
| :--- | :--- |
| **查询输入** | Query 文本 + `top_k`、`candidate_k`、`rrf_k`、`vector_weight`、`bm25_weight` 可调 |
| **筛选条件** | 分类、标签 |
| **分阶段结果** | 三栏并列展示：向量召回 Top N、BM25 召回 Top N、RRF 融合后候选 |
| **精排结果** | 展示 Reranker 打分后的最终结果，标注每条在 RRF 阶段的原始排名变化 |
| **耗时明细** | Embedding 调用、向量召回、BM25 召回、RRF 融合、Rerank 调用各阶段耗时 |
| **分词结果** | 展示 zhparser 对 Query 的分词结果，便于排查中文检索问题 |

**关键词检索 Tab**：

| 能力 | 说明 |
| :--- | :--- |
| **关键词输入** | 可增删的多值输入框，`match_mode` 与 `limit` 可调 |
| **权重调试** | 5 项权重可临时覆盖（不写入配置），用于对比不同权重下的排序 |
| **得分构成** | 每篇文章展开显示各关键词在各位置的命中情况与贡献分值 |
| **结果展示** | 标题、摘要、分类、标签、来源链接、总分 |

#### 2.5.6 MCP 调试（`/ui/mcp-debug`）

在后台直接构造 MCP 请求并查看原始响应，无需依赖 Claude Desktop 等外部客户端即可验证 MCP 接口。

| 能力 | 说明 | 优先级 |
| :--- | :--- | :--- |
| **工具选择** | 下拉列出 `tools/list` 返回的工具，选中后按 `inputSchema` 动态渲染参数表单 | P0 |
| **参数输入** | 表单模式（按 schema 渲染字段）与 JSON 模式（直接编辑 arguments）可切换 | P0 |
| **原始请求预览** | 展示将要发送的完整 JSON-RPC 请求体，可复制 | P0 |
| **原始响应展示** | JSON 高亮展示 `tools/call` 的完整响应，含 `content`、`annotations`、`isError` | P0 |
| **渲染预览** | 将 `content` 数组按类型渲染（text 显示文本，resource 显示为可点击卡片，跳转对应文章） | P0 |
| **协议操作** | 提供 `initialize`、`tools/list`、`tools/call` 三个按钮，分别发起对应请求 | P0 |
| **SSE 模式** | 勾选后以 `Accept: text/event-stream` 发起请求，逐条追加显示事件流 | P0 |
| **响应头展示** | 显示 `MCP-Protocol-Version`、`Mcp-Session-Id` 等关键响应头 | P0 |
| **鉴权切换** | 可选择使用当前登录 Session，或指定某个 API Key 发起调用（用于验证 scope 与 401 行为） | P0 |
| **耗时统计** | 显示端到端耗时，并展示响应 `annotations` 中的 `query_time_ms` | P0 |
| **从日志重放** | 在 MCP 请求日志页点击「重放」，自动跳转本页并回填该次调用的参数 | P0 |
| **调用历史** | 保留本次会话内最近 20 次调用记录，可一键回填参数重放 | P1 |

**调试页面调用链**：页面通过后端提供的 `/api/v1/admin/mcp/invoke` 代理接口转发到 MCP 处理器，而非浏览器直连 `/mcp`。这样做的原因是避免在前端暴露 API Key，同时绕开浏览器对 `Origin` 校验的限制。

#### 2.5.7 API Key 管理（`/ui/settings/api-keys`）

| 能力 | 说明 |
| :--- | :--- |
| **列表** | 显示描述、scope、前缀（如 `ak-xxx****`）、创建时间、最后使用时间、状态 |
| **创建** | 输入描述与 scope，生成后**仅展示一次明文**，之后仅存哈希 |
| **禁用 / 启用** | 临时停用而不删除 |
| **删除** | 二次确认后永久删除 |

> **注意**：API Key 在数据库中仅存储哈希值（与密码同等对待），列表页不提供明文查看功能。

#### 2.5.8 MCP 请求日志（`/ui/mcp-logs`）

记录每一次 MCP 请求的完整入参与出参，用于排查「Claude 那边为什么没搜到」这类问题——这类问题通常需要还原当时的确切调用参数才能定位。

| 能力 | 说明 | 优先级 |
| :--- | :--- | :--- |
| **列表字段** | 时间、trace_id、方法（`tools/call` 等）、工具名、调用方（API Key 描述）、耗时、结果状态 | P0 |
| **筛选** | 时间范围、方法、工具名、调用方、状态（成功/失败）、耗时区间 | P0 |
| **全文检索** | 按请求参数内容检索（如查所有含「Docker」的 query） | P0 |
| **trace_id 查询** | 输入 trace_id 精确定位单次调用 | P0 |
| **详情** | 展开查看完整 JSON-RPC 请求体、响应体、响应头、错误信息 | P0 |
| **一键重放** | 跳转 MCP 调试页并回填该次调用的参数 | P0 |
| **检索结果回溯** | `search_knowledge` 调用展示当次返回的 chunk 列表与分数 | P0 |
| **失败高亮** | `isError=true` 或 HTTP 非 2xx 的记录标红 | P0 |

**trace_id 规则**：优先取请求头 `X-Trace-Id`（不强制，客户端可不传）；未提供时服务端生成 UUID。响应头回写 `X-Trace-Id`，便于调用方关联自己的日志。

> **保留策略**：默认保留 30 天，通过 `pg_cron` 每日清理过期记录。保留天数在后台可配置（`mcp_log.retention_days`）。

### 2.6 系统配置模块

将原本集中在 `config.yaml` 的参数按性质分为两类：**启动配置**（环境变量 / `.env` / 配置文件）与**运行时业务配置**（数据库存储，后台可改）。详见 [7.3 配置分层设计](#73-配置分层设计)。

| 功能 | 描述 | 优先级 |
| :--- | :--- | :--- |
| **模型参数配置** | 在后台修改 LLM / Embedding / Reranker 的 model、temperature、超时、重试次数等 | P0 |
| **提示词配置** | 在后台编辑内容分析提示词模板，支持变量占位符 | P0 |
| **检索参数配置** | 在后台调整 `top_k`、`candidate_k`、`rrf_k`、权重、`ef_search` | P0 |
| **采集参数配置** | 在后台调整超时、最大重定向、User-Agent、正文最小长度 | P0 |
| **日志保留配置** | 在后台调整 MCP 请求日志保留天数 | P0 |
| **配置热生效** | 保存后无需重启服务，下一次请求即使用新配置 | P0 |
| **提示词测试** | 在配置页面选一篇已有文章，用当前提示词试跑并查看 LLM 返回 | P0 |
| **配置校验** | 保存前校验取值范围（如权重和为 1、`top_k` ≤ `candidate_k`），非法值拒绝保存 | P0 |
| **恢复默认** | 单项或整组恢复为内置默认值 | P0 |
| **变更记录** | 记录每次配置变更的时间、旧值、新值 | P1 |


## 三、技术架构

### 3.1 技术栈清单

| 层级 | 组件 | 选型 | 版本 | 说明 |
| :--- | :--- | :--- | :--- | :--- |
| **后端语言** | Go | 1.23+ | 编译型，高性能，单二进制部署 |
| **Web 框架** | Gin | 1.10+ | 轻量高性能 HTTP 框架 |
| **数据库** | PostgreSQL | 17+ | 核心数据存储，支持 pg_textsearch |
| **向量扩展** | pgvector | 0.7+ | 向量存储与相似性检索 |
| **全文检索** | pg_textsearch | 1.3.1+ | 提供标准 BM25 排序能力 |
| **中文分词** | zhparser | 2.2+ | PostgreSQL 中文分词解析器 |
| **消息队列** | PGMQ | 1.0+ | PostgreSQL 原生轻量消息队列 |
| **定时任务** | pg_cron | 1.0+ | 数据库内置定时任务 |
| **Go 数据库驱动** | pgx | 5.0+ | 高性能 PostgreSQL 驱动 |
| **前端架构** | 前后端分离 | - | 为未来 Vue 迁移预留空间 |
| **前端 UI 框架** | Alpine.js | 3.14+ | 轻量级响应式框架（~15KB），无需编译 |
| **前端 CSS** | Tailwind CSS | 3.4+ | 原子化 CSS，快速构建界面 |
| **前端 HTTP** | 原生 `fetch` API | - | 与 Go 后端 JSON API 交互 |
| **MCP 协议** | Streamable HTTP | 2025-06-18 | 行业标准，主流客户端原生支持 |
| **MCP Go SDK** | `mcp-go` | latest | Streamable HTTP 完整实现 |
| **AI 摘要/分类/标签** | 阿里云 Qwen 系列 | - | 单次调用产出三项，通过统一 LLMProvider 抽象调用 |
| **AI 向量模型** | BAAI bge-large-zh-v1.5 | - | 1024 维，免费 API（与 VECTOR(1024) 匹配） |
| **AI 重排模型** | BAAI bge-reranker-v2-m3 | - | 免费 API |

### 3.2 整体架构图

```
┌──────────────────────────────────────────────────────────────────────────┐
│                            客户端层                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────────┐ │
│  │   Web UI     │  │  内部系统    │  │  Claude Desktop/Codex        │ │
│  │ (Alpine.js)  │  │  (curl/脚本) │  │  (MCP Client)                │ │
│  └──────┬───────┘  └──────┬───────┘  └──────────┬───────────────────┘ │
└─────────┼─────────────────┼──────────────────────┼────────────────────┘
          │                 │                      │
          ▼                 ▼                      ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                         aiknow-server (Go)                              │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────────┐ │
│  │  Web UI 路由      │  │  RESTful API     │  │  MCP Streamable HTTP │ │
│  │  /ui/*           │  │  /api/v1/*       │  │  /mcp               │ │
│  └────────┬─────────┘  └────────┬─────────┘  └──────────┬───────────┘ │
│           │                     │                       │              │
│           └─────────────────────┼───────────────────────┘              │
│                                 │                                       │
│  ┌──────────────────────────────▼────────────────────────────────────┐ │
│  │                    核心业务逻辑层（Service）                        │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────┐│ │
│  │  │采集服务   │ │AI服务    │ │检索服务   │ │管理服务   │ │MCP服务 ││ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └────────┘│ │
│  └──────────────────────────────┬────────────────────────────────────┘ │
└─────────────────────────────────┼──────────────────────────────────────┘
                                  │
                                  ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                      PostgreSQL 17（单一数据层）                         │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌──────────────────┐ │
│  │  pgvector   │ │pg_textsearch│ │  zhparser   │ │      PGMQ        │ │
│  │ (向量存储)   │ │ (BM25索引)  │ │(中文分词)   │ │   (消息队列)      │ │
│  └─────────────┘ └─────────────┘ └─────────────┘ └──────────────────┘ │
│                         ┌──────────────────────────────────────────┐    │
│                         │     业务数据表 + 向量 + BM25 索引        │    │
│                         └──────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                        外部 AI 服务（云 API）                           │
│  ┌─────────────────────┐  ┌────────────────────┐  ┌──────────────────┐ │
│  │  Qwen (LLMProvider) │  │ BAAI Embedding     │  │BAAI Reranker     │ │
│  │ (单次调用产出三项)   │  │ (向量生成 1024维)  │  │ (精排)           │ │
│  └─────────────────────┘  └────────────────────┘  └──────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
```

### 3.3 核心工作流程

#### 3.3.1 文章录入与 AI 处理流程

```
用户提交文章 (手动/URL)
    │
    ▼
API 接收请求，创建文章记录 (status='pending')
    │
    ▼
将任务发送至 PGMQ 队列 (立即返回 task_id)
    │
    ▼ (异步)
Worker 消费消息
    │
    ├─ URL 采集 → 提取标题、正文、纯文本
    │
    ├─ 调用 Qwen API（单次，JSON 输出）→ 摘要 + 分类 + 标签
    │      └─ 校验 JSON schema 与业务约束，不合法则重试
    │
    ├─ 文本切分 → 生成 Chunks (200-500字)
    │
    ├─ 调用 BAAI Embedding → 为每个 Chunk 生成 1024 维向量
    │
    └─ 更新文章状态 (status='completed')
        ├─ 写入摘要、分类、标签
        ├─ 写入 Chunks + 向量
        └─ 更新 indexed_at, embedding_model, embedding_dim
```

#### 3.3.2 混合检索流程

```
用户 Query
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│  检索分支 1：语义检索 (pgvector)                           │
│  1. 调用 BAAI Embedding → Query 向量 (1024维)            │
│  2. 执行 pgvector HNSW 索引 → 返回 Top 20 Chunk          │
└─────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│  检索分支 2：关键词检索 (pg_textsearch BM25)              │
│  1. 使用 zhparser 分词 Query                              │
│  2. 执行 pg_textsearch BM25 索引 → 返回 Top 20 Chunk     │
└─────────────────────────────────────────────────────────────┘
    │
    ▼
RRF (倒数排名融合) → 合并两个结果集 → Top 20 候选
    │
    ▼
调用 BAAI Reranker API → 精排 → Top 5
    │
    ▼
根据 Chunk ID 查询父文章 → 补全标题、上下文、引用信息
    │
    ▼
返回结构化结果 (证据块 + 标题 + 来源 + chunk位置)
```


## 四、数据模型

### 4.1 核心表结构

#### 4.1.1 文章表（articles）

```sql
CREATE TABLE articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(500) NOT NULL,
    content TEXT,                      -- 原始内容（Markdown/HTML）
    plain_text TEXT,                   -- 纯文本内容（用于 AI 和检索）
    summary TEXT,                      -- AI 生成的摘要
    category_id UUID REFERENCES categories(id),  -- 分类（外键）
    status SMALLINT DEFAULT 0,         -- 0-草稿，1-已发布，2-待审核
    source_url VARCHAR(1000),
    source_hash VARCHAR(64),           -- 内容去重哈希
    author VARCHAR(200),
    view_count BIGINT DEFAULT 0,
    
    -- AI 处理状态追踪
    processing_status VARCHAR(20) DEFAULT 'pending',  -- pending/processing/completed/failed
    retry_count INT DEFAULT 0,
    last_error TEXT,
    processed_at TIMESTAMP,
    embedding_model VARCHAR(100),      -- 记录使用的 Embedding 模型
    embedding_dim INT DEFAULT 1024,    -- 记录向量维度（默认 1024）
    indexed_at TIMESTAMP,              -- 向量索引构建时间
    
    -- 软删除
    deleted_at TIMESTAMP,              -- 非空表示已删除
    deleted_by VARCHAR(100),           -- 删除来源：admin 或 API Key 前缀
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

> **删除追溯**：v1 不做通用审计日志表，但删除是不可逆操作，因此在软删除的基础上记录 `deleted_by`，保证「数据是谁删的」可查。所有列表查询默认附加 `WHERE deleted_at IS NULL`。

#### 4.1.2 文章-标签关联表（article_tags）

```sql
CREATE TABLE article_tags (
    article_id UUID REFERENCES articles(id) ON DELETE CASCADE,
    tag_id UUID REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (article_id, tag_id)
);
```

#### 4.1.3 标签表（tags）

```sql
CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    color VARCHAR(20),
    created_at TIMESTAMP DEFAULT NOW()
);
```

#### 4.1.4 分类表（categories）

```sql
CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    parent_id UUID REFERENCES categories(id),
    created_at TIMESTAMP DEFAULT NOW()
);
```

#### 4.1.5 文档块表（chunks）

```sql
CREATE TABLE chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id UUID REFERENCES articles(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL,          -- 块在文章中的顺序
    content TEXT NOT NULL,             -- 块内容（200-500字）
    embedding VECTOR(1024),            -- 1024 维向量（匹配 bge-large-zh-v1.5）
    created_at TIMESTAMP DEFAULT NOW()
);
```

#### 4.1.6 系统配置表（settings）

存储可在后台修改的运行时业务配置，采用 key-value 结构，值使用 JSONB 以支持嵌套结构。

```sql
CREATE TABLE settings (
    key VARCHAR(100) PRIMARY KEY,      -- 配置键，如 'ai.llm', 'search', 'prompt.analyze'
    value JSONB NOT NULL,              -- 配置值
    description TEXT,                  -- 配置说明（后台展示）
    updated_at TIMESTAMP DEFAULT NOW()
);
```

**配置键规划**：

| key | 说明 |
| :--- | :--- |
| `ai.llm` | LLM 模型名、temperature、max_tokens、超时、重试次数 |
| `ai.embedding` | Embedding 模型名、批量大小、超时、重试次数 |
| `ai.reranker` | Reranker 模型名、超时、重试次数 |
| `prompt.analyze` | 内容分析提示词模板（单次产出摘要+分类+标签） |
| `search` | top_k、candidate_k、rrf_k、vector_weight、bm25_weight、ef_search |
| `keyword_search` | 关键词检索各命中位置的权重 |
| `collector` | 超时、最大重定向、User-Agent、正文最小长度 |
| `chunk` | 切分大小、重叠字数 |
| `mcp_log` | MCP 请求日志保留天数 |

#### 4.1.7 配置变更记录表（setting_history）

```sql
CREATE TABLE setting_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(100) NOT NULL,
    old_value JSONB,
    new_value JSONB NOT NULL,
    changed_at TIMESTAMP DEFAULT NOW()
);
```

#### 4.1.8 API Key 表（api_keys）

从 `config.yaml` 迁移到数据库，以支持后台动态管理。**仅存储哈希值**，不存明文。

```sql
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash VARCHAR(64) NOT NULL UNIQUE,  -- SHA-256 哈希
    key_prefix VARCHAR(16) NOT NULL,       -- 前缀，用于列表展示（如 'ak-a1b2c3'）
    scope VARCHAR(20) NOT NULL,            -- readonly / readwrite
    description VARCHAR(200),
    enabled BOOLEAN DEFAULT TRUE,
    last_used_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);
```

> **说明**：API Key 查询走 `key_hash` 唯一索引，验证时对请求携带的明文做 SHA-256 后比对。这里用 SHA-256 而非 bcrypt，因为 API Key 是高熵随机串（不存在字典攻击风险），且每次请求都要验证，bcrypt 的计算开销不可接受。

#### 4.1.9 MCP 请求日志表（mcp_request_logs）

记录每一次 MCP 调用的完整请求与响应，用于问题排查和调用回放。

```sql
CREATE TABLE mcp_request_logs (
    id BIGSERIAL PRIMARY KEY,
    trace_id VARCHAR(64) NOT NULL,         -- 客户端传入或服务端生成
    session_id VARCHAR(64),                -- MCP 会话 ID
    method VARCHAR(50) NOT NULL,           -- initialize / tools/list / tools/call
    tool_name VARCHAR(100),                -- method='tools/call' 时的工具名
    api_key_prefix VARCHAR(16),            -- 调用方（未鉴权时为 NULL）
    client_ip VARCHAR(45),
    
    request_body JSONB NOT NULL,           -- 完整 JSON-RPC 请求
    response_body JSONB,                   -- 完整 JSON-RPC 响应
    response_headers JSONB,                -- 关键响应头
    
    http_status INT,
    is_error BOOLEAN DEFAULT FALSE,        -- 响应中 isError 或 HTTP 非 2xx
    error_message TEXT,
    elapsed_ms INT,
    
    -- 检索类调用的结果快照，便于回溯「当时返回了什么」
    result_summary JSONB,                  -- 如 [{chunk_id, article_id, title, score}]
    
    created_at TIMESTAMP DEFAULT NOW()
);
```

**字段说明**：

| 字段 | 用途 |
| :--- | :--- |
| `trace_id` | 优先取请求头 `X-Trace-Id`，未提供则服务端生成 UUID；响应头回写，便于与调用方日志关联 |
| `request_body` / `response_body` | 存完整 JSON，支持一键重放到调试页 |
| `result_summary` | `search_knowledge` 的命中列表与分数，用于回答「为什么这条没被搜到」 |
| `is_error` | 覆盖两类失败：JSON-RPC 层的 `isError=true` 和 HTTP 层的非 2xx |

> **写入方式**：日志写入在响应返回后异步进行（投递到 PGMQ 或本地 channel），不阻塞 MCP 请求主链路。写入失败仅记录服务端日志，不影响调用结果。

> **保留策略**：默认 30 天，`pg_cron` 每日清理 `created_at < NOW() - INTERVAL 'N days'` 的记录，N 由 `mcp_log.retention_days` 配置。

### 4.2 索引策略

#### 4.2.1 向量索引（pgvector HNSW）

```sql
-- HNSW 索引（近似最近邻，需要调优 ef_search 参数）
CREATE INDEX idx_chunks_embedding_hnsw ON chunks 
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);
```

#### 4.2.2 BM25 索引（pg_textsearch + zhparser）

```sql
-- 1. 创建使用 zhparser 的中文分词配置
CREATE EXTENSION zhparser;
CREATE TEXT SEARCH CONFIGURATION chinese (PARSER = zhparser);
ALTER TEXT SEARCH CONFIGURATION chinese 
ADD MAPPING FOR n,v,a,i,e,l,j,r,d,m,q,t,z WITH simple;

-- 2. 创建 pg_textsearch BM25 索引
CREATE EXTENSION pg_textsearch;
CREATE INDEX idx_chunks_content_bm25 ON chunks 
USING bm25 (content) 
WITH (
    text_config = 'chinese',
    k1 = 1.2,
    b = 0.75
);
```

#### 4.2.3 其他索引

```sql
-- 文章标题全文检索（可选，用于快速标题搜索）
CREATE INDEX idx_articles_title_gin ON articles 
USING gin (to_tsvector('chinese', title));

-- 常用查询索引
CREATE INDEX idx_articles_category_id ON articles (category_id);
CREATE INDEX idx_articles_status ON articles (status);
CREATE INDEX idx_articles_created_at ON articles (created_at DESC);
CREATE INDEX idx_articles_source_hash ON articles (source_hash);
CREATE INDEX idx_articles_processing_status ON articles (processing_status);

-- 软删除：未删除的文章占绝大多数查询，用部分索引
CREATE INDEX idx_articles_not_deleted ON articles (created_at DESC)
WHERE deleted_at IS NULL;

-- 关键词检索：标签/分类名的大小写不敏感精确匹配
CREATE INDEX idx_tags_name_lower ON tags (lower(name));
CREATE INDEX idx_categories_name_lower ON categories (lower(name));

-- 关键词检索：摘要全文检索（标题已有 idx_articles_title_gin）
CREATE INDEX idx_articles_summary_gin ON articles
USING gin (to_tsvector('chinese', coalesce(summary, '')));

-- 反向查标签：由 tag_id 找文章
CREATE INDEX idx_article_tags_tag_id ON article_tags (tag_id);

-- MCP 请求日志索引
CREATE INDEX idx_mcp_logs_created_at ON mcp_request_logs (created_at DESC);
CREATE INDEX idx_mcp_logs_trace_id ON mcp_request_logs (trace_id);
CREATE INDEX idx_mcp_logs_tool_name ON mcp_request_logs (tool_name, created_at DESC);
CREATE INDEX idx_mcp_logs_api_key ON mcp_request_logs (api_key_prefix, created_at DESC);
CREATE INDEX idx_mcp_logs_error ON mcp_request_logs (created_at DESC) WHERE is_error = TRUE;

-- 按请求参数内容检索（如查所有含「Docker」的 query）
CREATE INDEX idx_mcp_logs_request_gin ON mcp_request_logs USING gin (request_body jsonb_path_ops);

-- 其他
CREATE INDEX idx_setting_history_key ON setting_history (key, changed_at DESC);
CREATE INDEX idx_api_keys_enabled ON api_keys (enabled);
```


## 五、接口设计

### 5.1 RESTful API

| 方法 | 路径 | 功能 | 鉴权 |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/articles` | 录入文章（手动/URL） | ✅ API Key |
| `GET` | `/api/v1/articles` | 分页列表 + 关键词搜索 | ✅ API Key |
| `GET` | `/api/v1/articles/:id` | 获取文章详情 | ✅ API Key |
| `PUT` | `/api/v1/articles/:id` | 更新文章（含人工修正） | ✅ API Key |
| `DELETE` | `/api/v1/articles/:id` | 软删除文章 | ✅ API Key |
| `POST` | `/api/v1/articles/:id/reprocess` | 重新触发 AI 处理 | ✅ API Key |
| `GET` | `/api/v1/articles/:id/status` | 查询 AI 处理状态 | ✅ API Key |
| `POST` | `/api/v1/search` | 混合检索（返回 Top 5 精排结果） | ✅ API Key |
| `POST` | `/api/v1/search/keywords` | 关键词检索文章（标签/分类加权） | ✅ API Key |
| `GET` | `/api/v1/health` | 健康检查 | ❌ |

#### 5.1.1 文章录入请求示例

```json
POST /api/v1/articles
{
    "title": "Docker 生产环境部署指南",
    "content": "...（Markdown 正文）...",
    "source_url": "https://example.com/article",
    "category_id": "uuid-xxx",           // 方式1：直接指定分类 ID
    "category_name": "技术",              // 方式2：通过名称（不存在则自动创建）
    "tags": ["Docker", "运维", "部署"]    // 标签名称数组（不存在则自动创建）
}
```

**说明**：
- `category_id` 和 `category_name` 二选一，优先使用 `category_id`
- 分类通过 `category_name` 指定但不存在时，自动创建新分类
- 标签不存在时自动创建，重复标签自动去重

#### 5.1.2 检索请求示例

```json
POST /api/v1/search
{
    "query": "如何配置 Docker 容器网络",
    "top_k": 5,          // 最终返回数量
    "candidate_k": 20,   // RRF 召回候选数
    "rrf_k": 60,         // RRF 倒数排名融合参数
    "filters": {
        "category": "技术",
        "tags": ["Docker"]
    }
}
```

#### 5.1.3 检索响应示例

```json
{
    "results": [
        {
            "chunk_id": "uuid",
            "article_id": "uuid",
            "title": "Docker 生产环境部署指南",
            "content": "容器网络配置需注意...（证据块内容）",
            "chunk_index": 3,
            "score": 0.92,
            "source_url": "https://example.com/article",
            "created_at": "2026-09-01T10:00:00Z"
        }
    ],
    "total": 5,
    "query_time_ms": 248
}
```

#### 5.1.4 关键词检索请求/响应示例

```json
POST /api/v1/search/keywords
{
    "keywords": ["Docker", "容器网络"],
    "limit": 5,
    "match_mode": "any"       // any（默认）/ all
}
```

```json
{
    "results": [
        {
            "article_id": "uuid-1",
            "title": "Docker 生产环境部署指南",
            "summary": "本文介绍 Docker 在生产环境中的部署要点……",
            "category": "技术",
            "tags": ["Docker", "容器网络", "生产部署"],
            "source_url": "https://example.com/article",
            "score": 23.5,
            "matched": {
                "Docker": ["tag_exact", "title"],
                "容器网络": ["tag_exact", "content"]
            },
            "created_at": "2026-09-01T10:00:00Z"
        }
    ],
    "total": 2,
    "query_time_ms": 42
}
```

该接口与 MCP 的 `find_articles_by_keywords` 共用同一套检索逻辑，仅返回格式不同（REST 返回原始 JSON，MCP 包装为 `content` + `annotations`）。评分规则见 [5.2.5](#525-find_articles_by_keywords)。

### 5.2 MCP Streamable HTTP 接口

#### 5.2.1 协议规范

- **传输方式**：Streamable HTTP（MCP 2025-06-18 规范）
- **端点**：`/mcp`
- **支持方法**：`POST` 和 `GET`
- **请求头**：
  - `Content-Type: application/json`（POST）
  - `Accept: application/json, text/event-stream`（支持 SSE）
  - `MCP-Protocol-Version: 2025-06-18`
  - `Authorization: Bearer <api_key>`

#### 5.2.2 端点行为

| 方法 | 场景 | 响应 |
| :--- | :--- | :--- |
| **GET** | 连接协商、协议版本检测、健康检查 | 返回 `200 OK` + 协议信息 |
| **POST** | 发送 MCP 请求（initialize、tools/list、tools/call） | 标准 JSON 响应或 SSE 流 |

#### 5.2.3 MCP Tools（v1 共 4 个）

| Tool 名称 | 功能 | 所需 scope |
| :--- | :--- | :--- |
| `search_knowledge` | 语义检索，返回**证据块**（chunk 级） | readonly |
| `find_articles_by_keywords` | 关键词检索，返回**文章卡片**（article 级） | readonly |
| `get_article_detail` | 获取文章全文 | readonly |
| `add_article_by_url` | 提交 URL 采集入库 | readwrite |

**两个检索工具的分工**：这两者返回粒度不同，不是重复功能。

| | `search_knowledge` | `find_articles_by_keywords` |
| :--- | :--- | :--- |
| **输入** | 自然语言问句 | 一个或多个关键词 |
| **返回粒度** | Chunk 片段 | 整篇文章的卡片信息 |
| **检索方式** | 向量 + BM25 + RRF + Rerank | 标签/分类精确匹配 + 全文检索加权 |
| **适用场景** | 「Docker 容器网络怎么配置」——要具体答案依据 | 「Docker、K8s」——要找相关文章列表 |
| **典型用途** | RAG 证据供给 | 文章发现、主题浏览 |

工具描述（`description`）中会写明上述区别，引导调用方大模型正确选择。

#### 5.2.4 `search_knowledge`

**参数**：

| 参数 | 类型 | 必填 | 说明 |
| :--- | :--- | :---: | :--- |
| `query` | string | ✅ | 自然语言问句 |
| `limit` | integer | ❌ | 返回数量，默认 5，范围 1-20 |

**返回**：结构化证据块，而非直接生成的答案。答案生成由调用方大模型（如 Claude）完成，更符合 RAG 工具职责。

```json
{
    "content": [
        {
            "type": "text",
            "text": "找到 2 个相关文档片段"
        },
        {
            "type": "resource",
            "resource": {
                "uri": "aiknow://article/uuid-1/chunk/3",
                "name": "Docker 生产环境部署指南 - 第3节",
                "description": "容器网络配置需注意桥接模式和主机模式的差异...",
                "mimeType": "text/plain"
            }
        },
        {
            "type": "resource",
            "resource": {
                "uri": "aiknow://article/uuid-2/chunk/1",
                "name": "Kubernetes 网络策略 - 第2.1节",
                "description": "网络策略通过标签选择器控制 Pod 间通信...",
                "mimeType": "text/plain"
            }
        }
    ],
    "isError": false
}
```

**annotations 格式**（可选，用于调试）：
```json
{
    "annotations": {
        "references": [
            {
                "title": "Docker 生产环境部署指南",
                "article_id": "uuid-1",
                "chunk_index": 3,
                "source_url": "https://example.com/article",
                "relevance_score": 0.92
            },
            {
                "title": "Kubernetes 网络策略",
                "article_id": "uuid-2",
                "chunk_index": 1,
                "source_url": "https://example.com/k8s",
                "relevance_score": 0.85
            }
        ],
        "query_time_ms": 248,
        "search_params": {
            "rrf_k": 60,
            "candidate_k": 20,
            "vector_weight": 0.5,
            "bm25_weight": 0.5
        }
    }
}
```

#### 5.2.5 `find_articles_by_keywords`

输入一个或多个关键词，返回最相关的文章卡片。**标签和分类命中的权重高于正文命中**——用户给出关键词时，通常期望的是"打了这个标签的文章"，而不是"正文里恰好提到这个词的文章"。

**参数**：

| 参数 | 类型 | 必填 | 说明 |
| :--- | :--- | :---: | :--- |
| `keywords` | string[] | ✅ | 关键词数组，1-10 个，单个不超过 50 字 |
| `limit` | integer | ❌ | 返回数量，默认 5，范围 1-20 |
| `match_mode` | string | ❌ | `any`（默认，命中任一关键词即可）/ `all`（须命中全部关键词） |

**评分模型**：对每个关键词分别计算命中得分，累加后排序。

| 命中位置 | 权重 | 匹配方式 |
| :--- | :---: | :--- |
| **标签精确匹配** | 10.0 | `tags.name` 完全相等（忽略大小写） |
| **分类精确匹配** | 8.0 | `categories.name` 完全相等（忽略大小写） |
| **标题命中** | 3.0 | 标题全文检索命中 |
| **摘要命中** | 1.5 | 摘要全文检索命中 |
| **正文命中** | 1.0 | `plain_text` 全文检索命中，按 BM25 分数归一化后乘以权重 |

> **标签与分类只做精确匹配**：标签是原子词，substring 匹配会产生假阳性——`Java` 会命中 `JavaScript`，`云` 会命中 `云原生`、`阿里云`、`云计算`，几乎等于不筛选。而标签词本身通常也出现在标题或正文中，已由后三档全文检索（zhparser 分词）覆盖，部分匹配不提供新信息，只会给假阳性开一个高权重通道。

**最终得分**：`score = Σ(每个关键词的各项命中权重之和)`

多关键词命中数量本身也是信号，因此不做平均——命中 3 个关键词的文章自然高于只命中 1 个的。`match_mode=all` 时先过滤掉未命中全部关键词的文章，再排序。

**权重可配置**：上述 5 个权重值放入 L2 配置 `keyword_search`，后台可调。默认值经验取值，实际投用后按效果调整。

**返回格式**：

```json
{
    "content": [
        {
            "type": "text",
            "text": "找到 2 篇相关文章（关键词：Docker、容器网络）"
        },
        {
            "type": "resource",
            "resource": {
                "uri": "aiknow://article/uuid-1",
                "name": "Docker 生产环境部署指南",
                "description": "本文介绍 Docker 在生产环境中的部署要点，涵盖容器网络配置、资源限制、日志收集与监控告警……\n分类：技术 | 标签：Docker, 容器网络, 生产部署\n来源：https://example.com/article",
                "mimeType": "text/plain"
            }
        },
        {
            "type": "resource",
            "resource": {
                "uri": "aiknow://article/uuid-2",
                "name": "Kubernetes 网络策略实践",
                "description": "网络策略通过标签选择器控制 Pod 间通信，本文给出常见场景的配置示例……\n分类：技术 | 标签：Kubernetes, 容器网络\n来源：https://example.com/k8s",
                "mimeType": "text/plain"
            }
        }
    ],
    "isError": false
}
```

**annotations**（结构化字段，便于调用方程序化处理）：

```json
{
    "annotations": {
        "articles": [
            {
                "article_id": "uuid-1",
                "title": "Docker 生产环境部署指南",
                "summary": "本文介绍 Docker 在生产环境中的部署要点……",
                "category": "技术",
                "tags": ["Docker", "容器网络", "生产部署"],
                "source_url": "https://example.com/article",
                "score": 23.5,
                "matched": {
                    "Docker": ["tag_exact", "title"],
                    "容器网络": ["tag_exact", "content"]
                },
                "created_at": "2026-09-01T10:00:00Z"
            }
        ],
        "keywords": ["Docker", "容器网络"],
        "match_mode": "any",
        "query_time_ms": 42
    }
}
```

> **`matched` 字段的用途**：显式说明每个关键词是通过哪种方式命中的，让调用方大模型能判断结果的相关性强度——「标签精确匹配」比「正文提及」是更强的信号。

**无结果时**：返回 `content` 为一条 text（`"未找到匹配「xxx」的文章"`），`isError` 为 `false`。检索无结果是正常业务情况，不是错误。

#### 5.2.6 `get_article_detail`

**参数**：

| 参数 | 类型 | 必填 | 说明 |
| :--- | :--- | :---: | :--- |
| `article_id` | string | ✅ | 文章 UUID |

返回文章全文、摘要、分类、标签、来源 URL、创建时间。文章不存在或已软删除时返回 `isError: true` 并说明原因。

#### 5.2.7 `add_article_by_url`

提交一个 URL，服务端采集入库并异步做 AI 处理。**这是 v1 唯一的写入工具，需要 `readwrite` scope**。

**参数**：

| 参数 | 类型 | 必填 | 说明 |
| :--- | :--- | :---: | :--- |
| `url` | string | ✅ | 待采集的网页 URL |
| `category_name` | string | ❌ | 指定分类；不填则由 AI 自动分类 |
| `tags` | string[] | ❌ | 附加标签，与 AI 生成的标签合并去重 |

**行为**：与 `POST /api/v1/articles` 完全一致，复用同一套采集与入库逻辑，包括 SSRF 防护、去重、异步处理。**立即返回**，不等待 AI 处理完成。

**返回（成功入队）**：

```json
{
    "content": [
        {
            "type": "text",
            "text": "已提交采集：《Docker 生产环境部署指南》\narticle_id: uuid-1\n状态：pending（AI 处理中，通常需要 10-30 秒）\n可稍后用 get_article_detail 查看处理结果。"
        }
    ],
    "isError": false
}
```

**返回（命中去重）**：

```json
{
    "content": [
        {
            "type": "text",
            "text": "该 URL 已存在于知识库中：《Docker 生产环境部署指南》\narticle_id: uuid-1\n未重复入库。"
        }
    ],
    "isError": false
}
```

去重命中**不算错误**——对调用方而言"内容已在库里"是一个有效的结果，返回 `isError: true` 会让大模型误以为操作失败而重试。

**错误情况**：

| 情况 | `isError` | 返回内容 |
| :--- | :---: | :--- |
| scope 不足（readonly key） | — | HTTP `403`，不进入工具执行 |
| URL 格式非法 / 非 HTTP(S) | `true` | 说明协议限制 |
| 命中 SSRF 拦截（内网 IP 等） | `true` | 「目标地址不允许访问」，**不回显解析出的内网 IP** |
| 采集超时 / 目标站点不可达 | `true` | 说明具体原因 |
| 正文过短（低于 `min_content_length`） | `true` | 「未能提取到有效正文」 |

> **安全说明**：SSRF 拦截的错误信息统一措辞，不回显 DNS 解析结果或具体拦截原因。否则该工具会变成一个内网探测器——调用方可以通过错误信息的差异推断内网哪些 IP 存活。

**速率限制**：该工具受写操作限流约束（默认 20 req/min），且与 REST API 的写限流共享同一计数器（按 API Key 维度）。

#### 5.2.8 安全要求

- ✅ 必须验证 `Authorization` 头，返回 `401 Unauthorized` 或 `403 Forbidden`
- ✅ **写入工具必须校验 scope**：`readonly` Key 调用 `add_article_by_url` 返回 `403`
- ✅ `tools/list` 按 Key 的 scope 过滤：`readonly` Key 看不到 `add_article_by_url`
- ✅ 必须校验 `Origin` 头，防止 CSRF
- ✅ 必须在响应中包含 `MCP-Protocol-Version` 头
- ✅ POST 请求支持 `text/event-stream`（用于长时间检索的进度推送）
- ✅ 写入工具的 SSRF 防护与 REST API 采集完全一致，不因入口不同而放宽

> **`tools/list` 按 scope 过滤的理由**：让只读客户端根本看不到写工具比让它看到却调用失败更合理——大模型不会反复尝试一个注定 403 的工具，也避免在对话中暴露"存在但你不能用"的能力。

### 5.3 后台管理 API

后台专用接口，前缀 `/api/v1/admin`，使用 **Session Cookie 鉴权**（不接受 API Key），并要求 CSRF Token。v1 只有单一管理员账号，因此不做角色区分。

| 方法 | 路径 | 功能 |
| :--- | :--- | :--- |
| `POST` | `/api/v1/admin/login` | 登录，返回 Session Cookie |
| `POST` | `/api/v1/admin/logout` | 登出 |
| `GET` | `/api/v1/admin/me` | 当前登录信息 |
| `GET` | `/api/v1/admin/dashboard` | 仪表盘聚合数据 |
| `GET` | `/api/v1/admin/queue/status` | PGMQ 队列深度与 DLQ 统计 |
| `POST` | `/api/v1/admin/articles/batch` | 批量重新处理 / 批量删除 |
| `GET` | `/api/v1/admin/articles/:id/chunks` | 查看文章的 Chunk 列表 |
| `POST` | `/api/v1/admin/search/debug` | 检索调试，返回分阶段中间结果 |
| `POST` | `/api/v1/admin/search/keywords/debug` | 关键词检索调试，返回得分构成（支持临时权重覆盖） |
| `POST` | `/api/v1/admin/mcp/invoke` | MCP 调试代理，转发 JSON-RPC 请求 |
| `GET` | `/api/v1/admin/mcp/logs` | MCP 请求日志列表（分页 + 筛选 + 全文检索） |
| `GET` | `/api/v1/admin/mcp/logs/:id` | 单条日志详情（完整请求/响应） |
| `GET` | `/api/v1/admin/settings` | 获取全部业务配置（含默认值与当前值） |
| `PUT` | `/api/v1/admin/settings/:key` | 更新单组配置 |
| `POST` | `/api/v1/admin/settings/:key/reset` | 恢复该组默认值 |
| `GET` | `/api/v1/admin/settings/history` | 配置变更记录 |
| `POST` | `/api/v1/admin/prompts/test` | 用当前提示词试跑指定文章 |
| `GET` | `/api/v1/admin/api-keys` | API Key 列表 |
| `POST` | `/api/v1/admin/api-keys` | 创建 API Key（返回一次明文） |
| `PATCH` | `/api/v1/admin/api-keys/:id` | 启用 / 禁用 |
| `DELETE` | `/api/v1/admin/api-keys/:id` | 删除 |

#### 5.3.1 MCP 调试代理接口

请求：

```json
POST /api/v1/admin/mcp/invoke
{
    "method": "tools/call",              // initialize / tools/list / tools/call
    "params": {
        "name": "search_knowledge",
        "arguments": { "query": "Docker 网络配置", "limit": 5 }
    },
    "auth_mode": "session",              // session：以管理员身份；api_key：用指定 Key 验证 scope
    "api_key_id": null,                  // auth_mode=api_key 时必填
    "stream": false                      // true 时以 SSE 方式请求并聚合事件
}
```

响应：

```json
{
    "request": {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/call",
        "params": { "name": "search_knowledge", "arguments": { "query": "Docker 网络配置", "limit": 5 } }
    },
    "response": {
        "jsonrpc": "2.0",
        "id": 1,
        "result": {
            "content": [ /* 见 5.2.4 */ ],
            "isError": false
        }
    },
    "response_headers": {
        "MCP-Protocol-Version": "2025-06-18",
        "Mcp-Session-Id": "sess-xxx"
    },
    "http_status": 200,
    "elapsed_ms": 263,
    "events": []                          // stream=true 时按顺序记录 SSE 事件
}
```

**设计说明**：返回体同时包含 `request` 和 `response`，让调试页面可以展示完整的请求-响应对，便于复制到其他客户端复现问题。

#### 5.3.2 检索调试接口

请求参数与 `/api/v1/search` 一致，额外返回各阶段中间结果：

```json
{
    "query_tokens": ["Docker", "容器", "网络", "配置"],
    "stages": {
        "vector_recall": [ { "chunk_id": "uuid", "rank": 1, "distance": 0.18 } ],
        "bm25_recall":   [ { "chunk_id": "uuid", "rank": 1, "bm25_score": 12.4 } ],
        "rrf_fused":     [ { "chunk_id": "uuid", "rank": 1, "rrf_score": 0.032,
                             "vector_rank": 1, "bm25_rank": 3 } ],
        "reranked":      [ { "chunk_id": "uuid", "rank": 1, "rerank_score": 0.92,
                             "rrf_rank_before": 3 } ]
    },
    "timings_ms": {
        "embedding": 82, "vector_recall": 21, "bm25_recall": 14,
        "rrf": 1, "rerank": 145, "context_fill": 8, "total": 271
    }
}
```

#### 5.3.3 配置更新接口

```json
PUT /api/v1/admin/settings/ai.llm
{
    "value": {
        "model": "qwen-plus",
        "temperature": 0.3,
        "max_tokens": 2000,
        "timeout": "30s",
        "max_retries": 3
    }
}
```

校验失败返回 `400`，并指明具体字段：

```json
{
    "error": "validation_failed",
    "details": [
        { "field": "temperature", "message": "取值需在 0.0 - 2.0 之间" }
    ]
}
```

#### 5.3.4 MCP 请求日志查询接口

```
GET /api/v1/admin/mcp/logs
  ?start=2026-09-01T00:00:00Z
  &end=2026-09-07T23:59:59Z
  &method=tools/call
  &tool_name=search_knowledge
  &api_key_prefix=ak-a1b2c3
  &is_error=true
  &min_elapsed_ms=1000
  &q=Docker                  # 按 request_body 内容全文检索
  &trace_id=xxx              # 精确查询，与其他条件互斥
  &page=1&page_size=20
```

响应（列表）：

```json
{
    "items": [
        {
            "id": 12345,
            "trace_id": "9f8e7d6c-...",
            "created_at": "2026-09-07T14:23:11Z",
            "method": "tools/call",
            "tool_name": "search_knowledge",
            "api_key_prefix": "ak-a1b2c3",
            "elapsed_ms": 263,
            "http_status": 200,
            "is_error": false,
            "request_preview": "{\"query\":\"Docker 网络配置\",\"limit\":5}",
            "result_count": 5
        }
    ],
    "total": 1284,
    "page": 1,
    "page_size": 20
}
```

响应（详情 `GET /api/v1/admin/mcp/logs/:id`）：返回该记录的全部字段，含完整 `request_body`、`response_body`、`response_headers`、`result_summary`，可直接用于重放。

#### 5.3.5 关键词检索调试接口

```json
POST /api/v1/admin/search/keywords/debug
{
    "keywords": ["Docker", "容器网络"],
    "limit": 5,
    "match_mode": "any",
    "weights_override": {          // 可选，仅本次生效，不写入配置
        "tag_exact": 15.0,
        "content": 0.5
    }
}
```

响应在正式接口基础上增加每篇文章的得分构成：

```json
{
    "results": [
        {
            "article_id": "uuid-1",
            "title": "Docker 生产环境部署指南",
            "score": 23.5,
            "score_breakdown": [
                { "keyword": "Docker",   "position": "tag_exact", "weight": 10.0, "contribution": 10.0 },
                { "keyword": "Docker",   "position": "title",     "weight": 3.0,  "contribution": 3.0 },
                { "keyword": "容器网络", "position": "tag_exact", "weight": 10.0, "contribution": 10.0 },
                { "keyword": "容器网络", "position": "content",   "weight": 1.0,  "contribution": 0.5 }
            ]
        }
    ],
    "weights_used": { "tag_exact": 10.0, "category_exact": 8.0,
                      "title": 3.0, "summary": 1.5, "content": 1.0 },
    "query_time_ms": 42
}
```

> **`contribution` 与 `weight` 的差异**：正文命中的贡献值是 BM25 分数归一化后乘以权重，因此可能小于权重本身；其余位置为精确/全文命中，命中即贡献满额权重。


## 六、安全需求

### 6.1 防 SSRF（Server-Side Request Forgery）

| 控制点 | 措施 | 优先级 |
| :--- | :--- | :--- |
| **URL 白名单** | 仅允许 HTTP/HTTPS 协议 | P0 |
| **内网 IP 黑名单** | 禁止访问 10.0.0.0/8、172.16.0.0/12、192.168.0.0/16、127.0.0.0/8 | P0 |
| **DNS 重绑定防护** | 对域名进行两次解析比对 | P0 |
| **端口限制** | 仅允许 80、443、8080、8443 等常用端口 | P0 |
| **超时设置** | 连接超时 5s，读取超时 10s | P0 |
| **重定向限制** | 限制最大重定向次数为 3 次 | P0 |

### 6.2 防 XSS（Cross-Site Scripting）

| 控制点 | 措施 | 优先级 |
| :--- | :--- | :--- |
| **HTML 清洗** | 文章内容展示前使用 `bluemonday` 等库清洗 HTML | P0 |
| **CSP 头** | 设置 `Content-Security-Policy` 头限制脚本来源 | P0 |
| **输出转义** | Go 模板默认转义，前端避免 `innerHTML` 等危险操作 | P0 |

### 6.3 API 鉴权与安全

| 控制点 | 措施 | 优先级 |
| :--- | :--- | :--- |
| **API Key 认证** | 所有 API 请求必须携带 `Authorization: Bearer <key>` | P0 |
| **API Key Scope** | 支持只读/读写权限分离 | P0 |
| **速率限制** | 按 API Key 限流：读 100 req/min，写 20 req/min | P0 |
| **MCP 调用日志** | 记录所有 MCP 请求的完整入参出参、调用方、IP、trace_id | P0 |
| **删除追溯** | 文章软删除时记录 `deleted_by`，保留删除来源 | P0 |
| **API Key 轮换** | 支持定期轮换和撤销 | P1 |

> **v1 不做通用审计日志**：MVP 阶段只有单一管理员，「谁操作的」这一维度信息量有限。真正需要回溯的是 MCP 调用（排查检索效果）和删除操作（数据不可逆），这两项分别由 `mcp_request_logs` 表和 `articles.deleted_by` 字段覆盖。

### 6.4 Web 界面安全

| 控制点 | 措施 | 优先级 |
| :--- | :--- | :--- |
| **管理员认证** | 单管理员账号，凭据来自环境变量 / `.env` / 配置文件 | P0 |
| **密码存储** | 配置中存 bcrypt 哈希；若配置明文密码则启动时在内存中哈希，不落盘 | P0 |
| **Session 管理** | 使用安全的 HttpOnly Cookie，设置 SameSite=Strict | P0 |
| **登录失败限制** | 连续失败 5 次后锁定 15 分钟（锁定状态存内存） | P0 |
| **CSRF 防护** | 后台写操作要求 CSRF Token | P0 |

> **管理员账号说明**：v1 不设用户管理和密码修改功能，账号由部署方通过 `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH` 配置，改密码即改配置后重启。多用户、角色区分、密码自助修改留待 v2——在只有一个管理员的场景下，为此维护一张用户表和一组管理页面并不划算。


## 七、部署方案

### 7.1 部署架构

- **单二进制部署**：`aiknow-server` 编译为单一可执行文件
- **前端静态文件**：使用 Go `embed` 包将 HTML/CSS/JS 编译进二进制
- **数据库**：腾讯云 PostgreSQL 17（含 pgvector、pg_textsearch、zhparser、PGMQ、pg_cron）
- **反向代理**：可选 Nginx 作为前置网关（SSL 终结）

### 7.2 启动方式

```bash
# 开发模式
./aiknow-server --port 8080 --config ./config.yaml

# 或使用环境变量
DATABASE_URL="postgres://..." ./aiknow-server
```

### 7.3 配置分层设计

配置按**是否需要重启**和**是否为敏感信息**分为三层，职责不重叠：

| 层级 | 存储位置 | 内容 | 变更方式 |
| :--- | :--- | :--- | :--- |
| **L1 启动配置** | 环境变量 → `.env` → `config.yaml` | 连接串、密钥、端口、连接池、管理员账号 | 改后需重启 |
| **L2 业务配置** | 数据库 `settings` 表 | 模型参数、提示词、检索参数、采集参数 | 后台修改，热生效 |
| **L3 凭据数据** | 数据库 `api_keys` 表 | API Key 哈希 | 后台管理 |

**分层原则**：
- **密钥类**（API Key、数据库密码、管理员密码）只走 L1，绝不落到 `settings` 表或后台页面——避免在数据库和 UI 中扩散明文凭据。
- **模型名、温度、提示词**属于业务调优范畴，需要频繁试错，放数据库由后台改。
- **base_url、provider** 归入 L1：切换服务商属于部署级变更，通常伴随密钥更换，不适合热改。

#### 7.3.1 L1 配置来源优先级

按以下顺序查找，**先命中者生效**：

```
1. 环境变量          （最高优先级，适合容器/CI 注入）
2. .env 文件         （本地开发，不提交到版本库）
3. config.yaml       （默认值与非敏感配置，可提交）
4. 代码内置默认值     （兜底）
```

**实现要点**：
- 使用 `godotenv` 加载 `.env`，其默认行为是**不覆盖已存在的环境变量**，与上述优先级天然一致
- `.env` 文件不存在时静默跳过，不报错（生产环境通常只用环境变量）
- `.env` 必须写入 `.gitignore`，仓库中只提供 `.env.example`
- 加载完成后打印一行日志说明各关键配置的来源（如 `DATABASE_URL from: env`），但**不打印值**

**`.env` 文件示例**：

```bash
# 数据库
DATABASE_URL=postgres://user:pass@localhost:5432/aiknow

# AI 服务密钥
QWEN_API_KEY=sk-xxx
BAAI_API_KEY=xxx

# 后台管理员
ADMIN_USERNAME=admin
ADMIN_PASSWORD_HASH=$2a$10$xxxxx

# Session 签名密钥
SESSION_SECRET=xxxxx
```

#### 7.3.2 配置加载与热生效机制

```
启动时：
  环境变量 > .env > config.yaml > 内置默认值   → L1 配置（进程内不可变）
  从 settings 表读取全部 L2 配置              → 写入内存 ConfigCache

运行时：
  业务代码通过 ConfigCache.Get("ai.llm") 读取
  后台保存配置 → 写 settings 表 + setting_history → 刷新 ConfigCache
  多实例部署时：通过 PostgreSQL LISTEN/NOTIFY 广播变更，各实例收到后重载
```

**关键点**：`settings` 表中缺失的键回落到内置默认值，因此首次部署无需预先灌数据；「恢复默认」即删除该键对应的行。

#### 7.3.3 L1 启动配置（config.yaml）

```yaml
server:
  port: 8080                     # 可被 SERVER_PORT 覆盖
  read_timeout: 30s
  write_timeout: 30s
  cors_allowed_origins:
    - "https://ai-knowledge.your-company.com"

database:
  url: "postgres://user:pass@localhost:5432/aiknow"   # 建议用 DATABASE_URL 覆盖
  max_open_conns: 50
  max_idle_conns: 10

admin:
  username: "admin"              # 可被 ADMIN_USERNAME 覆盖
  password_hash: ""              # bcrypt 哈希，建议用 ADMIN_PASSWORD_HASH 注入
  session_ttl: 8h
  max_login_attempts: 5
  lockout_duration: 15m

ai:
  llm:
    provider: "qwen"                                    # 服务商，切换需重启
    api_key: "${QWEN_API_KEY}"                          # 建议只用环境变量
    base_url: "https://dashscope.aliyuncs.com/api/v1"
  embedding:
    provider: "baai"
    api_key: "${BAAI_API_KEY}"
    base_url: "https://api.baai.ac.cn/v1"
    dimension: 1024                                     # 与表结构绑定，不可热改
  reranker:
    provider: "baai"
    api_key: "${BAAI_API_KEY}"
    base_url: "https://api.baai.ac.cn/v1"

mcp:
  endpoint: "/mcp"
  enable_auth: true

session:
  secret: "${SESSION_SECRET}"     # 后台 Session 签名密钥，必须由环境变量提供

worker:
  concurrency: 4                  # PGMQ 消费者协程数
  poll_interval: 2s
```

> **注意**：`ai.embedding.dimension` 固定为 1024 且属于 L1，因为它与 `chunks.embedding VECTOR(1024)` 的表结构强绑定——改动需要迁移表结构并重算全部向量，重启服务本身并不足以生效。

#### 7.3.3 L2 业务配置（后台可改）

| 配置组 | 字段 | 默认值 | 取值约束 |
| :--- | :--- | :--- | :--- |
| `ai.llm` | `model` | `qwen-plus` | 非空字符串 |
| | `temperature` | 0.3 | 0.0 - 2.0 |
| | `max_tokens` | 2000 | 100 - 8000 |
| | `timeout` | 30s | 5s - 120s |
| | `max_retries` | 3 | 0 - 5 |
| `ai.embedding` | `model` | `bge-large-zh-v1.5` | 非空字符串 |
| | `batch_size` | 16 | 1 - 64 |
| | `timeout` | 20s | 5s - 60s |
| | `max_retries` | 3 | 0 - 5 |
| `ai.reranker` | `model` | `bge-reranker-v2-m3` | 非空字符串 |
| | `timeout` | 20s | 5s - 60s |
| | `max_retries` | 2 | 0 - 5 |
| `search` | `default_top_k` | 5 | 1 - 50，且 ≤ `candidate_k` |
| | `candidate_k` | 20 | 5 - 200 |
| | `rrf_k` | 60 | 1 - 200 |
| | `vector_weight` | 0.5 | 0.0 - 1.0，与 `bm25_weight` 之和为 1 |
| | `bm25_weight` | 0.5 | 0.0 - 1.0 |
| | `vector_ef_search` | 100 | 10 - 500 |
| `keyword_search` | `default_limit` | 5 | 1 - 20 |
| | `weight_tag_exact` | 10.0 | 0.0 - 100.0 |
| | `weight_category_exact` | 8.0 | 0.0 - 100.0 |
| | `weight_title` | 3.0 | 0.0 - 100.0 |
| | `weight_summary` | 1.5 | 0.0 - 100.0 |
| | `weight_content` | 1.0 | 0.0 - 100.0 |
| | `max_keywords` | 10 | 1 - 20 |
| `chunk` | `size` | 400 | 200 - 800（字） |
| | `overlap` | 50 | 0 - 200，且 < `size` |
| `collector` | `timeout` | 10s | 3s - 30s |
| | `max_redirects` | 3 | 0 - 10 |
| | `user_agent` | `AiKnow-Bot/1.0` | 非空字符串 |
| | `min_content_length` | 100 | 0 - 5000（字，低于此值判定采集失败） |
| `mcp_log` | `retention_days` | 30 | 1 - 365 |
| `prompt.analyze` | `template` | 见 7.3.5 | 非空，必须包含 `{{.Content}}`、`{{.Categories}}`，且含 "json" 字样 |

#### 7.3.5 内容分析提示词（单次调用产出三项）

摘要、分类、标签由**一次 LLM 调用**完成，通过 `response_format: {"type": "json_object"}` 约束返回结构。

**为什么合并**：三项任务都建立在对同一篇文章的理解之上。拆成三次调用意味着同一份正文被输入三次，token 成本和处理耗时都接近三倍，而收益仅是提示词更短。合并后输入 token 降至约 1/3，且三项结果出自同一次推理，一致性更好。

**默认模板**：

```
你是一个专业的知识管理助手。请分析以下文章，以 JSON 格式返回摘要、分类和标签。

返回的 JSON 必须包含以下三个字段：
- "summary"：100-300 字的中文摘要，准确概括核心内容，不添加原文没有的信息，
  不使用「本文」「这篇文章」等指代词开头
- "category"：从给定分类列表中选择最合适的一个。只能从列表中选，
  都不合适则返回「未分类」
- "tags"：3-10 个关键词标签组成的字符串数组。标签应为具体的技术名词、
  产品名或概念，避免「技术」「方法」等宽泛词，每个标签不超过 10 个字

可选分类：{{.Categories}}

示例返回格式：
{"summary": "……", "category": "技术", "tags": ["Docker", "容器网络"]}

文章标题：{{.Title}}
文章内容：
{{.Content}}
```

> **Qwen JSON 模式约束**：使用 `response_format: {"type":"json_object"}` 时，提示词中**必须出现 "json" 字样**，否则 API 会报错。模板保存时校验这一点。

**返回结果校验**：模型返回后按以下顺序校验，任一项失败则计入重试：

| 校验项 | 规则 | 失败处理 |
| :--- | :--- | :--- |
| JSON 可解析 | 能反序列化为对象 | 重试 |
| 字段完整 | `summary`、`category`、`tags` 均存在 | 重试 |
| 类型正确 | `summary` 为字符串，`tags` 为字符串数组 | 重试 |
| 摘要长度 | 50 - 500 字（比提示词要求略宽松） | 截断，不重试 |
| 分类有效 | 在候选分类列表内或等于「未分类」 | 置为「未分类」，不重试 |
| 标签数量 | 1 - 15 个 | 截取前 10 个，不重试 |
| 标签长度 | 单个不超过 20 字 | 丢弃超长标签，不重试 |

> **设计说明**：只有结构性错误才重试，内容层面的越界做兜底修正。这样避免因为模型多给了两个标签就重跑一次完整调用。重试达到 `max_retries` 后，`processing_status` 置为 `failed` 并记录 `last_error`。

**可用变量**：

| 变量 | 说明 |
| :--- | :--- |
| `{{.Title}}` | 文章标题 |
| `{{.Content}}` | 文章纯文本内容（超长时截断至前 8000 字） |
| `{{.Categories}}` | 当前分类表中的全部分类名，逗号分隔 |

#### 7.3.6 提示词测试

后台配置页提供「试跑」功能：选择一篇已有文章，用**当前编辑框中未保存的模板**调用 LLM，展示渲染后的完整 Prompt、模型原始返回和校验结果。该调用不写入文章数据，仅用于验证效果。

### 7.4 环境变量

按 [7.3.1](#731-l1-配置来源优先级) 的优先级，以下变量可通过环境变量或 `.env` 文件提供。

| 变量 | 必填 | 说明 |
| :--- | :---: | :--- |
| `DATABASE_URL` | ✅ | PostgreSQL 连接字符串 |
| `QWEN_API_KEY` | ✅ | 阿里云 Qwen API Key |
| `BAAI_API_KEY` | ✅ | 智源 API Key |
| `SESSION_SECRET` | ✅ | 后台 Session 签名密钥（32 字节以上随机串） |
| `ADMIN_PASSWORD_HASH` | ✅ | 管理员密码的 bcrypt 哈希 |
| `ADMIN_USERNAME` | ❌ | 管理员用户名，默认 `admin` |
| `ADMIN_PASSWORD` | ❌ | 管理员明文密码，仅开发环境便利用；设置后覆盖 `ADMIN_PASSWORD_HASH` |
| `SERVER_PORT` | ❌ | 服务端口，默认 8080 |
| `MCP_ENDPOINT` | ❌ | MCP 接口路径，默认 `/mcp` |
| `CONFIG_PATH` | ❌ | 配置文件路径，默认 `./config.yaml` |
| `ENV_FILE` | ❌ | `.env` 文件路径，默认 `./.env` |

**管理员凭据说明**：
- 生产环境应只提供 `ADMIN_PASSWORD_HASH`，用 `aiknow-server hashpwd` 子命令生成
- `ADMIN_PASSWORD` 是开发便利项：启动时在内存中 bcrypt 哈希后使用，明文不落盘、不写日志
- 两者都未提供时启动失败并给出明确错误，不生成随机密码——随机密码只打印一次的做法在容器重启场景下容易丢失

**生成密码哈希**：

```bash
./aiknow-server hashpwd
# 交互式输入密码，输出：$2a$10$xxxxx
```


## 八、项目结构

```
aiknow/
├── cmd/
│   └── aiknow-server/
│       └── main.go                     # 服务入口
├── internal/
│   ├── handler/
│   │   ├── api/                        # RESTful API 处理器
│   │   │   ├── article.go
│   │   │   ├── search.go
│   │   │   └── middleware.go           # API Key 鉴权、限流、审计
│   │   ├── admin/                      # 后台管理 API（Session 鉴权）
│   │   │   ├── auth.go                 # 登录/登出/Session
│   │   │   ├── dashboard.go            # 仪表盘聚合数据
│   │   │   ├── article.go              # 批量操作、Chunk 查看
│   │   │   ├── search_debug.go         # 检索分阶段调试
│   │   │   ├── mcp_debug.go            # MCP 调试代理
│   │   │   ├── setting.go              # 业务配置读写
│   │   │   ├── prompt.go               # 提示词测试
│   │   │   ├── apikey.go               # API Key 管理
│   │   │   ├── log.go                  # 审计日志查询
│   │   │   └── middleware.go           # Session 校验、CSRF、角色权限
│   │   ├── web/                        # Web UI 路由
│   │   │   └── handler.go
│   │   └── mcp/                        # MCP Streamable HTTP
│   │       └── server.go
│   ├── service/                        # 核心业务逻辑
│   │   ├── article.go
│   │   ├── collector.go                # 网页采集 + SSRF 防护
│   │   ├── llm.go                      # LLMProvider 抽象层
│   │   ├── llm_qwen.go                 # Qwen 实现
│   │   ├── embedding.go                # Embedding 抽象层
│   │   ├── embedding_baai.go           # BAAI 实现
│   │   ├── reranker.go                 # Reranker 抽象层
│   │   ├── reranker_baai.go            # BAAI 实现
│   │   ├── search.go                   # 混合检索（向量+BM25+RRF+重排）
│   │   ├── search_keyword.go           # 关键词加权检索
│   │   ├── chunk.go                    # 文档切分
│   │   ├── prompt.go                   # 提示词渲染 + JSON 结果校验
│   │   ├── setting.go                  # L2 配置读写 + 校验
│   │   ├── auth.go                     # 管理员认证、API Key 校验
│   │   ├── mcplog.go                   # MCP 请求日志异步写入 + 查询
│   │   └── worker.go                   # PGMQ 消费者
│   ├── repository/                     # 数据访问层
│   │   ├── postgres.go
│   │   ├── article_repo.go
│   │   ├── chunk_repo.go
│   │   ├── setting_repo.go
│   │   ├── apikey_repo.go
│   │   ├── mcplog_repo.go
│   │   └── search_repo.go              # pgvector + pg_textsearch 查询
│   ├── model/                          # 数据模型
│   │   ├── article.go
│   │   ├── chunk.go
│   │   ├── tag.go
│   │   ├── setting.go
│   │   ├── apikey.go
│   │   └── mcplog.go
│   └── config/                         # 配置管理
│       ├── config.go                   # L1 加载：env > .env > yaml > 默认值
│       ├── defaults.go                 # L2 配置内置默认值
│       └── cache.go                    # L2 配置内存缓存 + LISTEN/NOTIFY 热重载
├── pkg/                                # 公共库
│   ├── pgqueue/                        # PGMQ 封装
│   ├── htmlclean/                      # XSS 清洗（bluemonday）
│   └── ssrf/                           # SSRF 防护
├── web/                                # 前端静态资源（embed）
│   ├── index.html
│   ├── css/
│   │   └── output.css
│   └── js/
│       ├── app.js                      # Alpine.js 应用入口、路由
│       └── pages/
│           ├── dashboard.js
│           ├── articles.js
│           ├── article-edit.js
│           ├── search-debug.js
│           ├── mcp-debug.js
│           ├── mcp-logs.js
│           ├── settings.js
│           └── api-keys.js
├── migrations/                         # 数据库迁移
│   ├── 001_initial_schema.sql
│   ├── 002_vector.sql
│   ├── 003_pg_textsearch_bm25.sql
│   ├── 004_pgmq.sql
│   └── 005_settings_apikeys_mcplogs.sql
├── scripts/
│   └── tailwind-watch.sh
├── config.yaml.example
├── .env.example
├── .gitignore                          # 必须包含 .env
├── go.mod
├── go.sum
├── Makefile
└── README.md
```


## 九、非功能需求

### 9.1 性能指标

| 指标 | 目标 | 测量方式 | 说明 |
| :--- | :--- | :--- | :--- |
| **数据库召回 P95** | < 100ms | 内部计时（不含网络） | 含 pgvector HNSW + pg_textsearch BM25 |
| **Rerank 调用 P95** | < 500ms | 含 BAAI API 网络往返 | 外部 API，不可控 |
| **端到端 P95** | < 1s | 从收到请求到返回响应 | 含召回 + Rerank + 上下文补全 |
| **并发支持** | 100 req/s | 压测 | 单体服务 |
| **数据规模** | 100 万 Chunks | 预估 | 约 5-10 万篇文章 |

> **注**：P99 < 500ms 的硬性指标不适用于包含外部 API 的场景，因此拆分为三个层次分别测量。

### 9.2 可用性

| 指标 | 目标 |
| :--- | :--- |
| **服务可用性** | 99.5%（单实例，不含数据库） |
| **数据库可用性** | 依赖腾讯云 RDS SLA |
| **恢复时间** | < 5 分钟（自动重启） |

### 9.3 可扩展性

| 维度 | 方案 |
| :--- | :--- |
| **AI 服务切换** | 通过 `LLMProvider`、`EmbeddingProvider`、`RerankerProvider` 接口抽象 |
| **数据库垂直扩展** | 升级实例规格 |
| **服务水平扩展** | 无状态设计，支持多实例部署 + 前置负载均衡 |
| **检索索引重建** | 支持后台异步重建向量索引和 BM25 索引 |


## 十、验收标准

### 10.1 URL 采集验收

- [ ] 输入有效 URL 能正确提取标题、Markdown 正文（保留标题层级/列表/代码块）
- [ ] 渲染开启时 SPA 动态页可取到渲染后正文；渲染失败自动降级 http 不中断采集
- [ ] 微信公众号文章可正确提取正文（专用规则）
- [ ] 配置 COS 后 URL 文章处理完成时 `screenshot_url` 非空且可访问
- [ ] 采集失败时返回明确的错误信息（如网络超时、解析失败）
- [ ] 同 URL 或同内容 hash 重复提交时返回去重提示，不重复入库
- [ ] 尝试访问内网 IP（如 127.0.0.1、192.168.1.1）时被拦截

### 10.2 AI 处理验收

- [ ] 提交后立即返回，processing_status = 'pending'
- [ ] 任务完成后 processing_status = 'completed'，摘要/分类/标签已写入
- [ ] **单次 LLM 调用**即产出摘要、分类、标签三项（通过日志确认调用次数为 1）
- [ ] LLM 返回非法 JSON 时触发重试，retry_count 递增
- [ ] 返回分类不在候选列表时，自动置为「未分类」且不重试
- [ ] 返回标签超过 10 个时截取前 10 个，且不重试
- [ ] 处理失败时 processing_status = 'failed'，last_error 记录错误信息
- [ ] 支持手动重试（retry_count 递增）
- [ ] 管理员可在 Web 界面编辑摘要、分类、标签，修改后保留记录

### 10.3 检索验收

- [ ] 中文 Query 能正确分词并执行 BM25 检索（通过 zhparser）
- [ ] 语义检索返回与 Query 语义相关的结果（通过 pgvector）
- [ ] 混合检索结果优于单一检索方式
- [ ] 检索结果包含：证据块内容、标题、来源、chunk 位置、相关性分数
- [ ] 准备标准测试问题集，记录 TopK 命中率、引用准确率、P95 延迟

### 10.4 MCP 验收

- [ ] 通过 `initialize` 握手：返回协议版本和支持的 capabilities
- [ ] `tools/list` 用 readwrite Key 返回 4 个工具
- [ ] `tools/list` 用 readonly Key 只返回 3 个只读工具（不含 `add_article_by_url`）
- [ ] 通过 `tools/call` 调用 `search_knowledge` 返回 chunk 级证据块
- [ ] 通过 `tools/call` 调用 `find_articles_by_keywords` 返回文章卡片（含标题、摘要、分类、标签、链接）
- [ ] 通过 `tools/call` 调用 `get_article_detail` 返回文章全文
- [ ] 通过 `tools/call` 调用 `add_article_by_url` 成功入队并返回 article_id
- [ ] 支持 POST 和 GET 方法（GET 用于连接协商）
- [ ] 支持 `Accept: text/event-stream` 的 SSE 响应
- [ ] 返回 `MCP-Protocol-Version: 2025-06-18` 响应头
- [ ] 无有效 API Key 时返回 401 Unauthorized

### 10.5 关键词检索验收

- [ ] 单个关键词精确命中标签时，该文章排在仅正文命中的文章之前
- [ ] 精确命中分类的文章排序高于仅标题命中的文章
- [ ] 多个关键词时，命中数量多的文章排序更靠前
- [ ] `match_mode=all` 时，未命中全部关键词的文章被过滤掉
- [ ] `match_mode=any` 时，命中任一关键词的文章均可返回
- [ ] 返回结果包含标题、摘要、分类、标签、来源链接五项
- [ ] `matched` 字段正确标注每个关键词的命中方式
- [ ] 关键词大小写不影响标签/分类的精确匹配（如 `docker` 命中标签 `Docker`）
- [ ] 无匹配结果时返回友好提示，`isError` 为 `false`
- [ ] 超过 `max_keywords` 个关键词时返回参数校验错误
- [ ] 已软删除的文章不出现在结果中
- [ ] 后台调整权重后，排序结果相应变化

### 10.6 MCP 写入工具验收

- [ ] readonly Key 调用 `add_article_by_url` 返回 403
- [ ] readwrite Key 调用成功，文章以 `pending` 状态入库并入队
- [ ] 提交已存在的 URL 时返回去重提示，`isError` 为 `false`，不重复入库
- [ ] 提交内网 IP（如 `http://127.0.0.1`）被 SSRF 拦截
- [ ] SSRF 拦截的错误信息不回显 DNS 解析结果或具体内网地址
- [ ] 提交非 HTTP(S) 协议（如 `file://`）被拒绝
- [ ] 提交正文过短的页面时返回「未能提取到有效正文」
- [ ] 写操作限流与 REST API 共享同一计数器，超限返回 429
- [ ] 可选参数 `category_name`、`tags` 生效，标签与 AI 生成的合并去重

### 10.7 安全验收

- [ ] URL 采集：SSRF 黑白名单生效，内网 IP 被拦截
- [ ] HTML 展示：XSS 脚本被清洗，CSP 头生效
- [ ] API Key：只读 Key 无法执行写入操作（含 REST 与 MCP 两个入口）
- [ ] 速率限制：超过限制返回 429 Too Many Requests
- [ ] MCP 调用日志：每次 MCP 请求均有完整记录
- [ ] 删除文章后 `deleted_at`、`deleted_by` 被正确写入
- [ ] API Key 数据库中仅存哈希，列表页无法查看明文
- [ ] 未登录访问 `/api/v1/admin/*` 返回 401
- [ ] 管理员明文密码不出现在任何日志或数据库记录中

### 10.8 后台页面验收

- [ ] 登录后可正常进入后台，连续 5 次密码错误锁定 15 分钟
- [ ] 仪表盘正确展示文章数、Chunk 数、处理状态分布、队列深度
- [ ] 文章列表支持分类/标签/状态/时间范围筛选，筛选条件可组合
- [ ] 文章列表支持批量重新处理和批量删除，操作前有二次确认
- [ ] 已软删除的文章不出现在列表中
- [ ] `failed` 状态文章展示 `last_error`，点击重试后 `retry_count` 递增
- [ ] 文章编辑页可修改摘要/分类/标签并保存，不触发重新处理
- [ ] 修改正文并保存后 `processing_status` 重置为 `pending` 并重新入队
- [ ] 文章编辑页可查看所有 Chunk 的内容与向量生成状态
- [ ] 检索测试页展示向量召回、BM25 召回、RRF 融合、精排四个阶段的结果
- [ ] 检索测试页展示各阶段耗时和 Query 的 zhparser 分词结果
- [ ] 检索测试页支持关键词检索模式，展示每篇文章的得分构成

### 10.9 MCP 调试页面验收

- [ ] 选择工具后按 `inputSchema` 自动渲染参数表单
- [ ] 数组类型参数（如 `keywords`）能正确渲染为可增删的多值输入
- [ ] 表单模式与 JSON 模式可切换，切换时参数值保持同步
- [ ] 展示将要发送的完整 JSON-RPC 请求体，可复制
- [ ] 展示 `tools/call` 的完整原始响应（含 `content`、`annotations`、`isError`）
- [ ] `content` 中的 resource 类型渲染为卡片，点击可跳转对应文章
- [ ] `initialize`、`tools/list`、`tools/call` 三个操作均可独立发起
- [ ] 勾选 SSE 模式后，事件流逐条追加显示
- [ ] 展示 `MCP-Protocol-Version`、`Mcp-Session-Id` 响应头
- [ ] 切换为只读 API Key 调用写入类工具时，正确展示 403 响应
- [ ] 展示端到端耗时与响应中的 `query_time_ms`

### 10.10 MCP 请求日志验收

- [ ] 每次 MCP 调用后日志表新增一条记录，含完整请求体与响应体
- [ ] 客户端传入 `X-Trace-Id` 时，日志记录该值；未传时服务端生成 UUID
- [ ] 响应头回写 `X-Trace-Id`
- [ ] 可按 trace_id 精确查询到单次调用
- [ ] 可按时间范围、方法、工具名、调用方、错误状态筛选
- [ ] 可按请求参数内容全文检索（如搜「Docker」命中相关调用）
- [ ] 详情页展示完整请求/响应/响应头
- [ ] `search_knowledge` 调用的日志中，`result_summary` 记录命中 chunk 与分数
- [ ] 点击「重放」跳转调试页并正确回填参数
- [ ] 日志写入失败不影响 MCP 请求本身的返回
- [ ] 超过保留天数的日志被 pg_cron 自动清理

### 10.11 系统配置验收

- [ ] 后台修改 LLM model / temperature 后保存，无需重启即生效
- [ ] 后台修改检索参数（`top_k`、`rrf_k`、权重）后，检索测试页立即使用新值
- [ ] 后台修改关键词检索权重后，`find_articles_by_keywords` 排序相应变化
- [ ] 后台编辑内容分析提示词并保存，下一次 AI 处理使用新模板
- [ ] 提示词试跑功能可用未保存的模板调用 LLM，展示渲染后 Prompt 和模型返回
- [ ] 保存非法值被拒绝：`temperature` 超出 0-2、权重之和不为 1、`top_k` > `candidate_k`
- [ ] 提示词缺少必需变量（如 `{{.Content}}`）或不含 "json" 字样时保存被拒绝
- [ ] 「恢复默认」后配置回到内置默认值
- [ ] 敏感信息（API Key、数据库密码、管理员密码）不出现在配置页面中
- [ ] 多实例部署时，一个实例修改配置后其他实例同步生效

### 10.12 配置加载验收

- [ ] 同时存在环境变量和 `.env` 中的同名项时，环境变量生效
- [ ] 同时存在 `.env` 和 `config.yaml` 中的同名项时，`.env` 生效
- [ ] 三者都不存在时使用内置默认值
- [ ] `.env` 文件不存在时正常启动，不报错
- [ ] 启动日志打印各关键配置的来源，但不打印具体值
- [ ] 缺少必填项（如 `DATABASE_URL`）时启动失败并给出明确错误
- [ ] `ADMIN_PASSWORD_HASH` 和 `ADMIN_PASSWORD` 都未设置时启动失败


## 十一、开发路线图（7 周 MVP）

| 阶段 | 周期 | 任务 | 交付物 |
| :--- | :--- | :--- | :--- |
| **Phase 1：基础框架** | Week 1 | 项目初始化，PostgreSQL 17 环境搭建，核心数据表迁移，配置分层加载（env > .env > yaml，L2 缓存） | 可编译的骨架，Schema 就绪，配置可读写 |
| **Phase 2：采集 + AI 处理** | Week 2 | URL 采集（防 SSRF），Qwen 单次调用产出三项 + JSON 校验，PGMQ 异步队列 | 完整录入流程（手动+URL），状态查询 |
| **Phase 3：检索核心** | Week 3 | pgvector HNSW 索引，pg_textsearch BM25 + zhparser，RRF 融合，关键词加权检索 | 混合检索 + 关键词检索接口可调用 |
| **Phase 4：精排 + 后台基础页** | Week 4 | BAAI Reranker 集成，登录/仪表盘/文章列表/文章编辑 | 可管理文章的 Web 后台 |
| **Phase 5：MCP + API** | Week 5 | MCP Streamable HTTP，4 个工具实现（含写入工具 scope 校验），RESTful API 完善，鉴权/限流/MCP 日志 | Claude Desktop 可正常挂载并读写 |
| **Phase 6：调试与配置页** | Week 6 | 检索测试页、MCP 调试页、MCP 请求日志页、系统配置页、API Key 管理 | 完整的后台管理与调试能力 |
| **Phase 7：测试与发布** | Week 7 | 性能测试、安全测试、文档编写、Bug 修复 | v1.0.0 可交付版本 |

**总计**：7 周（MVP 范围，不含 P1/P2 功能）

> **变更说明**：相比 v3.1 的 6 周计划增加 1 周。原 Phase 4 将「Reranker + 完整 Web 后台」压在同一周，而细化后后台包含 9 个页面（含 MCP 调试、MCP 日志、检索调试、系统配置四个交互较重的页面），拆为 Phase 4（基础页）和 Phase 6（调试与配置页）两个阶段更贴合实际工作量。

> **v3.4 工期提示**：新增的关键词检索（Phase 3）和 MCP 写入工具（Phase 5）预计各增加 1.5-2 天工作量，两个阶段本就相对宽松，暂不调整总工期。若 Phase 3 或 Phase 5 出现延期，优先将 `match_mode=all`、`matched` 字段这两项降级为 P1 处理。


## 十二、风险与应对

| 风险 | 影响 | 应对策略 |
| :--- | :--- | :--- |
| **BAAI API 限流或变更** | 向量生成/重排服务中断 | 通过 Provider 抽象层预留 OpenAI/本地模型切换方案 |
| **pg_textsearch 与 PG 版本兼容性** | 若腾讯云暂未支持 PG 17 | 提前确认实例版本，或备选 ParadeDB `pg_bm25` 扩展 |
| **pgvector HNSW 内存占用过高** | 百万级 Chunks 时内存不足 | 调优 `m` 和 `ef_construction` 参数，或改用 IVFFlat |
| **中文分词效果不佳** | 检索准确率下降 | 评估 `zhparser` 配置调优，或备选 `pg_jieba` |
| **AI 处理延迟导致 PGMQ 积压** | 摘要处理跟不上录入速度 | 监控队列深度，支持水平扩展 Worker 数量 |
| **MCP 协议版本更新** | 兼容性问题 | 关注 MCP 规范演进，及时升级 `mcp-go` SDK |
| **前端功能复杂度增长** | Alpine.js 难以维护 | 保持前后端分离和组件化设计，预留向 Vue 迁移空间 |


## 十三、附录

### 附录 A：参考资料

- [pgvector 官方文档](https://github.com/pgvector/pgvector)
- [pg_textsearch 官方文档](https://github.com/timescale/pg_textsearch)
- [zhparser 官方文档](https://github.com/amutu/zhparser)
- [PGMQ 官方文档](https://github.com/tembo-io/pgmq)
- [MCP 2025-06-18 规范](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)
- [mcp-go SDK](https://github.com/mark3labs/mcp-go)
- [Alpine.js 官方文档](https://alpinejs.dev)
- [Tailwind CSS 官方文档](https://tailwindcss.com)
- [阿里云 Qwen API 文档](https://help.aliyun.com/zh/dashscope/)
- [BAAI 智源 API 文档](https://huggingface.co/BAAI)

### 附录 B：术语表

| 术语 | 说明 |
| :--- | :--- |
| **MCP** | Model Context Protocol，Anthropic 提出的 AI 模型上下文协议 |
| **Streamable HTTP** | MCP 2025-06-18 规范推荐的传输方式，支持 POST/GET/SSE |
| **RRF** | 倒数排名融合，混合检索结果合并算法 |
| **RAG** | 检索增强生成，将检索结果作为上下文供给大模型 |
| **PGMQ** | PostgreSQL 原生轻量级消息队列 |
| **Chunk** | 文档拆分后的语义块，用于精细检索 |
| **Embedding** | 文本向量嵌入，将文本转换为高维向量（本方案 1024 维） |
| **Reranker** | 重排模型，对检索结果进行二次精排 |
| **SSRF** | 服务端请求伪造，一种利用服务端发起恶意请求的安全漏洞 |
| **XSS** | 跨站脚本攻击，通过注入恶意脚本攻击用户 |
| **HNSW** | 分层可导航小世界图，pgvector 支持的近似最近邻索引算法 |

### 附录 C：关键配置参数

| 参数 | 值 | 说明 |
| :--- | :--- | :--- |
| `pgvector HNSW m` | 16 | 每层连接数，调高提升召回但增加内存 |
| `pgvector HNSW ef_construction` | 64 | 构建时动态列表大小，调高提升索引质量 |
| `pgvector HNSW ef_search` | 100 | 查询时动态列表大小，调高提升召回但增加延迟 |
| `pg_textsearch k1` | 1.2 | BM25 词频饱和参数 |
| `pg_textsearch b` | 0.75 | BM25 文档长度归一化参数 |
| `Chunk 大小` | 200-500 字 | 文档拆分粒度 |
| `RRF k` | 60 | 倒数排名融合参数（标准值） |
| `RRF 候选数` | 20 | 各分支召回后 RRF 融合的候选数 |
| `向量权重` | 0.5 | 向量检索在混合检索中的权重 |
| `BM25 权重` | 0.5 | BM25 检索在混合检索中的权重 |
| `最终返回数` | 5 | Reranker 精排后最终返回数量 |
| `向量维度` | 1024 | 使用 bge-large-zh-v1.5 模型 |
| `中文分词词性` | n,v,a,i,e,l,j,r,d,m,q,t,z | 名词、动词、形容词、成语、叹词、习用语、简称、代词、副词、数词、量词、时间词、状态词 |


## 十四、变更记录

| 版本 | 日期 | 作者 | 变更说明 |
| :--- | :--- | :--- | :--- |
| v3.4.1 | 2026-09-07 | - | 移除关键词检索的「标签部分匹配」档：标签是原子词只能精确命中，substring 匹配会产生假阳性（`Java` 命中 `JavaScript`）且与全文检索信号重复；权重从 6 档减为 5 档，删除 `pg_trgm` 扩展依赖 |
| v3.4 | 2026-09-07 | - | MCP 工具扩展版：MCP 工具从 2 个增至 4 个，新增 `find_articles_by_keywords`（关键词检索文章，标签/分类加权）和 `add_article_by_url`（写入工具，需 readwrite scope）；`tools/list` 按 scope 过滤；新增 REST `/api/v1/search/keywords`；新增 `keyword_search` 权重配置组与相关索引；检索测试页增加关键词检索 Tab；新增 2 组验收标准 |
| v3.3 | 2026-09-07 | - | 需求优化版：AI 处理合并为单次 LLM 调用（JSON 输出 + 结果校验），三段提示词合并为 `prompt.analyze`；移除 `admins` 表与角色体系，管理员账号改由环境变量配置；`audit_logs` 表替换为 `mcp_request_logs`（含 trace_id、全文检索、一键重放），删除追溯改由 `articles.deleted_by` 承担；配置加载优先级明确为 环境变量 > `.env` > `config.yaml` > 内置默认值 |
| v3.2 | 2026-09-07 | - | 功能细化版：细化 Web 后台 10 个页面功能；新增 MCP 调试页与检索测试页；配置拆分为 L1 启动配置 / L2 业务配置 / L3 凭据数据三层，业务参数与提示词移至后台管理；新增 settings、setting_history、api_keys、admins、audit_logs 五张表；新增后台管理 API；补充 4 组验收标准；路线图调整为 7 周 |
| v3.1 | 2026-09-07 | - | 技术调整版：向量维度调整为 1024（bge-large-zh-v1.5）；补充 RRF 参数配置；完善中文分词词性映射；优化 MCP 返回格式为结构化 resource；明确 API 分类/标签创建逻辑 |
| v3.0 | 2026-09-07 | - | 评审修订版：统一命名为 AiKnow；修正数据模型（category_id + article_tags）；明确 pg_textsearch + zhparser；缩减 v1 范围；补充 MCP 规范细节；补充安全需求；调整性能指标 |

