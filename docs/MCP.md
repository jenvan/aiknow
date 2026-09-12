# AiKnow MCP 接入文档

## 协议版本
MCP 2025-06-18 (Streamable HTTP)

## 端点
`POST /mcp` 或 `GET /mcp`

## 认证
```
Authorization: Bearer <api_key>
```

## 可用工具

### search_knowledge (readonly)
语义检索，返回 chunk 级证据块。

**参数**：
- `query` (string): 检索查询
- `limit` (integer, default: 5): 返回结果数
- `category` (string, 可选): 按分类名称过滤（大小写不敏感），不传则检索全部分类

### find_articles_by_keywords (readonly)
关键词检索，返回文章卡片。

**参数**：
- `keywords` (array): 关键词列表
- `limit` (integer, default: 5): 返回数量
- `tokenized` (boolean, default: true): 是否启用 jieba 分词匹配。true（默认）分词，"父母"可匹配"父母牵线"；false 精确整词匹配，"小日本"不会被拆分

### get_article_detail (readonly)
获取文章全文。

**参数**：
- `article_id` (string): 文章 ID

### add_article_by_url (readwrite)
通过 URL 采集入库。

**参数**：
- `url` (string): 文章 URL

## Claude Desktop 配置

```json
{
  "mcpServers": {
    "aiknow": {
      "command": "http",
      "args": ["http://localhost:8080/mcp"],
      "env": {
        "AUTHORIZATION": "Bearer YOUR_API_KEY"
      }
    }
  }
}
```
