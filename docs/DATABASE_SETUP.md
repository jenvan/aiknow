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

## 一、启动数据库

推荐使用 Docker Compose 一并拉起数据库与应用（**方式 A**）；
若只需数据库、应用在宿主机以二进制运行，用 **方式 B**。

### 方式 A：Docker Compose（推荐）

仓库根目录已提供 `docker-compose.yml` 与 `Dockerfile`。

```bash
# 1. 准备环境变量
cp .env.example .env

# 2. 至少填写以下三项（编辑 .env）
#    POSTGRES_PASSWORD  数据库密码
#    SESSION_SECRET     32 字符以上随机串
#    ADMIN_PASSWORD_HASH（或开发期用 ADMIN_PASSWORD）
#
#    生成管理员密码哈希：
#    go run ./cmd/aiknow-server hashpwd 你的密码

# 3. 启动
docker compose up -d --build      # 或 make up

# 4. 查看状态与日志
docker compose ps                 # 或 make ps
docker compose logs -f app        # 或 make logs
```

启动后访问 `http://localhost:8080`，健康检查端点为 `/api/v1/health`。

编排包含两个服务：

| 服务 | 说明 |
|---|---|
| `postgres` | ParadeDB 0.25.6-pg17，数据持久化在 `pgdata` 卷 |
| `app` | 应用镜像，由本仓库 `Dockerfile` 构建 |

几个要点：

- **扩展自动创建**：`deploy/postgres/init/01-extensions.sql` 通过镜像的
  `docker-entrypoint-initdb.d` 机制执行，**仅在数据卷为空（首次初始化）时运行一次**
- **启动顺序有保障**：`app` 的 `depends_on` 使用 `service_healthy`，而 `postgres`
  的健康检查会实际探测 `vector` 与 `pg_search` 是否已创建（而非仅探测端口）。
  这是必要的——应用启动即执行迁移，而迁移依赖这两个扩展
- **`DATABASE_URL` 无需手工设置**：由编排按服务名 `postgres` 自动拼装
- **数据库默认不暴露公网**：端口映射绑定 `127.0.0.1`，可用 `POSTGRES_BIND` 调整

常用操作：

```bash
docker compose down              # 停止，保留数据
docker compose down -v           # 停止并删除数据卷（会清空数据库）
docker compose exec postgres psql -U postgres -d aiknow    # 进入 psql
```

> **数据卷已存在但扩展缺失时**（例如卷创建于本次调整之前），初始化脚本不会重跑，
> 需手动补建：
>
> ```bash
> docker compose exec -T postgres psql -U postgres -d aiknow \
>   < deploy/postgres/init/01-extensions.sql
> ```

### 方式 B：仅启动数据库

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

此方式需按「二、创建扩展」手动创建扩展。

> **务必钉死版本号。** pg_search 的索引 DDL 与查询语法在 0.25.0 发生过不兼容变更
> （见下文"版本注意事项"），使用 `latest` 会在升级时静默破坏检索功能。

### 版本注意事项

本项目基于 **pg_search 0.25.6** 编写。该版本的关键语法特征：

- 索引访问方法为 `paradedb`（0.25.0 起由 `bm25` 更名，`bm25` 保留为兼容别名）
- 分词器通过**类型转换**声明：`(content::pdb.jieba)`
- **不再使用** `WITH (text_fields='{...}')` 这种 JSON 配置形式（0.25.0 之前的写法）
- 打分函数为 `pdb.score(<key_field>)`，命名空间是 `pdb` 而非 `paradedb`
- 匹配算子：`|||` 任一词元命中，`&&&` 全部词元命中
- **每张表只允许一个 `paradedb` 索引**

