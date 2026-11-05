# AiKnow 后台管理接口文档（TAD）

> 本文档记录仅供 Web UI 与后台自动化脚本使用的 **Session 鉴权接口**，对外 API 接口见 [docs/API.md](./API.md)。
>
> 所有后台接口路径前缀为 `/api/v1/admin`。

---

## 认证流程

### 1. 登录

```
POST /api/v1/admin/auth/login
Content-Type: application/json
```

**请求体：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `username` | string | 是 | 管理员用户名 |
| `password` | string | 是 | 管理员密码 |

**响应（200 OK）：**

```json
{
  "success": true,
  "csrf_token": "a1b2c3d4e5f6..."
}
```

- 服务端在响应头中设置 `Set-Cookie: session_id=xxx; HttpOnly; SameSite=Lax; Max-Age=86400`
- `csrf_token` 存于服务端，随响应体一并返回；后续写请求须在请求头携带

### 2. 刷新 Session / 获取 CSRF Token

```
GET /api/v1/admin/auth/session
Cookie: session_id=<sessionId>
```

**响应（200 OK）：**

```json
{
  "username": "admin",
  "csrf_token": "a1b2c3d4e5f6..."
}
```

> 页面加载时调用此接口恢复登录态并取到最新 CSRF Token。
> 登录响应中的 CSRF Token 在浏览器刷新后失效，须从此接口重新获取。

### 3. 登出

```
POST /api/v1/admin/auth/logout
Cookie: session_id=<sessionId>
```

**响应：**

```json
{ "success": true }
```

同时清除服务端 Session 与客户端 Cookie。

### 4. CSRF 规则

| 方法 | 是否校验 CSRF |
|------|-------------|
| `GET` / `HEAD` / `OPTIONS` | 否 |
| `POST` / `PUT` / `DELETE` | **是**，须携带 `X-CSRF-Token` 请求头 |

---

## 错误响应

```json
{ "error": "错误描述" }
```

| 状态码 | 含义 |
|--------|------|
| `400` | 参数错误 |
| `401` | 未登录 / Session 无效 |
| `403` | CSRF 校验失败 |
| `404` | 资源不存在 |
| `409` | 资源冲突 |
| `429` | 登录失败次数过多，账号已临时锁定（5 次失败 → 15 分钟锁定） |
| `500` | 服务器内部错误 |

---

## 接口清单

### 文章管理

#### 文章列表

```
GET /api/v1/admin/articles
```

查询参数与对外 API 相同（[docs/API.md](./API.md)「查询文章列表」），另支持按 `deleted_at IS NULL` 过滤。

#### 创建文章

```
POST /api/v1/admin/articles
Header: X-CSRF-Token
```

请求体同对外 API 的 `POST /api/v1/articles`（[docs/API.md](./API.md)「创建文章」）。

**区别**：后台接口额外支持 URL 预览与编辑已有文章。

#### URL 采集预览（不入库）

```
POST /api/v1/admin/articles/preview
Header: X-CSRF-Token
```

**请求体：**

```json
{ "url": "https://example.com/article" }
```

**响应：**

```json
{
  "title": "文章标题",
  "content": "清洗后的正文纯文本",
  "markdown": "# 文章标题\n\n保留结构的正文",
  "rendered": true,
  "truncated": false
}
```

> 用于录入页在提交前预览抓取结果，SSRF 校验在 collector 内部完成。
> 渲染开启时 `rendered=true`，`content` 为渲染后提取的纯文本，`markdown` 保留
> 标题/列表/代码块结构（录入页优先提交它作为正文）。截图为二进制，不上传、
> 不出现在响应中；URL 文章在异步处理时会重新采集并归档截图到
> `articles.screenshot_url`。

#### 获取文章详情

```
GET /api/v1/admin/articles/:id
```

#### 更新文章

```
PUT /api/v1/admin/articles/:id
Header: X-CSRF-Token
```

请求体同创建接口。

#### 软删除文章

```
DELETE /api/v1/admin/articles/:id
Header: X-CSRF-Token
```

#### 批量删除

```
POST /api/v1/admin/articles/batch-delete
Header: X-CSRF-Token
Content-Type: application/json
```

**请求体：** ID 数组

```json
["id-1", "id-2", "id-3"]
```

**响应：**

```json
{ "success": true, "deleted": 3 }
```

#### 获取文章 Chunks

