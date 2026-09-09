# AiKnow 项目重要信息

## 项目概述

- **项目名称**：爱脑 (AiKnow)
- **定位**：企业内部知识库管理 + RAG 检索 + API/MCP 接入
- **当前版本**：v0.6.0（W5-W6 鉴权+后台已完成）
- **状态**：PRD 已评审完成，W1 基础框架骨架已落地

## 核心技术栈

### 后端
- **语言**：Go 1.23+
- **框架**：Gin 1.10+
- **数据库**：ParadeDB 0.25.6-pg17（PostgreSQL 17 发行版）
- **扩展**：
  - `pgvector` - 向量存储与检索（ParadeDB 镜像内置）
  - `pg_search` 0.25.6 - BM25 全文检索 + jieba 中文分词（底层 Tantivy）
  - 任务队列不依赖扩展：数据表 + `FOR UPDATE SKIP LOCKED`

### 前端
- **框架**：Alpine.js 3.14+ (轻量级，约 15KB)
- **CSS**：Tailwind CSS 3.4+
- **架构**：前后端分离

### AI 服务
- **LLM**：阿里云 Qwen 系列（单次调用产出摘要+分类+标签）
- **Embedding**：BAAI bge-large-zh-v1.5 (**1024 维**)
- **Reranker**：BAAI bge-reranker-v2-m3

## 关键技术决策

### 1. 向量维度：1024 维
- **模型**：`bge-large-zh-v1.5`
- **理由**：相比 768 维的 base 模型，large 模型检索准确率更高
- **影响**：
  - 数据库字段：`embedding VECTOR(1024)`
  - 内存占用略增加，但在可接受范围内

### 2. 混合检索架构
```
向量检索 (pgvector HNSW) → Top 20
         ↓
BM25 检索 (pg_search + jieba) → Top 20
         ↓
RRF 融合 (k=60, 权重 0.5:0.5) → Top 20
         ↓
Reranker 精排 (BAAI) → Top 5
         ↓
补全父文档上下文 → 返回结果
```

### 3. 中文分词配置
分词由 pg_search 内置的 jieba 分词器完成，在建索引时以类型转换声明，无需额外配置词性映射：

```sql
CREATE INDEX chunks_search_idx ON chunks
USING paradedb (id, (content::pdb.jieba), deleted, article_id, chunk_index)
WITH (key_field = 'id');
```

jieba 采用词典 + 统计模型做词级切分，相较逐字切分（`chinese_compatible`）precision 更高。
验证分词效果：

```sql
SELECT '你好世界，全文检索'::pdb.jieba::text[];
```

> 原 zhparser 方案的词性映射（`ADD MAPPING FOR n,v,a,...`）已废弃。

### 4. MCP 返回格式
采用结构化 `resource` 格式，便于大模型解析：
```json
{
  "content": [
    {"type": "text", "text": "找到 N 个相关文档片段"},
    {
      "type": "resource",
      "resource": {
        "uri": "aiknow://article/{uuid}/chunk/{index}",
        "name": "标题 - 第X节",
        "description": "内容片段",
        "mimeType": "text/plain"
      }
    }
  ]
}
```

### 5. AI 处理：单次 LLM 调用产出三项（v3.3）

摘要、分类、标签**一次调用**完成，`response_format: {"type":"json_object"}` 约束结构。

**为什么合并**：三项都建立在对同一篇文章的理解上，拆三次意味着同一份正文输入三次，成本和耗时接近三倍。合并后输入 token 降至约 1/3。

**Qwen 坑点**：用 `json_object` 时提示词中**必须含 "json" 字样**，否则 API 报错。模板保存时校验。

**校验分级**（只有结构性错误才重试）：
| 校验项 | 失败处理 |
|--------|---------|
| JSON 可解析 / 字段完整 / 类型正确 | 重试 |
| 摘要长度超 50-500 字 | 截断 |
| 分类不在候选列表 | 置「未分类」 |
| 标签超 15 个 / 单个超 20 字 | 截取前 10 / 丢弃超长项 |

### 6. 配置分层（v3.2，v3.3 调整，v0.6.2 收紧）

| 层级 | 存储 | 内容 | 变更方式 |
|------|------|------|---------|
| **L1 启动配置** | 环境变量 → `.env` → config.yaml | 连接串、密钥、端口、运行环境、provider、base_url、LLM 模型名、采集超时/重定向/最小内容长度、管理员账号、Session 有效期 | 需重启 |
| **L2 业务配置** | `settings` 表 | 真正支持运行时修改的业务参数（LLM 参数、检索/分块/关键词权重、提示词、User-Agent 覆盖） | 后台改，热生效 |
| **L3 凭据数据** | `api_keys` 表 | API Key 哈希 | 后台管理 |