升级前请先阅读 [ParadeDB 升级说明](https://github.com/paradedb/paradedb/releases)。

---

## 二、创建扩展

> **使用方式 A（Docker Compose）可跳过本节** —— 扩展由初始化脚本自动创建。
> 本节适用于方式 B 及自建数据库环境。

> **必须手动执行。** 项目的迁移执行器会跳过一切扩展创建语句
> （见 `internal/repository/migrations.go`），因此扩展无法由迁移自动创建。

```bash
psql -h 127.0.0.1 -U postgres -d aiknow <<'SQL'
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_search;
SQL
```

`pg_search` 自 0.25.0 起依赖 pgvector 的 `vector` 类型，需先创建 `vector`；
若使用 `CREATE EXTENSION pg_search CASCADE;` 则会自动带出 `vector`。

自建（非镜像）环境还需在 `postgresql.conf` 中设置并重启：

```ini
shared_preload_libraries = 'pg_search'
```

### 验证

```bash
psql -h 127.0.0.1 -U postgres -d aiknow -c '\dx'
```

应能看到 `vector` 与 `pg_search`。再验证 jieba 分词是否可用：

```sql
SELECT '你好世界，全文检索'::pdb.jieba::text[];
```

预期返回词级切分结果（而非逐字切分）。若报错说明该版本未包含 jieba 分词器，
请核对镜像版本。

---

## 三、执行迁移

迁移在**服务启动时自动执行**（见 `cmd/aiknow-server/main.go`），无需单独命令。
迁移脚本已通过 `go:embed` 打包进二进制，容器内也无需挂载。

迁移文件位于 `internal/repository/migrations/`（`migrations/` 为软链接）：

| 文件 | 内容 |
|---|---|
| `001_initial_schema.sql` | 基础表结构 |
| `002_vector.sql` | pgvector HNSW 索引 |
| `003_pg_textsearch_bm25.sql` | **已废弃**（原 zhparser 方案，保留为空操作） |
| `004_pgmq.sql` | **已废弃**（原 pgmq 方案，保留为空操作） |
| `005_settings_apikeys_mcplogs.sql` | 配置、API Key、MCP 日志 |
| `006_paradedb_search.sql` | pg_search BM25 索引 + `chunks.deleted` 冗余列 |
| `007_job_queue.sql` | 任务队列表 |

> 003 / 004 被降级为空操作而非原地改写：已有开发库中它们已记录为"已应用"，
> 原地改写不会重跑，新逻辑必须放在新版本号里。

### 待验证事项

`006_paradedb_search.sql` 使用 UUID 列作为 `key_field`。官方文档对 `key_field`
只提出三项要求（唯一约束、位于列首、text 类型时不分词），UUID 均满足，但未明确
背书该类型。**若 `CREATE INDEX` 报类型错误**，需为 `chunks` 增加 bigint 代理键：

```sql
ALTER TABLE chunks ADD COLUMN idx_key BIGINT GENERATED ALWAYS AS IDENTITY;
-- 并将索引与 pdb.score() 的参数改为 idx_key
```

---

## 四、环境变量

由 `.env` 提供（`cp .env.example .env`）。`.env` 已在 `.gitignore` 中，不得提交。

**方式 A（Docker Compose）**：`DATABASE_URL` 由编排自动拼装，**不要**手动设置；
需填写的是数据库账号密码：

```bash
POSTGRES_PASSWORD=你的数据库密码
SESSION_SECRET=至少32字符的随机串
ADMIN_PASSWORD_HASH=（由 hashpwd 生成）
```

**方式 B（应用在宿主机运行）**：需显式指定连接串：

```bash
DATABASE_URL=postgres://postgres:password@127.0.0.1:5432/aiknow?sslmode=disable
```

生成管理员密码哈希：

```bash
go run ./cmd/aiknow-server hashpwd 你的密码
# 或容器内：docker compose exec app /app/aiknow-server hashpwd 你的密码
```

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
```

### 打分受死行影响

pg_search 的 BM25 打分会把未回收的死行计入统计。若发现排序异常：

```sql
VACUUM chunks;
```

---

## 六、云托管的限制

pg_search 是自建专属扩展，阿里云 / 腾讯云等托管 RDS **不支持**（它们支持 zhparser）。
选用 ParadeDB 意味着数据库需自建。若未来计划迁移至托管 RDS，需重新评估检索方案。

此外 pg_search 采用 **AGPL-3.0** 许可。自用无影响；若涉及对外分发或 SaaS 交付，
建议提前进行许可合规确认。