```
GET /api/v1/admin/articles/:id/chunks
```

**响应：**

```json
{
  "chunks": [
    {
      "id": "chunk-xxx",
      "article_id": "article-xxx",
      "chunk_index": 0,
      "content": "分块文本内容",
      "word_count": 320,
      "created_at": "2026-09-10T12:01:00Z"
    }
  ],
  "total": 5
}
```

#### 重新处理文章

```
POST /api/v1/admin/articles/:id/reprocess
Header: X-CSRF-Token
```

文章重新进入 AI 处理流水线（分块 / Embedding / 写入向量）。适用于处理失败后手动重试。

#### 批量重新处理

```
POST /api/v1/admin/articles/batch-reprocess
Header: X-CSRF-Token
Content-Type: application/json
```

**请求体：** ID 数组。

**响应：**

```json
{
  "success": true,
  "reprocessed": 8,
  "failed": { "id-3": "upsert chunk: ..." }
}
```

---

### API Key 管理

#### 列出所有 Key

```
GET /api/v1/admin/api-keys
```

响应同 [docs/API.md](./API.md)「API Key 管理」章节。

#### 创建 Key

```
POST /api/v1/admin/api-keys
Header: X-CSRF-Token
Content-Type: application/json
```

**请求体：**

```json
{
  "description": "外部采集机器人",
  "scope": "readwrite"
}
```

`scope` 默认 `readonly`。响应中的 `key` 字段**仅此一次**可见。

#### 启用 / 禁用 Key

```
POST /api/v1/admin/api-keys/:id/enable
POST /api/v1/admin/api-keys/:id/disable
Header: X-CSRF-Token
```

禁用后该 Key 所有请求返回 `401 invalid API key`。

#### 删除 Key

```
DELETE /api/v1/admin/api-keys/:id
Header: X-CSRF-Token
```

---

### 仪表盘

#### 统计数据

```
GET /api/v1/admin/dashboard/stats
```

**响应：**

```json
{
  "total_articles": 150,
  "total_chunks": 820,
  "total_tags": 45,
  "total_categories": 8,
  "status_distribution": {
    "completed": 140,
    "pending": 5,
    "processing": 3,
    "failed": 2
  }
}
```

#### 队列状态

```
GET /api/v1/admin/dashboard/queue
```

**响应：**

```json
{
  "depth": 3,
  "oldest_message_age_ms": 12000,
  "dlq_count": 0
}
```

#### 最近活动

```
GET /api/v1/admin/dashboard/activity
```

**响应：**

```json
{
  "recent_articles": [ ... ],
  "failed_articles": [ ... ]
}
```

- `recent_articles`：最近 10 条文章（含 id、title、source_type、processing_status、created_at）
- `failed_articles`：最近 10 条失败文章（含 last_error）

#### 服务健康

```
GET /api/v1/admin/dashboard/health
```

**响应：**

```json
{
  "database": "connected",
  "llm_provider": "qwen",
  "embedding_provider": "baai",
  "reranker_provider": "baai"
}
```

`database` 为 `error: <msg>` 时表示数据库连通异常。

---

### 检索调试

#### 混合检索调试

```
POST /api/v1/admin/search/debug
Header: X-CSRF-Token
Content-Type: application/json
```

**请求体：**

```json
{
  "query": "Go 并发编程",
  "params": {
    "top_k": 10,
    "candidate_k": 50,
    "rrf_k": 60,
    "vector_weight": 0.5,
    "bm25_weight": 0.5,
    "ef_search": 40
  }
}
```

**响应：**

```json
{
  "search_params": { ... },
  "vector_result": { "results": [...], "timings": {...} },
  "bm25_result": { "results": [...], "timings": {...} },
  "rerank_result": { "results": [...], "timings": {...} },
  "keyword_result": null
}
```

展示各检索阶段的候选与耗时，便于调参。

#### 关键词检索调试

```
POST /api/v1/admin/search/debug/keywords
Header: X-CSRF-Token
Content-Type: application/json
```

**请求体：**

```json
{
  "keywords": ["Go", "并发"],
  "params": {
    "top_k": 10,
    "tag_weight": 3.0,
    "category_weight": 2.0,
    "title_weight": 2.5,
    "summary_weight": 1.5,
    "content_weight": 1.0
  }
}
```

**响应：**