**分层判断原则**：
- **隐私与密钥**（API Key、密码）绝不进 `settings` 表和后台页面
- **启动期即固定的参数归 L1**：如采集超时/重定向次数——`http.Client` 在 `serve()`
  启动时创建，运行时改 settings 表并不会重建连接，放 L2 只会制造"改了没生效"的假象
- `provider` / `base_url` / LLM 模型名同时允许 L2 覆盖：起作用的运行时来源优先
- **向量维度不是配置项**：与 `VECTOR(1024)` 表结构、HNSW 索引、bge-large-zh-v1.5
  三者强绑定，改了要重建表与索引并重灌数据。当前硬编码在
  `internal/service/provider/`，刻意不开放配置，避免给出"可调"的错误暗示

**L1 加载优先级**（先命中者生效）：
```
1. 环境变量        2. .env 文件
3. config.yaml     4. 代码内置默认值
```
`.env` 不存在时静默跳过。启动后打印各配置的**来源**但不打印值（`cfg.LogSources()`）。

优先级不依赖 `godotenv` 的覆盖行为：`NewConfig()` 会在加载 `.env` **之前**先快照真实
环境变量，再用 `godotenv.Read()` 单独取文件内容做来源归因。原因是 `godotenv.Load()`
会把文件内容注入进程环境，之后 `os.Getenv` 便无法区分某个值来自 `.env` 还是环境变量，
只按 `Getenv` 判断会把 `.env` 的值误标为环境变量来源。

**L2 热生效**：
```
启动：settings 表 → 内存 ConfigCache（serve() 创建并 Reload）
运行：后台保存 → 写 settings 表 → SettingHandler.onChanged 回调 → ConfigCache.Reload
```
`settings` 表缺键回落内置默认值，首次部署无需灌数据；「恢复默认」= 删除该行
（随后同样触发 Reload）。

**L2 值格式为扁平标量**：数字（`0.3`）或 JSON 编码字符串（`"qwen-plus"`）。此前
ConfigCache.Reload 用 `map[string]interface{}` 解析，对扁平标量必然失败，L2 配置
从未真正生效；v0.6.2 起按各 key 已知类型直接解码（见
`internal/config/cache.go` 的 applySetting）。

**运行时来源优先于启动配置**：各消费端通过 `service.L2ConfigSource` 接口（
`config.ConfigCache` 实现）每次调用读取最新 L2 值。接入清单见
`docs/CONFIG_ARCHITECTURE.md#消费端接入`：
- 混合检索 / 关键词检索 / 文章处理 / 日志清理均读 L2 对应分组
- `llm.model`：QwenProvider 注入 L2 ConfigCache（`SetModelSource`）；Generate 时
  模型名优先级 = 调用方显式指定 > L2 `llm.model` > L1 `QWEN_MODEL` > 内置 qwen-plus
- `collector.user_agent`：Collector 每次抓取读 settings 表；非空则覆盖 L1
  `COLLECTOR_USER_AGENT`

### 7. API Key 存储：SHA-256 而非 bcrypt
- API Key 是高熵随机串，无字典攻击风险
- 每次请求都要验证，bcrypt 计算开销不可接受
- 存 `key_hash`（SHA-256）+ `key_prefix`（列表展示用），**不存明文**，创建时仅展示一次

### 8. MCP 调试走后端代理
调试页面通过 `/api/v1/admin/mcp/invoke` 转发，而非浏览器直连 `/mcp`：
- 避免前端暴露 API Key
- 绕开浏览器对 MCP 端点 `Origin` 校验的限制

### 8.1 MCP 工具集（v3.4，共 4 个）

| Tool | 功能 | scope |
|------|------|-------|
| `search_knowledge` | 语义检索 → **chunk 证据块** | readonly |
| `find_articles_by_keywords` | 关键词检索 → **文章卡片** | readonly |
| `get_article_detail` | 文章全文 | readonly |
| `add_article_by_url` | URL 采集入库 | **readwrite** |

**两个检索工具不是重复功能**，返回粒度不同：
- `search_knowledge`：自然语言问句 → chunk 片段，用于 RAG 证据供给
- `find_articles_by_keywords`：关键词数组 → 整篇文章卡片，用于文章发现

工具 `description` 中写明该区别，引导大模型正确选择。

**关键词检索评分**（标签/分类权重高于正文——用户给关键词时期望「打了这个标签的文章」，而非「正文恰好提到的文章」）：

| 命中位置 | 权重 |
|---------|------|
| 标签精确 | 10.0 |
| 分类精确 | 8.0 |
| 标题 | 3.0 |
| 摘要 | 1.5 |
| 正文 | 1.0（BM25 归一化后乘权重） |

