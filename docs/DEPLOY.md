# 部署与运维指南

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

### 初始化职责划分

| 内容 | 唯一来源 | 执行方式 |
|---|---|---|
| 扩展（`vector` / `pg_search`） | `docker-compose-initdb.sql` | compose 首次启动自动执行，或 `make db-init` |
| 表结构、索引、触发器 | `internal/repository/migrations/` | 服务启动时自动执行，或 `make migrate-up` |

扩展之所以不能并入迁移：迁移执行器会跳过一切扩展创建语句
（见 `internal/repository/migrations.go`），这是硬约束。除此之外不存在第二份 DDL。

> `docker-compose-initdb.sql` 虽以 `docker-compose` 开头，但**本地开发同样需要执行**，
> 它是扩展创建的唯一来源。

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
> 使用 `latest` 会在升级时静默破坏检索功能（见第三章"版本注意事项"）。

### 2. 创建扩展

扩展只需创建一次。表结构不在此步，见第 4 步。

```bash
psql -h 127.0.0.1 -U postgres -d aiknow -f docker-compose-initdb.sql
# 或（读取 .env 中的 DATABASE_URL）
make db-init
```

脚本会创建 `vector` 与 `pg_search`，并验证 jieba 分词器可用。

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

### 4. 建表（执行迁移）

```bash
make migrate-up
# 或
go run ./cmd/aiknow-server migrate
```

表结构的**唯一来源**是 `internal/repository/migrations/`（已通过 `go:embed` 打包进
二进制）。开发期生产环境执行的是同一套迁移，不存在第二份 DDL 定义。

这一步可跳过 —— `serve` 启动时会自动执行迁移。单独提供 `migrate` 子命令是为了
便于开发期初始化与重建，以及在 CI 中先建表再跑集成测试。

### 5. 启动应用

```bash
make run
# 或
go run ./cmd/aiknow-server serve
```

访问 `http://localhost:8080`，健康检查端点为 `/api/v1/health`。

### 重建数据库

清空 `public` schema 后重新创建扩展与表结构：

```bash
make db-reset
```

等价于：

```bash
psql "$DATABASE_URL" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
psql "$DATABASE_URL" -f docker-compose-initdb.sql
go run ./cmd/aiknow-server migrate
```

> `DROP SCHEMA public CASCADE` 会删除该 schema 下的全部数据，仅用于开发环境。

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
>
> `.env` 从 compose 的 project directory（即项目根）加载，因此需放在仓库根目录。

### 2. 启动

```bash
docker compose up -d --build
# 或
make up
```

首次启动会：
1. 拉取 ParadeDB 0.25.6-pg17 镜像
2. 构建应用镜像（基于本仓库 `Dockerfile`）
3. 创建扩展（`docker-compose-initdb.sql` 挂载到 `/docker-entrypoint-initdb.d/`，
   **仅在数据卷为空时运行一次**）
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

# 仅重启应用（不动数据库）
docker compose restart app
```

### 编排说明

| 服务 | 说明 |
|---|---|
| `postgres` | ParadeDB 0.25.6-pg17，数据持久化在 `pgdata` 卷 |
| `app` | 应用镜像，由本仓库 `Dockerfile` 构建 |

几个要点：

- **扩展自动创建**：`docker-compose-initdb.sql` 挂载为单文件，由镜像的
  `docker-entrypoint-initdb.d` 机制执行，仅在数据卷为空（首次初始化）时运行一次
- **表结构由迁移创建**：应用启动时自动执行 `internal/repository/migrations/`，
  编排中无需挂载任何建表 SQL
- **数据库默认不暴露公网**：端口映射绑定 `127.0.0.1`，可用 `POSTGRES_BIND` 调整
- **应用端口**：默认 `0.0.0.0:8080`，可用 `APP_BIND` / `APP_PORT` 调整
- **应用以非 root 运行**：镜像内使用 uid/gid 10001

### 数据卷已存在但扩展缺失时

初始化脚本不会重跑，需手动补建：

```bash
docker compose exec -T postgres psql -U postgres -d aiknow \
  < docker-compose-initdb.sql
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

### 升级 ParadeDB 的步骤

