# AiKnow API 文档

## 概览

| 项目 | 说明 |
|------|------|
| 基础地址 | `http://<host>:<port>`（默认端口 `8080`） |
| 版本前缀 | `/api/v1` |
| 数据格式 | JSON（请求与响应均为 `application/json`） |
| 时区 | UTC |

## 认证方式

### API Key 鉴权（对外 API）

所有对外接口（`/api/v1/articles`、`/api/v1/search`）均需在请求头中携带 API Key：

```http
Authorization: Bearer <api_key>
```

API Key 以 `ak_` 开头，例如 `ak_8f3a2b1c9d4e5f6a7b8c9d0e1f2a3b4c`。

**Scope 权限模型：**

| Scope | 说明 | 可访问接口 |
|-------|------|-----------|
| `readonly` | 只读 | 文章查询、检索 |
| `readwrite` | 读写 | 全部对外接口（含录入、删除） |

`readwrite` 隐含 `readonly` 权限。

### Session 鉴权（后台管理）

后台管理接口使用 HTTP Cookie + CSRF Token 鉴权，供 Web UI 使用。
完整接口说明见 [docs/API_ADMIN.md](./API_ADMIN.md)。

---

## 错误响应

所有接口在异常时返回统一结构：

```json
{
  "error": "错误描述"
}
```

常见 HTTP 状态码：

| 状态码 | 含义 |
|--------|------|
| `400` | 请求参数错误 |
| `401` | 缺少或无效的认证凭据 |
| `403` | Scope 权限不足 |
| `404` | 资源不存在 |
| `409` | 资源冲突（如重复录入同一 URL） |
| `429` | 请求过于频繁 |
| `500` | 服务器内部错误 |

---

## 接口列表

### 健康检查

无鉴权。

```
GET /api/v1/health
```

**响应示例：**