**标签/分类只做精确匹配，不做 substring**：标签是原子词，`LIKE '%Java%'` 会命中 `JavaScript`，`云` 会命中 `云原生`/`阿里云`/`云计算`。且标签词通常也出现在标题正文中，已被后三档 jieba 全文检索覆盖——部分匹配不提供新信息，只给假阳性开高权重通道。

- 得分 = Σ(每个关键词的各项命中权重)，**不做平均**（命中数量本身是信号）
- `match_mode`：`any`（默认）/ `all`
- 权重在 L2 配置 `keyword_search` 中，后台可调
- 返回的 `matched` 字段说明每个关键词的命中方式，让大模型判断相关性强度

**写入工具的三个关键决策**：
1. **去重命中返回 `isError: false`** —— 若返回 true，大模型会误以为失败而重试
2. **SSRF 拦截错误信息统一措辞**，不回显 DNS 解析结果 —— 否则该工具变成内网探测器
3. **写限流与 REST API 共享计数器**（按 API Key），避免绕过限流

**`tools/list` 按 scope 过滤**：readonly Key 看不到 `add_article_by_url`。让只读客户端根本看不到写工具，比看到却调用失败更合理。调用层仍强制校验 scope 返回 403。

### 9. 日志策略：只记 MCP 调用，不做通用审计（v3.3）

**取舍**：MVP 单管理员场景下「谁操作的」信息量有限，真正需回溯的是 MCP 调用（排查检索效果）。因此不建通用 `audit_logs`，改建 `mcp_request_logs`。

**代价与补偿**：换掉审计日志后「谁删了文章」失去记录 → 在 `articles` 表补 `deleted_at` / `deleted_by`（本来就是软删除，成本极低）。

**trace_id**：优先取请求头 `X-Trace-Id`（不强制），未传则服务端生成 UUID，响应头回写。

**写入**：响应返回后异步写，不阻塞 MCP 主链路；写入失败仅记服务端日志。

**保留**：默认 30 天，`pg_cron` 清理，天数由 `mcp_log.retention_days` 配置。

### 10. 管理员账号：配置文件而非数据库（v3.3）

v1 单管理员，凭据来自 `ADMIN_USERNAME` + `ADMIN_PASSWORD_HASH`，无用户表、无角色区分、无密码修改页面。改密码 = 改配置后重启。

- 生产只提供 `ADMIN_PASSWORD_HASH`，用 `aiknow-server hashpwd` 生成
- `ADMIN_PASSWORD`（明文）仅开发便利，启动时内存哈希，不落盘不写日志
- 两者都缺 → **启动失败报错**，不生成随机密码（容器重启会丢）
- 登录失败锁定状态存内存
- 多用户/角色/自助改密留待 v2

## Web 管理后台（9 个页面）

| 页面 | 路径 | 核心内容 |
|------|------|---------|
| 登录页 | `/ui/login` | 密码登录，5 次失败锁定 15 分钟 |
| 仪表盘 | `/ui/dashboard` | 统计卡片、状态分布、队列深度、最近活动、服务健康 |
| 文章列表 | `/ui/articles` | 筛选、搜索、排序、行内/批量操作 |
| 文章编辑 | `/ui/articles/:id` | 正文编辑、AI 结果修正、状态面板、Chunk 预览 |
| 文章录入 | `/ui/articles/new` | 手动录入 / URL 采集（含预览） |
| 检索测试 | `/ui/search` | 分阶段结果、耗时明细、分词结果 |
| MCP 调试 | `/ui/mcp-debug` | 参数表单、原始请求响应、SSE、鉴权切换 |
| MCP 请求日志 | `/ui/mcp-logs` | 筛选、全文检索、trace_id 查询、一键重放 |
| 系统配置 | `/ui/settings` | 模型参数、提示词、检索/采集参数 |
| API Key 管理 | `/ui/settings/api-keys` | 创建/禁用/删除 |

**关键行为约定**：
- 改正文保存 → `processing_status` 重置 `pending` 并重新入队
- 仅改 AI 结果（摘要/分类/标签）→ **不**触发重新处理

## 鉴权体系（两套并存）

| 入口 | 鉴权方式 | 路径前缀 |
|------|---------|---------|
| 对外 API | API Key（`Authorization: Bearer`） | `/api/v1/*` |
| MCP | API Key | `/mcp` |
| 后台管理 | Session Cookie + CSRF Token | `/api/v1/admin/*` |

v1 单管理员，无角色区分。

