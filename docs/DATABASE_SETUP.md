# 数据库环境搭建指南

## 架构说明

本项目使用 **ParadeDB**（PostgreSQL 发行版）作为唯一数据库，一个镜像覆盖全部检索需求：

| 能力 | 实现 | 说明 |
|---|---|---|
| 向量检索 | pgvector | HNSW 索引，余弦距离 |
| BM25 全文检索 | pg_search | 底层 Tantivy，真正的 BM25 打分 |
| 中文分词 | pg_search 内置 jieba | 词典 + 统计模型，词级切分 |
| 定时任务 | pg_cron | 镜像已内置（本项目暂未使用，见下文） |
| 任务队列 | 数据表 + `FOR UPDATE SKIP LOCKED` | 不依赖扩展 |

### 为什么不用 zhparser / pg_textsearch

- zhparser 依赖 SCWS 词库，同样需要编译安装，并未比 ParadeDB 更省事
- PostgreSQL 内置的 `ts_rank` 是 TF-IDF 变体，缺少文档长度归一化，不是 BM25

### 为什么不用 pgmq

pgmq 需要编译安装且**不在 ParadeDB 镜像内**。以本项目的队列场景（文章入库任务，量级小），
用数据表配合 `FOR UPDATE SKIP LOCKED` 完全够用，可见性超时/重试/死信在应用层实现，
详见 `pkg/pgqueue`。

### 关于 pg_cron

镜像内置 pg_cron，但它**只能配置在 `postgres` 库上且不可更改**，跨库调度需
`cron.schedule_in_database()`。为避免引入部署期依赖，队列的超时任务回收改由
应用层 Go 定时器执行（`pkg/pgqueue.StartReclaimer`）。

---

## 一、本地开发环境（仅启动数据库）

应用在宿主机以二进制运行，数据库在容器内。适合日常开发与调试。

### 1. 启动 ParadeDB 容器

```bash
docker run -d \
  --name aiknow-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=aiknow \
  -p 5432:5432 \
  -v aiknow-pgdata:/var/lib/postgresql/data \
  paradedb/paradedb:0.25.6-pg17
```

> **务必钉死版本号。** pg_search 的索引 DDL 与查询语法在 0.25.0 发生过不兼容变更，
> 使用 `latest` 会在升级时静默破坏检索功能（见本文末"版本注意事项"）。

### 2. 初始化数据库

```bash
# 创建扩展（vector + pg_search，含可用性验证）
psql -h 127.0.0.1 -U postgres -d aiknow -f deploy/postgres/000_init.sql

# 建表（合并了所有迁移中的建表 DDL）
psql -h 127.0.0.1 -U postgres -d aiknow -f deploy/postgres/schema.sql
```

### 3. 配置环境变量

```bash
cp .env.example .env
```

编辑 `.env`，至少填写：

```bash
DATABASE_URL=postgres://postgres:password@127.0.0.1:5432/aiknow?sslmode=disable
SESSION_SECRET=至少32字符的随机串
ADMIN_PASSWORD_HASH=（由下方命令生成）
```

生成管理员密码哈希：

```bash
go run ./cmd/aiknow-server hashpwd 你的密码
```

开发期也可直接用明文密码（`ADMIN_PASSWORD=你的密码`），应用会自动哈希。

### 4. 启动应用

```bash
make run
# 或
go run ./cmd/aiknow-server serve
```

访问 `http://localhost:8080`，健康检查端点为 `/api/v1/health`。

### 重建数据库

```bash
docker exec -it aiknow-postgres psql -U postgres -d aiknow \
  -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"

psql -h 127.0.0.1 -U postgres -d aiknow -f deploy/postgres/000_init.sql
psql -h 127.0.0.1 -U postgres -d aiknow -f deploy/postgres/schema.sql
```

---

## 二、测试环境（Docker Compose 全栈）

一键拉起数据库与应用，适合集成测试、演示或生产预演。

### 1. 准备环境变量

```bash
cp .env.example .env
```

编辑 `.env`，至少填写以下三项：

```bash
POSTGRES_PASSWORD=数据库密码
SESSION_SECRET=至少32字符的随机串
ADMIN_PASSWORD_HASH=（由下方命令生成）
```

生成管理员密码哈希：

```bash
go run ./cmd/aiknow-server hashpwd 你的密码
# 或构建后：docker compose run --rm app /app/aiknow-server hashpwd 你的密码
```

> **`DATABASE_URL` 无需手动设置**：由 `docker-compose.yml` 按服务名 `postgres` 自动拼装。

### 2. 启动

```bash
docker compose up -d --build
# 或
make up
```

首次启动会：
1. 拉取 ParadeDB 0.25.6-pg17 镜像
2. 构建应用镜像（基于本仓库 `Dockerfile`）
3. 初始化数据库（`deploy/postgres/init/01-extensions.sql` 通过镜像的
   `docker-entrypoint-initdb.d` 机制执行，**仅在数据卷为空时运行一次**）
4. 应用启动后自动执行迁移（迁移脚本已通过 `go:embed` 打包进二进制）

**启动顺序有保障**：`app` 的 `depends_on` 使用 `service_healthy`，而 `postgres` 的
健康检查会实际探测 `vector` 与 `pg_search` 是否已创建（而非仅探测端口）。

### 3. 查看状态与日志

```bash
docker compose ps           # 或 make ps
docker compose logs -f app  # 或 make logs
```

访问 `http://localhost:8080`。

### 4. 常用操作

```bash
# 停止（保留数据）
docker compose down         # 或 make down

# 停止并删除数据卷（会清空数据库）
docker compose down -v

# 进入 psql
docker compose exec postgres psql -U postgres -d aiknow

# 查看应用版本
docker compose exec app /app/aiknow-server --version

# 重建（代码变更后）
docker compose up -d --build
```