```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

---

### 文章

#### 创建文章

```
POST /api/v1/articles
```

需要 `readwrite` scope。调用方需提供完整正文，服务端**不代为抓取** URL。

支持两种来源标记（仅元数据用途，不影响处理逻辑）：

**方式一：手动录入（`source_type=manual`，默认）**

```json
{
  "title": "Go 并发模式：Worker Pools",
  "content": "本文介绍 Go 语言中 Worker Pool 的典型实现模式...",
  "source_type": "manual",
  "tags": ["go", "并发"],
  "category_name": "编程"
}
```

**方式二：URL 来源（`source_type=url`，需同时提供已抓取的正文）**

```json
{
  "title": "Go 并发模式：Worker Pools",
  "content": "本文介绍 Go 语言中 Worker Pool 的典型实现模式...",
  "source_type": "url",
  "source_url": "https://example.com/article",
  "tags": ["example"],
  "category_name": ""
}
```

> 💡 **URL 预览**：后台管理接口 `POST /api/v1/admin/articles/preview` 可先用一个 URL 抓取预览正文再决定提交，但该端点仅限 Session 登录用户调用。

字段约束：

| 字段 | 类型 | 必填 | 最大长度 | 说明 |
|------|------|------|---------|------|
| `title` | string | 是 | 500 | 文章标题 |
| `content` | string | 是 | — | 完整正文，服务端按此生成 embedding 与分块 |
| `source_type` | string | 否 | — | `manual`（默认）或 `url` |
| `source_url` | string | 否 | — | 仅 `source_type=url` 时填写 |
| `category_name` | string | 否 | — | 分类名，不存在时自动创建 |
| `category_id` | string | 否 | — | 分类 ID，与 `category_name` 二选一，优先 ID |
| `tags` | string[] | 否 | — | 标签列表，不存在时自动创建 |

**响应示例（201 Created）：**

```json
{
  "id": "7c9a3b2e-1d4f-4a8b-9e0c-5f6d7a8b9c0d",
  "title": "Go 并发模式：Worker Pools",
  "content": "...",
  "source_type": "url",
  "source_url": "https://example.com/article",
  "processing_status": "pending",
  "summary": null,
  "category_id": "cat-001",
  "category_name": "编程",
  "tags": [{"id": "tag-001", "name": "go"}, {"id": "tag-002", "name": "并发"}],
  "published": false,
  "view_count": 0,
  "retry_count": 0,
  "created_at": "2026-09-10T12:00:00Z",
  "updated_at": "2026-09-10T12:00:00Z"
}
```

`processing_status` 取值：`pending` → `processing` → `completed` / `failed`。

**冲突响应（409）：** 同一 `source_url` 已存在时返回：

```json
{"error": "article already exists with this source url"}
```

#### 查询文章列表

```
GET /api/v1/articles
```

需要 `readonly` scope。

**查询参数：**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `page` | int | `1` | 页码，从 1 开始 |
| `page_size` | int | `20` | 每页条数，最大 100 |
| `category_id` | string | — | 按分类 ID 过滤 |
| `tag_id` | string (可多选) | — | 按标签 ID 过滤，可多次出现 |
| `status` | string | — | `pending` / `processing` / `completed` / `failed` |
| `search` | string | — | 按标题模糊搜索 |
| `sort_by` | string | `created_at` | `created_at` / `updated_at` / `view_count` |
| `sort_order` | string | `desc` | `asc` / `desc` |
| `date_from` | string (RFC3339) | — | 创建时间下限（包含） |
| `date_to` | string (RFC3339) | — | 创建时间上限（包含） |

**响应示例：**

```json
{
  "total": 150,
  "pages": 8,
  "articles": [
    {
      "id": "7c9a3b2e-1d4f-4a8b-9e0c-5f6d7a8b9c0d",
      "title": "Go 并发模式：Worker Pools",
      "source_type": "url",
      "source_url": "https://example.com/article",
      "processing_status": "completed",
      "summary": "Worker Pool 是 Go 并发编程中最常用的模式之一...",
      "category_id": "cat-001",
      "category_name": "编程",
      "tags": [{"id": "tag-001", "name": "go"}, {"id": "tag-002", "name": "并发"}],
      "published": true,
      "view_count": 42,
      "indexed_at": "2026-09-10T12:01:00Z",
      "created_at": "2026-09-10T12:00:00Z",
      "updated_at": "2026-09-10T12:00:00Z"
    }
  ]
}
```

#### 获取文章详情

```
GET /api/v1/articles/:id
```

需要 `readonly` scope。

**响应：** 单篇文章对象，字段同列表项，另含完整 `content` 正文。

#### 软删除文章

```
DELETE /api/v1/articles/:id
```

需要 `readwrite` scope。文章标记删除后不再出现在列表和检索结果中，但数据保留在数据库中。

---

### 检索

#### 混合检索（向量 + BM25 + Rerank）

```
POST /api/v1/search
```

需要 `readonly` scope。结合语义向量检索与关键词 BM25 检索，并对前 N 个候选 rerank 重排序。

**请求体：**

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `query` | string | 是 | — | 检索查询语句 |
| `params.top_k` | int | 否 | `10` | 最终返回条目数 |
| `params.candidate_k` | int | 否 | `50` | 向量/BM25 召回候选数 |
| `params.rrf_k` | int | 否 | `60` | RRF 融合公式中的常数 k |
| `params.vector_weight` | float | 否 | `0.5` | 向量分数权重 |
| `params.bm25_weight` | float | 否 | `0.5` | BM25 分数权重 |
| `params.ef_search` | int | 否 | `40` | pgvector 近似搜索精度（越大越准越慢） |

**响应示例：**

```json
{
  "query": "Go worker pool",
  "results": [
    {
      "id": "chunk-abc123",
      "article_id": "7c9a3b2e-1d4f-4a8b-9e0c-5f6d7a8b9c0d",
      "chunk_index": 0,
      "content": "Worker Pool 模式通过固定数量的 worker goroutine 处理任务队列...",
      "score": 0.873,
      "rrf_score": 0.0167,
      "vector_rank": 2,
      "bm25_rank": 1
    }
  ],
  "timings": {
    "embedding_ms": 12,
    "vector_ms": 8,
    "bm25_ms": 3,
    "rrf_ms": 1,
    "rerank_ms": 45,
    "total_ms": 69
  }
}
```

`rrf_score` 为倒数排名融合得分，越高越相关；`vector_rank` / `bm25_rank` 为各子检索的排名（1 为最优）。

#### 关键词检索

```
POST /api/v1/search/keywords
```

需要 `readonly` scope。基于文章标签、分类、标题、摘要、正文字段的关键词匹配，适合精确术语查找。

**请求体：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `keywords` | string[] | 是 | 关键词列表，至少 1 个 |
| `params.top_k` | int | 否 | 返回条数，默认 `10` |
| `params.tag_weight` | float | 否 | 标签字段权重，默认 `3.0` |
| `params.category_weight` | float | 否 | 分类字段权重，默认 `2.0` |
| `params.title_weight` | float | 否 | 标题字段权重，默认 `2.5` |
| `params.summary_weight` | float | 否 | 摘要字段权重，默认 `1.5` |
| `params.content_weight` | float | 否 | 正文字段权重，默认 `1.0` |

**响应示例：**

```json
{
  "total": 3,
  "articles": [
    {
      "id": "7c9a3b2e-1d4f-4a8b-9e0c-5f6d7a8b9c0d",
      "title": "Go 并发模式：Worker Pools",
      "source_type": "url",
      "source_url": "https://example.com/article",
      "processing_status": "completed",
      "summary": "Worker Pool 是 Go 并发编程中最常用的模式之一...",
      "category_id": "cat-001",
      "category_name": "编程",
      "tags": [{"id": "tag-001", "name": "go"}, {"id": "tag-002", "name": "并发"}],
      "published": true,
      "view_count": 42,
      "created_at": "2026-09-10T12:00:00Z"
    }
  ]
}
```

---

### MCP 工具

MCP（Model Context Protocol）接口暴露检索能力给 AI 助手调用，使用 JSON-RPC 2.0 协议。

```
GET /mcp
POST /mcp
```

需携带有效的 API Key（`Authorization: Bearer <key>`）。

详见 [docs/MCP.md](./MCP.md)。

---

## 速率限制

当前版本暂无速率限制。

---

## 快速开始

```bash

# 采集一篇文章
curl -X POST http://localhost:8080/api/v1/articles \
  -H "Authorization: Bearer ak_XXXXXXXXXXXXXXXXXXXXXXXX" \
  -H "Content-Type: application/json" \
  -d '{"source_type":"url","source_url":"https://example.com/article"}'

# 搜索
curl -X POST http://localhost:8080/api/v1/search \
  -H "Authorization: Bearer ak_XXXXXXXXXXXXXXXXXXXXXXXX" \
  -H "Content-Type: application/json" \
  -d '{"query":"Go 并发编程","params":{"top_k":5}}'
```

---

## 附录

### 字段说明

**Article.processing_status**

| 值 | 说明 |
|----|------|
| `pending` | 等待处理 |
| `processing` | AI 处理中（分块 / Embedding / 写入向量） |
| `completed` | 处理完成，可被检索 |
| `failed` | 处理失败，可查看 `last_error` 字段排查 |

**Article.source_type**

| 值 | 说明 |
|----|------|
| `manual` | 手动录入，`content` 字段非空 |
| `url` | URL 采集，`source_url` 字段非空 |

### CLI 生成 API Key

不登录后台时，可通过 CLI 子命令创建：

```bash
aiknow-server apikey create --desc "my-bot" --scope readwrite
```