> ✅ **v0.6.0 已全部接线，上表即为当前实现**。具体变更见 `docs/CHANGELOG.md` v0.6.0。
>
> - 对外 API 与 MCP 走 API Key + scope（`readonly` 拒绝写入，`readwrite` 隐含只读）
> - 后台管理走 Session Cookie + CSRF Token（同步器令牌模式，Token 随登录下发，
>   GET 放行，写请求须 `X-CSRF-Token` 请求头）
> - 登录失败锁定已实现（5 次 → 15 分钟）
> - API Key 按 SHA-256 哈希直接查库（`idx_api_keys_key_hash` 索引命中），
>   可通过后台页面或 CLI（`aiknow-server apikey create|list|revoke`）管理
> - `sameSite=Lax` + `HttpOnly` 双重防护

## 数据表清单

**核心业务表**：`articles`、`chunks`、`categories`、`tags`、`article_tags`

**管理与配置表**：
| 表名 | 用途 |
|------|------|
| `settings` | L2 业务配置，key-value + JSONB |
| `setting_history` | 配置变更记录 |
| `api_keys` | API Key（哈希存储） |
| `mcp_request_logs` | MCP 调用日志（含 trace_id、完整请求响应） |

## 提示词模板

单一提示词 `prompt.analyze` 存 `settings` 表，Go `text/template` 语法，返回 JSON 含 `summary` / `category` / `tags`。

**可用变量**：
- `{{.Title}}` — 标题
- `{{.Content}}` — 纯文本，超长截断至前 8000 字
- `{{.Categories}}` — 全部分类名，逗号分隔

**校验**：保存时校验模板可编译、含 `{{.Content}}` 和 `{{.Categories}}`、且含 "json" 字样（Qwen JSON 模式要求）。

**试跑**：可用未保存的模板试跑指定文章，展示渲染后 Prompt、模型原始返回和校验结果，不写入数据。

## API 设计规范

### 文章录入
```json
POST /api/v1/articles
{
  "title": "标题",
  "content": "内容",
  "category_id": "uuid",        // 方式1：直接指定 ID
  "category_name": "分类名",     // 方式2：名称（不存在则创建）
  "tags": ["标签1", "标签2"]     // 不存在则自动创建
}
```
**规则**：
- `category_id` 和 `category_name` 二选一，优先 `category_id`
- 分类/标签不存在时自动创建

### 检索接口
```json
POST /api/v1/search
{
  "query": "检索问题",
  "top_k": 5,           // 最终返回数
  "candidate_k": 20,    // RRF 召回数
  "rrf_k": 60,          // RRF 参数
  "filters": {...}
}
```

## 核心配置参数

> 标注「L2」的参数在后台可改且热生效；标注「L1 + L2 覆盖」的，L1 设默认、L2 可覆盖；
> 其余 L1 为启动配置（需重启）。

| 参数 | 值 | 层级 | 说明 |
|------|-----|------|------|
| LLM 模型名 | qwen-plus | L1 + L2 覆盖 | `QWEN_MODEL` 设基线；后台 `llm.model` 留空回落基线 |
| LLM temperature | 0.3 | L2 | 0.0-2.0 |
| LLM max_tokens | 2000 | L2 | 100-8000 |
| LLM retry_count / timeout_ms | 3 / 30000 | L2 | 0-5 / 5000-120000 |
| Embedding batch_size | 10 | L2 | 1-64 |
| 采集 User-Agent | AiKnow/1.0 (Internal Knowledge Base) | L1 + L2 覆盖 | `COLLECTOR_USER_AGENT` 设基线；后台 `collector.user_agent` 留空回落基线 |
| 采集超时 | 10s | L1 | `COLLECTOR_TIMEOUT`，重启生效 |
| 采集重定向次数 | 3 | L1 | `COLLECTOR_MAX_REDIRECTS` |
| 采集最小内容长度 | 200 | L1 | `COLLECTOR_MIN_CONTENT_LENGTH`（字符） |
| 向量维度 | 1024 | 固定 | bge-large-zh-v1.5，与表结构绑定，不可配置 |
| HNSW m | 16 | 建表参数 | 每层连接数 |
| HNSW ef_construction | 64 | 建表参数 | 构建时列表大小 |
| HNSW ef_search | 100 | L2 | 查询时列表大小 |
| BM25 k1 | 1.2 | 建表参数 | 词频饱和参数 |
| BM25 b | 0.75 | 建表参数 | 文档长度归一化 |
| RRF k | 60 | L2 | 倒数排名融合参数 |
| 向量权重 / BM25 权重 | 0.5 / 0.5 | L2 | 之和须为 1 |
| default_top_k | 5 | L2 | 须 ≤ candidate_k |
| candidate_k | 20 | L2 | RRF 召回候选数 |
| Chunk size / overlap | 400 / 50 | L2 | 200-800 / 0-200，overlap < size |
| Reranker top_k | 20 | L2 | 1-50 |
| mcp_log retention_days | 30 | L2 | 1-365，pg_cron 清理 |
| keyword_search 权重组 | 见 8.1 节 | L2 | 5 项权重，0-100 |
| prompt.analyze | 内置模板 | L2 | 文章分析提示词，后台可改 |