### 编排说明

| 服务 | 说明 |
|---|---|
| `postgres` | ParadeDB 0.25.6-pg17，数据持久化在 `pgdata` 卷 |
| `app` | 应用镜像，由本仓库 `Dockerfile` 构建 |

几个要点：

- **扩展自动创建**：`deploy/postgres/init/01-extensions.sql` 调用 `000_init.sql`，
  仅在数据卷为空（首次初始化）时运行一次
- **数据库默认不暴露公网**：端口映射绑定 `127.0.0.1`，可用 `POSTGRES_BIND` 调整
- **应用端口**：默认 `0.0.0.0:8080`，可用 `APP_BIND` / `APP_PORT` 调整

### 数据卷已存在但扩展缺失时

初始化脚本不会重跑，需手动补建：

```bash
docker compose exec -T postgres psql -U postgres -d aiknow \
  < deploy/postgres/000_init.sql
```

---

## 三、版本注意事项

本项目基于 **pg_search 0.25.6** 编写。该版本的关键语法特征：

- 索引访问方法为 `paradedb`（0.25.0 起由 `bm25` 更名，`bm25` 保留为兼容别名）
- 分词器通过**类型转换**声明：`(content::pdb.jieba)`
- **不再使用** `WITH (text_fields='{...}')` 这种 JSON 配置形式（0.25.0 之前的写法）
- 打分函数为 `pdb.score(<key_field>)`，命名空间是 `pdb` 而非 `paradedb`
- 匹配算子：`|||` 任一词元命中，`&&&` 全部词元命中
- **每张表只允许一个 `paradedb` 索引**

升级前请先阅读 [ParadeDB 升级说明](https://github.com/paradedb/paradedb/releases)。

---

## 四、待验证事项

以下事项**需在真实 ParadeDB 实例上实机验证**（本地开发或测试环境均可）：

### 1. UUID 作 `key_field`

`deploy/postgres/schema.sql` 中两个 BM25 索引（`chunks_search_idx` / `articles_search_idx`）
使用 UUID 列作为 `key_field`。官方文档对 `key_field` 只提出三项要求（唯一约束、位于列首、
text 类型时不分词），UUID 均满足，但未明确背书该类型。

**若 `CREATE INDEX` 报类型错误**，需为表增加 bigint 代理键：

```sql
-- 以 chunks 为例
ALTER TABLE chunks ADD COLUMN idx_key BIGINT GENERATED ALWAYS AS IDENTITY;

-- 重建索引，将 key_field 改为 idx_key
DROP INDEX chunks_search_idx;
CREATE INDEX chunks_search_idx ON chunks
USING paradedb (idx_key, (content::pdb.jieba), deleted, article_id, chunk_index)
WITH (key_field = 'idx_key');

-- 代码中 pdb.score(id) 须改为 pdb.score(idx_key)
```

articles 同理。

### 2. jieba 分词的实际召回效果

验证中文查询的词级切分与召回质量，与原 zhparser 方案对比。

### 3. KeywordSearch 的加权 SQL

`internal/repository/search_repo.go` 中 `KeywordSearch` 方法使用 CTE 与 `|||` 算子，
尚未在真实实例上运行。重点验证：

- 标签/分类精确匹配的 UNION 分支
- 标题/摘要/正文 BM25 匹配分支
- 加权求和与排序

---

## 五、运维须知

### chunks.deleted 冗余列

`chunks` 表的 `deleted` 列是 `articles.deleted_at` 的冗余投影，由触发器
（`trg_sync_chunks_deleted`、`trg_inherit_chunk_deleted`）自动维护。

**存在的原因**：BM25 过滤条件必须能下推进 pg_search 索引。若改为 JOIN `articles`
再过滤，会先取回大量候选再筛除，导致 topK 与分页失真（返回结果不足 K 条）。

排查数据不一致时可用以下语句校验：

```sql
SELECT count(*) FROM chunks c JOIN articles a ON a.id = c.article_id
WHERE c.deleted <> (a.deleted_at IS NOT NULL);
-- 预期为 0
```

### 队列运维

```sql
-- 队列深度
SELECT count(*) FROM job_queue
WHERE status IN ('pending','processing') AND retry_count < max_retries;

-- 死信
SELECT id, retry_count, last_error, updated_at
FROM job_queue WHERE status = 'failed' ORDER BY updated_at DESC;

-- 重投死信
UPDATE job_queue SET status='pending', retry_count=0, visible_at=now()
WHERE id = <msg_id>;

-- 清理归档（应用层亦提供 PurgeArchive）
DELETE FROM job_archive WHERE archived_at < now() - interval '30 days';
```

### 索引重建

修改分词器配置后必须重建索引（`CREATE INDEX` 开销较大，建议在低峰期执行）：

```sql
REINDEX INDEX CONCURRENTLY chunks_search_idx;
REINDEX INDEX CONCURRENTLY articles_search_idx;
```

### 打分受死行影响

pg_search 的 BM25 打分会把未回收的死行计入统计。若发现排序异常：

```sql
VACUUM chunks;
VACUUM articles;
```

---

## 六、云托管的限制

pg_search 是自建专属扩展，阿里云 / 腾讯云等托管 RDS **不支持**（它们支持 zhparser）。
选用 ParadeDB 意味着数据库需自建。若未来计划迁移至托管 RDS，需重新评估检索方案。

此外 pg_search 采用 **AGPL-3.0** 许可。自用无影响；若涉及对外分发或 SaaS 交付，
建议提前进行许可合规确认。