1. 阅读目标版本的 release notes，确认索引 DDL 与查询语法有无变更
2. **先备份**（见第五章"备份与恢复"）
3. 修改 `docker-compose.yml` 中的镜像 tag，以及本文档与 `AGENT.md` 中的版本记录
4. `docker compose up -d`，观察 `docker compose logs postgres`
5. 若语法有变更，需同步修改 `internal/repository/migrations/006_paradedb_search.sql`
   与 `internal/repository/search_repo.go`，并新增迁移重建索引
6. 验证检索功能（中文查询、BM25 打分、关键词加权）

> 跨多个 minor 版本升级时，逐版本阅读 release notes，不要直接跳到最新。

---

## 四、待验证事项

以下事项**需在真实 ParadeDB 实例上实机验证**（本地开发或测试环境均可）：

### 1. UUID 作 `key_field`

`internal/repository/migrations/006_paradedb_search.sql` 中两个 BM25 索引
（`chunks_search_idx` / `articles_search_idx`）使用 UUID 列作为 `key_field`。
官方文档对 `key_field` 只提出三项要求（唯一约束、位于列首、text 类型时不分词），
UUID 均满足，但未明确背书该类型。

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

```sql
SELECT '你的测试查询'::pdb.jieba::text[];
```

### 3. KeywordSearch 的加权 SQL

`internal/repository/search_repo.go` 中 `KeywordSearch` 方法使用 CTE 与 `|||` 算子，
尚未在真实实例上运行。重点验证：

- 标签/分类精确匹配的 UNION 分支
- 标题/摘要/正文 BM25 匹配分支
- 加权求和与排序

---

## 五、运维须知

### 健康检查与监控

应用健康检查端点：

```bash
curl http://localhost:8080/api/v1/health
# {"status":"ok","version":"..."}
```

容器健康状态（`Dockerfile` 内置 `HEALTHCHECK`，30s 间隔）：

```bash
docker compose ps           # STATUS 列显示 healthy / unhealthy
docker inspect --format '{{.State.Health.Status}}' aiknow-app
```

关键观测指标：

```sql
-- 文章处理状态分布（failed 堆积说明 AI 流水线有问题）
SELECT processing_status, count(*) FROM articles
WHERE deleted_at IS NULL GROUP BY processing_status;

-- 队列深度（持续增长说明消费能力不足）
SELECT count(*) FROM job_queue
WHERE status IN ('pending','processing') AND retry_count < max_retries;

-- 死信数量（> 0 需人工介入）
SELECT count(*) FROM job_queue WHERE status = 'failed';

-- 最早未完成任务的等待时长
SELECT now() - min(created_at) FROM job_queue
WHERE status IN ('pending','processing') AND retry_count < max_retries;

-- 各表体积
SELECT relname, pg_size_pretty(pg_total_relation_size(relid)) AS size
FROM pg_catalog.pg_statio_user_tables ORDER BY pg_total_relation_size(relid) DESC;
```

### 备份与恢复

**备份**（compose 环境）：

```bash
docker compose exec -T postgres pg_dump -U postgres -d aiknow -Fc \
  > backup-$(date +%Y%m%d-%H%M%S).dump
```

`-Fc`（custom 格式）支持并行恢复与选择性恢复，优于纯文本。

**恢复**：

```bash
# 恢复前必须先创建扩展 —— pg_dump 的转储中包含 CREATE INDEX 语句，
# 而 paradedb / hnsw 索引依赖 pg_search 与 vector 扩展
docker compose exec -T postgres psql -U postgres -d aiknow \
  < docker-compose-initdb.sql

docker compose exec -T postgres pg_restore -U postgres -d aiknow \
  --clean --if-exists < backup-20260908-120000.dump
```

**仅备份数据、不含结构**（表结构由迁移管理，通常这样更干净）：

```bash
docker compose exec -T postgres pg_dump -U postgres -d aiknow \
  --data-only -Fc > data-only.dump
```

恢复时先 `make migrate-up` 建好结构，再 `pg_restore --data-only`。

> 备份文件包含全部文章内容，属敏感数据，注意存储位置与访问权限。

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

若不为 0（例如曾绕过触发器批量改数据），手动修复：

```sql
UPDATE chunks c SET deleted = (a.deleted_at IS NOT NULL)
FROM articles a
WHERE c.article_id = a.id AND c.deleted <> (a.deleted_at IS NOT NULL);
```

### 队列运维