```json
{
  "keywords": ["Go", "并发"],
  "total": 3,
  "articles": [ ... ],
  "query_time_ms": 12
}
```

---

### MCP 调试代理

```
POST /api/v1/admin/mcp/debug
Header: X-CSRF-Token
Content-Type: application/json
```

后端代理 MCP 调用，无需持有 API Key，调用日志统一记录。

**请求体：**

```json
{
  "method": "tools/call",
  "tool": "search_articles",
  "arguments": { "query": "Go worker pool" },
  "scope": "readonly"
}
```

| 字段 | 说明 |
|------|------|
| `method` | `initialize` / `tools/list` / `tools/call` |
| `tool` | 工具名称（`tools/call` 时必填） |
| `arguments` | 工具入参（`tools/call` 时传入） |
| `scope` | 模拟的权限范围，不传默认 `readonly` |

**响应：**

```json
{
  "request": { ... },
  "response": { "content": [...] },
  "duration_ms": 85,
  "scope": "readonly"
}
```

调用失败时返回 `error` 字段而非仅状态码，便于排查。

---

### MCP 日志

#### 日志列表

```
GET /api/v1/admin/logs
```

查询参数：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `page` | int | `1` | 页码 |
| `page_size` | int | `20` | 每页条数 |
| `trace_id` | string | — | 按 Trace ID 过滤 |
| `method` | string | — | 按 method 过滤（如 `tools/call`） |
| `tool_name` | string | — | 按工具名过滤 |
| `keyword` | string | — | 全文检索 request_body |
| `date_from` | string (RFC3339) | — | 创建时间下限 |
| `date_to` | string (RFC3339) | — | 创建时间上限 |
| `status` | string | — | `success` / `error` |

#### 日志详情

```
GET /api/v1/admin/logs/:trace_id
```

**响应：**

```json
{
  "id": "log-xxx",
  "trace_id": "trace-xxx",
  "method": "tools/call",
  "tool_name": "search_articles",
  "request_body": { "query": "..." },
  "response_body": { "results": [...] },
  "response_headers": { "Content-Type": "application/json" },
  "error_message": null,
  "query_time_ms": 120,
  "api_key_id": "ak_8f3a2b...",
  "created_at": "2026-09-10T12:00:00Z"
}
```

#### 清理过期日志

```
DELETE /api/v1/admin/logs
Header: X-CSRF-Token
```

查询参数 `days`（默认取 `mcp_log.retention_days`，再缺省 30 天）。

**响应：**

```json
{ "success": true, "deleted": 152, "before": "2026-08-11T12:00:00Z" }
```

---

### 系统配置

#### 获取全部配置

```
GET /api/v1/admin/settings
```

**响应：**

```json
{
  "defaults": {
    "llm.provider": "mock",
    "llm.model": "qwen-turbo",
    "llm.temperature": 0.3,
    "embedding.provider": "mock",
    "chunk.max_tokens": 512,
    ...
  },
  "overrides": {
    "llm.model": "qwen-plus"
  }
}
```

- `defaults`：完整可配置项清单（内置默认值）
- `overrides`：当前被显式覆盖的配置项

#### 获取单个配置

```
GET /api/v1/admin/settings/:key
```

#### 更新配置

```
PUT /api/v1/admin/settings/:key
Header: X-CSRF-Token
Content-Type: application/json
```

**请求体：** 只传内层 value，不含 key。

```json
{ "value": "qwen-plus" }
```

或数值型：

```json
{ "value": 0.5 }
```

配置变更后 L2 缓存立即热生效，无需重启。

#### 重置单个配置

```
POST /api/v1/admin/settings/:key/reset
Header: X-CSRF-Token
```

删除该 key 的覆盖值，回落到 L2 默认值。

#### 重置所有配置

```
POST /api/v1/admin/settings/reset
Header: X-CSRF-Token
```

**响应：**

```json
{ "success": true, "reset": 42 }
```

#### 配置变更历史

```
GET /api/v1/admin/settings/:key/history?limit=20
```

**响应：**

```json
{
  "key": "llm.model",
  "history": [
    {
      "id": "hist-001",
      "key": "llm.model",
      "old_value": "qwen-turbo",
      "new_value": "qwen-plus",
      "changed_by": "admin",
      "changed_at": "2026-09-10T10:00:00Z"
    }
  ]
}
```

---

### 提示词管理

#### 获取提示词模板