## v1 范围（MVP）

### ✅ 包含功能
- 文章增删改查 + 手动/URL 采集
- AI 异步处理（单次调用产出摘要/分类/标签）
- 混合检索 + Reranker 精排
- 关键词检索文章（标签/分类加权）
- RESTful API + API Key 鉴权
- MCP 四个工具（3 只读 + 1 写入，按 scope 过滤）
- MCP 请求日志（检索 + 回放）
- Web 管理后台（9 个页面，含 MCP 调试与检索测试）
- 业务参数与提示词后台可配置（热生效）
- 基础安全措施（防 SSRF、防 XSS）

### ❌ 不包含功能
- 批量导入/导出
- 标签/分类管理界面
- 版本历史与回滚
- 回收站
- MCP 更新/删除类工具（`update_article`、`delete_article`）
- 搜索建议/高级筛选
- 多管理员 / 角色权限 / 密码自助修改（v2）
- 通用操作审计日志（v2）

## 开发注意事项

### 1. 数据库迁移顺序
```
001_initial_schema.sql          # 基础表结构
002_vector.sql                  # pgvector HNSW 索引（余弦距离）
003_pg_textsearch_bm25.sql      # 已废弃（原 zhparser 方案，保留为空操作）
004_pgmq.sql                    # 已废弃（原 PGMQ 方案，保留为空操作）
005_settings_apikeys_mcplogs.sql # settings/setting_history/api_keys/mcp_request_logs
006_paradedb_search.sql         # pg_search BM25 索引（jieba）+ chunks.deleted 冗余列
007_job_queue.sql               # 任务队列表 job_queue / job_archive
008_articles_retry_columns.sql  # articles 补 retry_count / last_error 列
009_operator_columns_to_text.sql # api_keys.created_by / setting_history.changed_by / articles.deleted_by UUID → TEXT
```

> 003/004 降级为空操作而非原地改写：已有开发库中它们已记录为"已应用"，
> 原地改写不会重跑，新逻辑必须放在新版本号里。

**表结构的唯一来源就是这个目录**，不要在别处再放一份建表 DDL。开发期初始化用
`make migrate-up`（等价于 `aiknow-server migrate`），与 `serve` 启动时执行的是同一套
迁移代码。曾经在 `deploy/postgres/schema.sql` 放过一份"合并版 DDL"用于开发期建表，
结果它与迁移迅速漂移（列名、缺列、默认配置项都对不上，且无人察觉），已删除。

> 扩展（`vector`、`pg_search`）是唯一的例外，须由根目录的
> `docker-compose-initdb.sql` 创建——迁移执行器会跳过一切扩展创建语句。
> 该文件虽以 `docker-compose` 开头，但**本地开发同样需要执行**（`make db-init`），
> 它是扩展创建的唯一来源，详见 `docs/DEPLOY.md`。

关键词检索还需标签/分类的 `lower(name)` 索引（精确匹配）+ `article_tags.tag_id` 索引。
标题/摘要/正文的全文检索由 `articles_search_idx`（paradedb 索引）统一承担，不再使用 GIN 索引。

### 2. ParadeDB / pg_search 使用约束

语法在 0.25.0 发生过不兼容变更，编写 SQL 前务必确认版本（本项目基于 **0.25.6**）：

| 项目 | 写法 |
|---|---|
| 访问方法 | `USING paradedb`（`bm25` 为兼容别名） |
| 分词器 | 类型转换 `(content::pdb.jieba)`，**非** `WITH (text_fields='{...}')` |
| 打分 | `pdb.score(<key_field>)`，命名空间是 `pdb` |
| 匹配算子 | `\|\|\|` 任一词元、`&&&` 全部词元 |

其他硬约束：

- **每张表只允许一个 paradedb 索引**——新增可检索字段须改动同一个索引定义
- **过滤字段必须一并建进索引**，否则条件无法下推，topK 与分页会失真
  （这正是 `chunks.deleted` 冗余列存在的原因，由触发器与 `articles.deleted_at` 同步）
- 打分受未回收死行影响，排序异常时执行 `VACUUM`
- 排序须加 ID 兜底（`ORDER BY pdb.score(id) DESC, id ASC`），否则同分结果不稳定
- **UUID 作 key_field 尚未实机验证**：文档未明确背书，若报类型错误需改用 bigint 代理键

