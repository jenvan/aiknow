# AiKnow API 文档

## 认证方式

### API Key 鉴权
所有 `/api/v1/*` 接口需要在请求头中携带 API Key：
```
Authorization: Bearer <api_key>
```

### Session 鉴权
后台管理接口使用 Session Cookie：
```
Cookie: session_id=<session_id>
```

## 错误响应

```json
{
  "error": "错误描述"
}
```

## 接口列表

### 健康检查
- `GET /api/v1/health`
- 返回：`{"status": "ok", "version": "0.1.0"}`

### 认证
- `POST /api/v1/admin/auth/login`
- `POST /api/v1/admin/auth/logout`
- `GET /api/v1/admin/auth/session`

### 文章
- `GET /api/v1/articles` - 文章列表
- `GET /api/v1/articles/:id` - 文章详情
- `POST /api/v1/articles` - 创建文章
- `DELETE /api/v1/articles/:id` - 软删除

### 检索
- `POST /api/v1/search` - 混合检索
- `POST /api/v1/search/keywords` - 关键词检索
