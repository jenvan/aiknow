# 配置层级设计（v0.6.2）

## 配置架构

### L1：启动配置（config.yaml / .env / 环境变量）
**优先级**：环境变量 > .env > config.yaml > 代码默认值
**生效时机**：需要重启服务

#### 必须在config.yaml中体现的配置：

**数据库（DATABASE_URL 优先）**
- DATABASE_URL
- POSTGRES_HOST / POSTGRES_PORT / POSTGRES_USER / POSTGRES_PASSWORD / POSTGRES_DB / POSTGRES_SSLMODE

**服务器**
- PORT（默认 8080）
- APP_ENV（development/production）

**安全与会话**
- SESSION_SECRET（≥32字符）
- SESSION_LIFETIME（默认 24h）
- ADMIN_USERNAME / ADMIN_PASSWORD / ADMIN_PASSWORD_HASH

**AI Provider 选择与凭据**
- LLM_PROVIDER（qwen/mock）
- EMBEDDING_PROVIDER（baai/mock）
- RERANKER_PROVIDER（baai/mock）
- QWEN_API_KEY / QWEN_BASE_URL / QWEN_MODEL
- BAAI_API_KEY / BAAI_BASE_URL

**采集（基线配置，L2可覆盖）**
- COLLECTOR_USER_AGENT

**固定配置（与表结构绑定，不在后台显示）**
- EMBEDDING_DIMENSION = 1024（必须删除L2版本）
- HNSW_M = 16（建表参数）
- HNSW_EF_CONSTRUCTION = 64（建表参数）

---

### L2：运行时配置（settings表，管理后台可改）
**优先级**：L2设置 > L1配置（覆盖）
**生效时机**：立即热生效，支持在线修改

#### L2配置清单（共23项）：

**LLM（4项）**
- llm.model（qwen-plus等，支持在线切换模型）
- llm.temperature（0.0-2.0）
- llm.max_tokens（100-8000）
- llm.timeout_ms（5000-120000）
- llm.retry_count（0-5）

**Embedding（1项）**
- embedding.batch_size（1-64）

**Reranker（1项）**
- reranker.top_k（1-50）

**搜索（6项）**
- search.top_k（1-50）
- search.candidate_k（5-200）
- search.rrf_k（1-200）
- search.vector_weight（0.0-1.0）
- search.bm25_weight（0.0-1.0）
- search.ef_search（10-500）

**分块（2项）**
- chunk.size（200-800）
- chunk.overlap（0-200）

**关键词搜索（5项）**
- keyword_search.tag_weight
- keyword_search.category_weight
- keyword_search.title_weight
- keyword_search.summary_weight
- keyword_search.content_weight

**日志（1项）**
- mcp_log.retention_days（1-365）

**采集（1项，可覆盖L1）**
- collector.user_agent（留空回落L1值）

**提示词（1项）**
- prompt.analyze（支持热更新）

---

## 需要实施的改动

### 1. 数据库迁移
- 删除`embedding.dimension`（固定为1024，与表结构绑定）
- 删除`collector.timeout_ms`、`collector.max_redirects`、`collector.min_content_length`
- 添加`llm.model`（默认值从L1的QWEN_MODEL获取）

### 2. 代码修改

#### model/setting.go
- 从L2Config结构体中移除：
  - Embedding.Dimension
  - Collector.TimeoutMS, MaxRedirects, MinContentLength
- 添加：
  - LLM.Model（string类型）

#### config/cache.go
- applySetting中添加llm.model处理
- 移除已删除配置项的处理逻辑

#### cmd/aiknow-server/main.go
- collector初始化：user_agent从L1读取，然后被L2覆盖（已实现）
- pipeline初始化：llm.model从L2读取（需新增）

### 3. 前端配置页优化
- 按功能分组显示（LLM、搜索、分块等）
- 移除不该显示的配置
- 排序优化
- 支持所有L2配置的在线修改

### 4. 消费端接入（已完成）

各服务通过 `service.L2ConfigSource` 接口注入 `config.ConfigCache`，每次调用读取最新 L2 值：

| 消费端 | 读取的 L2 配置 | 生效时机 |
|--------|---------------|---------|
| AIPipeline（worker 文章处理） | `llm.model` / `temperature` / `max_tokens`、`chunk.size` / `chunk.overlap` | 每篇文章处理时 |
| SearchService（混合检索） | `search.top_k` / `candidate_k` / `rrf_k` / `vector_weight` / `bm25_weight` / `ef_search`、`reranker.top_k`（精排候选上限） | 每次检索时 |
| KeywordSearchService | `keyword_search.*`（5 项权重） | 每次关键词检索时 |
| QwenProvider | `llm.model`（`.SetModelSource`，覆盖 L1 QWEN_MODEL） | 每次调用时 |
| Collector | `collector.user_agent`（读 settings 表，覆盖 L1 COLLECTOR_USER_AGENT） | 每次抓取时 |
| LogHandler `/logs` DELETE | `mcp_log.retention_days` | 清理日志时 |

> 说明：`embedding.batch_size` 已入库可编辑，但当前 Embedding 调用为逐条执行
> （`EmbedBatch` 内部循环单条），尚无真正的批量调用入口，暂未接入消费端。

### 5. 文档更新（已完成）
- config.yaml.example：补充所有L1配置
- AGENT.md：更新核心配置参数表
- CHANGELOG.md：记录配置架构重构