### 3. 任务队列约束

- pg_cron 只能配置在 `postgres` 库且不可更改，故超时任务回收由应用层 Go 定时器
  （`pgqueue.StartReclaimer`）执行，不依赖 pg_cron
- 消费者取走任务后须在租约（默认 5 分钟）内调用 `Archive` 或 `Fail`，
  否则任务会被回收重投
- 重试采用指数退避（10s 起，上限 10 分钟），重试耗尽转入死信（`status='failed'`）
- **消息载荷只放 `article_id`**，正文由 worker 按 ID 自行读取。不要把 title/content
  塞进载荷：大文本进队列既浪费空间，又可能与库中内容不一致。
  文章由写入侧同步落库（PRD 要求"提交后立即返回"），worker 只做后续 AI 加工
- **队列转入死信后必须回写文章状态**（`Worker.markArticleFailed`）。流水线开始即把
  状态置为 `processing`，但只有 LLM 阶段失败会自行回写 `failed`，切分/向量/写库
  阶段出错时无人收尾，文章会永久停留在 `processing`。
  回写用独立的带超时 context，不要复用 `w.ctx`——它可能已在停止流程中被取消
- `Create` 入队失败**不**回滚已落库的文章、也不返回错误（否则调用方重试会撞上
  内容去重 409，更难恢复），仅告警并停在 `pending`；`Reprocess` 入队失败**必须**报错

### 4. 迁移执行器约束

- **会跳过一切扩展创建语句**，扩展须由根目录的 `docker-compose-initdb.sql` 创建；
  迁移文件中亦不可在注释里出现该语句的英文字面量，否则会误伤紧随其后的语句
- 已应用的迁移不会重跑，修改既有迁移无效——新逻辑一律放新版本号。
  唯一的例外是"该迁移在任何环境都从未成功过"（如语句报错导致整体回滚，
  `schema_migrations` 里根本没有记录），此时原地修正才是对的，无需新版本号
- **语句按文件内书写顺序逐条执行，不做依赖重排**：带外键的表必须排在被引用表之后。
  001 曾把 `articles`（引用 `categories`）写在 `categories` 之前，
  全新库初始化即报 `relation "categories" does not exist`（v0.5.7 修复）
- `RunMigrations(ctx, tx)` 只接收事务、不负责提交，**调用方必须 Commit**。
  统一走 `cmd/aiknow-server/main.go` 的 `runMigrations()`，不要另写一份事务管理。
  曾出现过 `Begin` + `defer Rollback` 但漏掉 `Commit` 的情况：`serve` 会阻塞在
  HTTP 服务上，导致该事务在进程存活期间一直开着，DDL 始终未提交，
  连接池里其他连接看不到任何新表
- `splitSQL` 支持美元引用（`$$`/`$tag$`）、`$1` 占位符、`''` 转义与注释；
  新增 PL/pgSQL 函数体时无需规避 `;`

### 5. SSRF 防护要求
- 仅允许 HTTP/HTTPS 协议
- 禁止内网 IP：10.0.0.0/8、172.16.0.0/12、192.168.0.0/16、127.0.0.0/8
- DNS 重绑定防护：域名两次解析比对
- 端口限制：80、443、8080、8443
- 超时：连接 5s，读取 10s
- 最大重定向：3 次

### 6. 配置约束

**无条件必填**：数据库连接（`DATABASE_URL` 或 `POSTGRES_*` 分项二者之一）、
`SESSION_SECRET`（≥32 字符），以及 `ADMIN_PASSWORD_HASH` 或 `ADMIN_PASSWORD` 二者之一。

**条件必填**：`QWEN_API_KEY` 仅在 `LLM_PROVIDER=qwen` 时必填；`BAAI_API_KEY` 仅在
`EMBEDDING_PROVIDER` 或 `RERANKER_PROVIDER` 为 `baai` 时必填。三个 Provider 缺省
均为 `mock`，无凭据也能启动。

**优先级**：环境变量 > `.env` > `config.yaml` > 内置默认值。
`.env` 必须在 `.gitignore` 中，仓库只放 `.env.example`。

**新增配置项必须在 `internal/config/config.go` 的 `managedKeys` 中登记**，
仅在结构体上加字段或标签不会生效 —— 三个加载来源共用这份清单。
此前 `PORT` 与 `APP_ENV` 只写了 `env:` 标签而无人读取，导致任何来源都无法覆盖，
端口恒为 8080。若同时需要 YAML 支持，还要在 `YAMLConfig` 与 `toFlat()` 中补映射；
`config_test.go` 的 `TestExampleYAMLMatchesParser` 会守住示例文件与解析器的一致性。