```sql
-- 队列深度
SELECT count(*) FROM job_queue
WHERE status IN ('pending','processing') AND retry_count < max_retries;

-- 死信
SELECT id, retry_count, last_error, updated_at
FROM job_queue WHERE status = 'failed' ORDER BY updated_at DESC;

-- 重投单条死信
UPDATE job_queue SET status='pending', retry_count=0, visible_at=now()
WHERE id = <msg_id>;

-- 批量重投（确认根因已修复后再执行）
UPDATE job_queue SET status='pending', retry_count=0, visible_at=now()
WHERE status='failed';

-- 清理归档（应用层亦提供 PurgeArchive）
DELETE FROM job_archive WHERE archived_at < now() - interval '30 days';
```

租约机制：消费者领取任务后须在租约（默认 5 分钟）内确认，否则任务由
`StartReclaimer` 回收重投。若任务处理耗时可能超过 5 分钟，需调大
`pgqueue.Config.LeaseDuration`，否则会出现重复处理。

### 索引维护

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

日志表增长较快，定期清理后建议 `VACUUM`：

```sql
DELETE FROM mcp_request_logs WHERE created_at < now() - interval '30 days';
VACUUM mcp_request_logs;
```

保留天数由 L2 配置 `mcp_log.retention_days` 控制（后台可调）。

### 凭据轮换

- **`SESSION_SECRET`**：修改后所有管理端会话失效，需重新登录
- **`ADMIN_PASSWORD_HASH`**：用 `aiknow-server hashpwd` 重新生成后替换
- **API Key**：在管理后台吊销并重新签发；`api_keys` 表只存 SHA-256 哈希，
  原文无法找回
- **`POSTGRES_PASSWORD`**：需同时修改数据库用户密码与 `.env`，然后重启应用

```sql
ALTER USER postgres WITH PASSWORD '新密码';
```

### 故障排查

| 症状 | 排查方向 |
|---|---|
| 启动报 `type "pdb.jieba" does not exist` | 扩展未创建，执行 `make db-init`；或镜像版本不含 jieba |
| 启动报 `relation "..." does not exist` | 迁移未执行成功，查看启动日志中"数据库迁移完成"是否出现 |
| compose 中 app 一直 `unhealthy` | `docker compose logs app`；确认 8080 端口未与宿主机冲突 |
| postgres 一直 `starting` 不转 `healthy` | 健康检查在探测扩展，`docker compose logs postgres` 看 init 脚本是否失败 |
| 检索无结果但数据存在 | 确认 `chunks.deleted` 是否被误置为 true；确认 embedding 是否已生成（`indexed_at`） |
| 检索结果排序异常 | 执行 `VACUUM`；确认 `search.*` 权重配置 |
| 队列堆积不消费 | 查看 `job_queue.last_error`；确认 Worker 已启动（启动日志）；查死信数量 |
| 文章长期停在 `processing` | AI 流水线中断且租约未到期；等待回收或手动重置状态 |

查看应用日志：

```bash
docker compose logs -f app          # compose 环境
docker compose logs --tail=200 app  # 最近 200 行
```

---

## 六、云托管的限制

pg_search 是自建专属扩展，阿里云 / 腾讯云等托管 RDS **不支持**（它们支持 zhparser）。
选用 ParadeDB 意味着数据库需自建。若未来计划迁移至托管 RDS，需重新评估检索方案。

此外 pg_search 采用 **AGPL-3.0** 许可。自用无影响；若涉及对外分发或 SaaS 交付，
建议提前进行许可合规确认。

---

## 七、生产部署补充建议

当前编排面向测试与演示，用于生产还需补充：

- **反向代理与 TLS**：应用未内置 HTTPS，需前置 Nginx / Caddy 终止 TLS
- **不暴露数据库端口**：生产环境建议移除 `postgres` 的 `ports` 映射，
  仅通过 compose 内网访问
- **凭据管理**：`.env` 不进版本库（已在 `.gitignore`）；生产建议使用
  密钥管理服务而非明文文件
- **备份自动化**：将上述 `pg_dump` 命令接入 cron 或编排平台的定时任务，
  并验证恢复流程可用
- **日志收集**：容器日志默认写入本地，生产建议配置 log driver 输出到集中式日志
- **资源限制**：为容器设置 `deploy.resources.limits`，避免单容器耗尽宿主机资源
- **监控告警**：对第五章"关键观测指标"中的死信数量、队列深度、
  `failed` 文章数设置告警