```
GET /api/v1/admin/prompts/:key
```

#### 更新提示词模板

```
PUT /api/v1/admin/prompts/:key
Header: X-CSRF-Token
Content-Type: application/json
```

**请求体：**

```json
{ "value": "请为以下文章生成摘要...\n{{.Content}}\n分类：\n{{.Categories}}" }
```

保存时服务端校验：模板须含 `{{.Content}}`、`{{.Categories}}` 和 `"json"` 字样（Qwen JSON 模式要求）。

#### 测试提示词

```
POST /api/v1/admin/prompts/test
Header: X-CSRF-Token
Content-Type: application/json
```

**请求体：**

```json
{
  "article_id": "article-xxx",
  "template": "...自定义模板..."
}
```

展示渲染后的 Prompt、模型原始返回与校验结果，**不写入数据**。

---

### 微信入口

管理微信 iLink Bot：状态、扫码登录、删除凭据。设计与排障见
[docs/WEIXIN_BOT.md](WEIXIN_BOT.md)。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/admin/weixin/status` | Bot 状态 |
| POST | `/api/v1/admin/weixin/login/start` | 生成扫码链接 |
| GET | `/api/v1/admin/weixin/login/poll` | 轮询扫码结果 |
| POST | `/api/v1/admin/weixin/logout` | 删除本地凭据 |

`GET /weixin/status` 响应（`logged_in=false` 时不含 bot 字段）：

```json
{
  "enabled": true,
  "logged_in": true,
  "bot_id": "xxx@im.bot",
  "owner_user_id": "xxx@im.wechat",
  "allow_from": "",
  "cursor_saved": true,
  "last_seen_at": "2026-11-04T15:52:03+08:00",
  "updated_at": "2026-11-04T15:52:03+08:00"
}
```

`POST /weixin/login/start` → `{"url":"https://liteapp.weixin.qq.com/q/xxx","reused":false}`。
**不返回二维码图片**：`url` 用手机微信打开即等于扫码。4 分钟内重复调用复用同一二维码
（`reused=true`），避免连点让先前那个失效。

`GET /weixin/login/poll` → `{"status":"wait|scaned|confirmed|expired|none"}`，
`confirmed` 时额外返回 `bot_id`，此时凭据已落库，并在重启 `serve` 后生效。

> 白名单（`allow_from`）已移除：Bot 只放行扫码者本人。原因是它只在启动时读一次，
> 后台改完不重启不生效，实际从未起作用。

### 文件传输助手通道

管理网页版协议通道（只收转发来的公众号文章）。设计见
[docs/WECHATWEB.md](WECHATWEB.md)。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/admin/wechatweb/status` | 登录态 / 是否在运行 / 游标项数 / 最近同步 |
| POST | `/api/v1/admin/wechatweb/login/start` | 生成扫码链接 |
| GET | `/api/v1/admin/wechatweb/login/poll` | 轮询扫码（confirmed 时落库并**当场拉起接收器**） |
| POST | `/api/v1/admin/wechatweb/logout` | 停止接收并删除凭据 |

`GET /wechatweb/status`：

```json
{"logged_in": true, "running": true, "user_name": "@cb22...",
 "sync_key_items": 13, "last_sync_at": "2026-11-05T22:45:13+08:00",
 "last_error": ""}
```

`login/poll` 的 `status`：`wait` / `scaned` / `confirmed` / `none`。
`confirmed` 时额外返回 `user_name` 与 `started`（接收器是否已启动）——**无需重启 serve**。

## 快速上手

```bash
# 1. 登录
curl -s -c /tmp/cookies.txt -X POST http://localhost:8080/api/v1/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
# 提取响应中的 csrf_token

# 2. 创建 API Key
curl -s -b /tmp/cookies.txt \
  -H "X-CSRF-Token: <csrf_token>" \
  -X POST http://localhost:8080/api/v1/admin/api-keys \
  -H "Content-Type: application/json" \
  -d '{"description":"my-bot","scope":"readwrite"}'

# 3. URL 预览
curl -s -b /tmp/cookies.txt \
  -H "X-CSRF-Token: <csrf_token>" \
  -X POST http://localhost:8080/api/v1/admin/articles/preview \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/article"}'

# 4. 登出
curl -s -b /tmp/cookies.txt \
  -X POST http://localhost:8080/api/v1/admin/auth/logout
```