`SESSION_SECRET` 目前只被校验、尚未被消费（Session ID 走密码学随机源并存于服务端）。
保留必填是为 CSRF Token / Cookie 签名预留，勿因"看起来没用"而删除。

**端口不设映射变量**：compose 下宿主机与容器使用同一端口，应用由 `PORT` 控制、
数据库由 `POSTGRES_PORT` 控制，两侧同步生效。监听地址也不提供配置项 ——
容器端口默认发布在所有网卡上，访问控制交由网络层（安全组 / 防火墙 / 反向代理）负责。

**但 `POSTGRES_PORT` 必须在 `environment` 中覆盖为 `5432`**：compose 下 `.env` 里的
该值是宿主机发布端口，容器网络内恒为 5432。若被 `env_file` 原样带入容器（例如用户
为避开端口冲突改成 15432），app 会去连容器网络内并不存在的端口。曾为此把两者拆成
`POSTGRES_PORT` / `POSTGRES_PUBLISH_PORT` 两个变量，后改为单变量 + 编排覆盖 ——
若日后调整这段覆盖逻辑，需保住这条约束。

**数据库连接优先 `DATABASE_URL`，为空时由 `POSTGRES_*` 分项拼装**
（`cfg.DatabaseURL()`，密码经 URL 编码，无需手动转义特殊字符）。
compose 部署下编排会把 `DATABASE_URL` 显式置空 —— 它通常指向 `127.0.0.1`，
而容器内回环地址是容器自己。置空后走分项拼装，故 `.env` 中两者可以并存。
`app` 服务用 `env_file: .env` 整体注入，只在 `environment:` 中覆盖容器内必须不同的项
（`environment` 优先级高于 `env_file`）。

### 7. 反复踩过的编码陷阱

以下每一条都实际造成过线上级别的故障（v0.5.8 一轮联调中集中暴露），改动相关代码时务必对照：

**`sql.ErrNoRows` 只能用 `errors.Is` 判断**

```go
if err == sql.ErrNoRows {            // ✗ 恒为假
if errors.Is(err, sql.ErrNoRows) {   // ✓
```

本项目用 pgx 驱动，`QueryRow(...).Scan` 返回的是 `pgx.ErrNoRows`。它 wrap 了
`sql.ErrNoRows`，故 `errors.Is` 为真而 `==` 为假。写错的后果是"查无结果"这个
正常分支被当成错误往上抛。曾在 9 处同时存在。

**`range` 的第二个返回值是副本**

```go
for i, c := range chunks { c.ArticleID = id }        // ✗ 改不到切片
for i := range chunks { chunks[i].ArticleID = id }   // ✓
```

元素为指针（`[]*T`）时通过指针改字段是有效的，值切片（`[]T`）则必须按下标。
曾导致 chunk 的 `article_id` 保持空串，插库报 `invalid input syntax for type uuid`。

**切分/滑窗循环必须显式判断已到结尾**

末片不满窗口时 `end` 被截到 `totalLen`，而 `start = end - overlap` 会回退到一个
仍小于 `totalLen` 的位置，下一轮 `end` 又被截回 `totalLen`，`start` 恒定不变形成
死循环，切片无限增长直至耗尽内存（实测 RSS 4GB）。`SplitContent` 现以
`end == totalLen` 退出，并有 `ai_pipeline_test.go` 覆盖边界。

**不要用 `c.FileFromFS` 送 HTML**

Gin 会把 `c.Request.URL.Path` 改写成目标路径后交给 `http.FileServer`，而 net/http 的
`serveFile` 对任何以 `/index.html` 结尾的路径**无条件 301 跳到 `./`**，SPA 回退因此
无限重定向（`ERR_TOO_MANY_REDIRECTS`）。改用 `web.serveHTML()`：读出内容后 `c.Data()`。

**gin 的 catch-all 不能与同层静态路由并存**

`/ui/login` 与 `/ui/*path` 同时注册会 panic。同层的具体路径需在 wildcard handler
内部分流。

**占位符绑参顺序**

`LIMIT $N OFFSET $N+1` 必须按 `(pageSize, offset)` 传参。曾传成 `(offset, pageSize)`，
首页 `offset=0` 使 `LIMIT 0`，表现为 `total` 正确但列表恒空。

**改 SELECT 时同步检查 UPDATE 的 SET 列表**

`articleRepo.Update` 曾遗漏 `indexed_at`、`embedding_model`、`embedding_dim`、
`last_error`——AI 流水线正是通过它保存这几项，导致算完的值全都落不了库。

**不要把向量查进检索响应**

`SearchChunk` 不含 `embedding` 字段。1024 维浮点数会被原样序列化（每条约 4KB，
实测响应从 22KB 降到 1.1KB），且 `omitempty` 对非空切片不生效，无法靠标签规避。
`VectorSearch` 的 SQL 也不要 `SELECT c.embedding`。

**业务预期的失败要用哨兵错误**

去重命中返回 `service.ErrDuplicateArticle`，Handler 映射 409；用无类型
`fmt.Errorf` 会让调用方无从区分，一律落到 500。

**JSONB ≠ Go `map[string]interface{}`**

迁移 005 播种的 `settings.value` 是标量（`0.3`、`2000`），而模型声明为
`map[string]interface{}`，导致 `ListAll` 对每一行报 `cannot unmarshal number`。
用 `json.RawMessage` 容纳任意 JSON 值，这是 DB JSONB 列的通用方案。

**模型类型 ≠ DB 列类型**

本项目的 `created_by` / `changed_by` / `deleted_by` 三列建表时定为 UUID，但
v1 无用户表（管理员凭据在配置文件中），系统中不存在可填入的用户 UUID。
实际后果是写入时报 `invalid input syntax for type uuid`（SQLSTATE 22P02）。
在确认需要外键引用前，这些列应保持 TEXT。

**仓库存取时不要自行哈希**

`apikey_repo.Create` 在 `service.HashAPIKey` 已哈希的基础上又做了一次 `sha256`，
等于要求调用方把明文放进名为 `KeyHash` 的字段，实际传入哈希时就会因双重哈希
导致写入值与校验值永不一致。哈希入口必须唯一，仓库层只存不哈希。

**管线硬编码的常量如能被后台修改，应走服务层读取**

`runLLM` 的提示词模板曾硬编码在流水线内部，后台的"提示词"修改对此毫无影响。
改为经 `PromptService` 读取库中模板——否则用户改了配置却看不到效果。

**修改后不持久化的 handler 不是未实现，是已在运行但数据悄悄丢失**

`prompt.UpdatePrompt` 曾做完整的校验流程，然后 `return nil`——调用方收到
200，下次读取仍是旧值。这种"无声无操作"比报错更危险，找起来极费时间。

**Alpine.js CDN 与 ES module 的加载顺序**

Alpine CDN 和 type=module 的 app.js 都是异步加载，但按文档顺序解析。
若 Alpine 先启动，它会在 app.js 注册 `alpine:init` 监听器之前就 `start()`
并派发该事件，导致组件全部未注册（`shell is not defined`）。
app.js 必须排在 Alpine CDN 之前。

### 8. 目录结构
```
aiknow/
├── cmd/aiknow-server/      # 服务入口（含 hashpwd 子命令）
├── internal/
│   ├── handler/
│   │   ├── api/            # 对外 API（API Key 鉴权）
│   │   ├── admin/          # 后台管理 API（Session 鉴权）
│   │   ├── web/            # Web UI 路由
│   │   └── mcp/            # MCP Streamable HTTP
│   ├── service/            # 业务逻辑
│   ├── repository/         # 数据访问
│   ├── model/              # 数据模型
│   └── config/             # config.go(L1) / defaults.go(L2默认) / cache.go(热重载)
├── pkg/                    # 公共库
│   ├── ssrf/               # SSRF 防护（协议/端口/IP 校验）
│   ├── htmlclean/          # HTML 清洗（标签移除 + 实体解码）
│   └── pgqueue/            # 任务队列（表 + FOR UPDATE SKIP LOCKED，含租约/重试/死信）
├── web/js/pages/           # 前端按页面拆分
├── migrations/             # 数据库迁移
├── .env.example            # .env 本身在 .gitignore 中
└── docs/                   # 文档
```

### 9. 开发路线图（7 周）
| Phase | 周 | 内容 | 状态 |
|-------|-----|------|------|
| 1 | W1 | 基础框架 + 配置分层加载（env > .env > yaml） | ✅ 骨架完成 |
| 2 | W2 | 采集 + AI 处理（单次调用 + JSON 校验） |
| 3 | W3 | 检索核心（向量 + BM25 + RRF） |
| 4 | W4 | 精排 + 后台基础页 |
| 5 | W5 | MCP + API + 鉴权/限流/MCP 日志记录 |
| 6 | W6 | 调试页、MCP 日志页、配置页 |
| 7 | W7 | 测试与发布 |

## 重要链接

- PRD 文档：`docs/PRD.md`
- 变更日志：`docs/CHANGELOG.md`
- 部署与运维：`docs/DEPLOY.md`
- ParadeDB / pg_search：https://github.com/paradedb/paradedb
- pgvector：https://github.com/pgvector/pgvector
- MCP 规范：https://modelcontextprotocol.io/specification/2025-06-18
