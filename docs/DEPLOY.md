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
> 使用 `latest` 会在升级时静默破坏检索功能（见第四章"版本注意事项"）。

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

> **`DATABASE_URL` 无需设置**：compose 会将其置空，应用改用上面这组 `POSTGRES_*`
> 分项拼装连接串，主机名与端口由编排固定为 `postgres:5432`。
> `.env` 中已有的、面向宿主机的 `DATABASE_URL` 可以保留，不会被带入容器。
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
- **端口**：宿主机与容器使用同一端口 —— 应用由 `PORT`（默认 8080）控制，
  数据库由 `POSTGRES_PORT`（默认 5432）控制。宿主机端口被占用时改这两项即可
- **监听地址不做配置**：容器端口默认发布在所有网卡上，访问控制交由网络层
  （安全组 / 防火墙 / 反向代理）负责，见第八章
- **应用以非 root 运行**：镜像内使用 uid/gid 10001

### 数据卷已存在但扩展缺失时

初始化脚本不会重跑，需手动补建：

```bash
docker compose exec -T postgres psql -U postgres -d aiknow \
  < docker-compose-initdb.sql
```

---

## 三、配置项参考

### 优先级

```
环境变量  >  .env  >  config.yaml  >  内置默认值
```

同一项在多处出现时取优先级最高者。启动日志会打印每一项的实际来源（不打印值），
排查"配置为何没生效"时先看这里。

配置项分两层：

- **L1 启动配置**：本章所列，进程启动时读取，改动需重启
- **L2 业务配置**：检索权重、分块大小、日志保留天数等，存于数据库 `settings` 表，
  由后台管理界面调整并热生效，不在本章范围

### 应用读取的配置项

直跑二进制与 compose 部署都生效。

| 变量 | 默认值 | 说明 |
|---|---|---|
| `DATABASE_URL` | 无 | 完整连接串。与下方分项**二选一**，填了它就忽略分项 |
| `POSTGRES_HOST` | `postgres` | 分项：主机名，默认值为 compose 服务名 |
| `POSTGRES_PORT` | `5432` | 分项：数据库端口（非宿主机发布端口） |
| `POSTGRES_USER` | `postgres` | 分项：用户名 |
| `POSTGRES_PASSWORD` | 无 | 分项：密码。走分项形态时必填 |
| `POSTGRES_DB` | `aiknow` | 分项：库名 |
| `POSTGRES_SSLMODE` | `disable` | 分项：SSL 模式 |
| `PORT` | `8080` | 应用监听端口 |
| `APP_ENV` | `development` | `development` \| `production` |
| `SESSION_SECRET` | 无（必填，≥32 字符） | 见下文"尚未接线的配置项" |
| `SESSION_LIFETIME` | `24h` | Session 有效期，兼作 Cookie MaxAge |
| `ADMIN_USERNAME` | `admin` | 管理员用户名 |
| `ADMIN_PASSWORD_HASH` | 无 | bcrypt 哈希，与下者至少提供一个 |
| `ADMIN_PASSWORD` | 无 | 明文密码，仅开发环境 |
| `LLM_PROVIDER` | `mock` | `qwen` \| `mock` |
| `EMBEDDING_PROVIDER` | `mock` | `baai` \| `mock` |
| `RERANKER_PROVIDER` | `mock` | `baai` \| `mock` |
| `QWEN_API_KEY` | 无 | `LLM_PROVIDER=qwen` 时必填 |
| `QWEN_BASE_URL` | 无 | Qwen 兼容模式端点 |
| `BAAI_API_KEY` | 无 | Embedding / Reranker 选 `baai` 时必填 |
| `BAAI_BASE_URL` | 无 | BAAI 服务端点 |
| `COLLECTOR_TIMEOUT` | `10s` | 采集 http 超时（渲染关闭时的兜底路径） |
| `COLLECTOR_MAX_REDIRECTS` | `3` | 采集最大重定向次数 |
| `COLLECTOR_MIN_CONTENT_LENGTH` | `200` | 正文最小字符数，低于此值判定采集失败 |
| `COLLECTOR_USER_AGENT` | 移动端 Safari UA | 采集 UA 基线；后台 `collector.user_agent` 可热覆盖 |
| `COLLECTOR_BROWSER_ENABLED` | `false` | 无头浏览器渲染总开关，须与 endpoint 同时满足才渲染 |
| `COLLECTOR_BROWSER_ENDPOINT` | 空 | browserless 端点，如 `http://10.0.0.5:3000`；空则渲染关闭 |
| `COLLECTOR_BROWSER_TOKEN` | 空 | browserless 访问令牌（`x-access-token`） |
| `COLLECTOR_BROWSER_TIMEOUT` | `30s` | 单页渲染硬超时（含导航、等待、截图）；服务 `WriteTimeout` 取此值 + 20s |
| `COS_SECRET_ID` / `COS_SECRET_KEY` | 空 | 腾讯云 API 密钥。建议只放 `.env`，勿写入 `config.yaml` |
| `COS_BUCKET` | 空 | 存储桶名（含 appid 后缀），如 `aiknow-1300000000` |
| `COS_REGION` | 空 | 地域，如 `ap-guangzhou` |
| `COS_ENDPOINT` | 空 | 自定义访问域名（CDN）；空则用 COS 默认域名 |
| `COS_PATH_PREFIX` | `screenshots` | 截图对象键前缀 |

> `COS_*` 任一必填项（密钥 / bucket / region）缺失时截图归档整体关闭，**不影响正文采集**。

`APP_ENV=production` 会改变两处行为：

1. 禁止使用明文 `ADMIN_PASSWORD`，必须提供 `ADMIN_PASSWORD_HASH`，否则启动失败
2. Session Cookie 启用 `Secure` 标记，仅通过 HTTPS 传输

> 本地通过 HTTP 访问时**不要**设为 `production` —— 浏览器不会回传带 `Secure`
> 标记的 Cookie，将无法登录。生产部署需在应用前放置终止 TLS 的反向代理。

### 端口与监听地址

**上表就是全部配置项** —— 没有额外的编排专用变量。

compose 下宿主机与容器使用同一端口，无需单独的映射变量：

| 服务 | 控制变量 | 默认 | 端口映射 |
|---|---|---|---|
| 应用 | `PORT` | `8080` | `8080:8080` |
| 数据库 | `POSTGRES_PORT` | `5432` | `5432:5432` |

宿主机端口被占用时改对应变量即可，两侧同步生效。

**`POSTGRES_PORT` 在两种部署形态下含义略有不同**，但都指向"postgres 可达的端口"：

- **compose**：宿主机发布端口，供 `psql`、`make db-init`、客户端工具直连。
  容器网络内固定 `5432`（由编排在 `environment` 中覆盖），因此改这个值
  **不影响 app 连接数据库**
- **直跑二进制**：数据库实际端口，直接用于拼装连接串

**监听地址不提供配置项**。容器端口默认发布在所有网卡上，访问控制交由网络层
（安全组 / 防火墙 / 反向代理）负责 —— 见第八章"生产部署补充建议"。

### `DATABASE_URL` 与 `POSTGRES_*` 的关系

**二选一，`DATABASE_URL` 优先**：填了它就忽略分项；为空时由 `POSTGRES_*` 拼装成
`postgres://user:password@host:port/dbname?sslmode=...`。

拼装时密码会做 URL 编码，因此密码中含 `@` `:` `/` `?` `#` 等字符无需手动转义。

两种部署形态各用一套，因此 `.env` 里两者可以并存：

| 部署方式 | 实际生效 | 说明 |
|---|---|---|
| 直跑二进制（本地开发 / 生产） | `DATABASE_URL` | 只需填这一项 |
| Docker Compose | `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | 主机名与端口由编排固定为 `postgres:5432` |

**compose 下 `.env` 里的 `DATABASE_URL` 会被编排显式置空**
（`docker-compose.yml` 的 `environment:` 段），原因是它通常指向 `127.0.0.1`，
而容器内的回环地址是容器自己，会连不上且报错难以定位。置空后应用改走分项拼装，
两者并存互不干扰。

直跑二进制时若想改用分项而非 `DATABASE_URL`，需额外设置 `POSTGRES_HOST`
（默认值 `postgres` 是 compose 服务名，本机应为 `127.0.0.1`）。

### 使用 config.yaml

`config.yaml` 采用嵌套结构，字段与环境变量一一对应，完整示例见
`config.yaml.example`：

```yaml
database:
  url: "postgres://postgres:password@127.0.0.1:5432/aiknow?sslmode=disable"
server:
  port: 8080
app:
  env: "development"
session:
  secret: "至少32个字符的随机串"
  lifetime: "24h"
admin:
  username: "admin"
  password_hash: ""
ai:
  llm_provider: "mock"
```

优先级低于 `.env` 与环境变量，适合放非敏感的默认值。凭据建议仍走环境变量或
`.env`，避免写入文件。

> 新增 YAML 字段需同时在 `internal/config/config.go` 的 `YAMLConfig` 结构与
> `managedKeys` 清单中登记，否则不会生效。`config_test.go` 中的
> `TestExampleYAMLMatchesParser` 会守住示例文件与解析器的一致性。

### 无头浏览器渲染与截图归档（可选）

URL 采集默认只走轻量 http；配置后改为**渲染优先**：每篇先请求 browserless 拿
「渲染后 HTML + 全页截图」，渲染失败才降级 http。适合采集 SPA 动态页与需要
截图归档的场景，静态站/纯内网可不部署。

1. **部署 browserless**（独立容器，不进主镜像）：

   ```bash
   cd deploy/browserless
   cp .env.example .env    # 填 TOKEN 等
   docker compose up -d
   ```

   详细说明与网络隔离要求见 `deploy/browserless/README.md`。**必须部署在访问不到
   敏感内网的网段（DMZ）**：Go 层 SSRF 只校验入口 URL，页面内 JS 自发的子请求
   拦不住。

2. **主服务开启渲染**：`COLLECTOR_BROWSER_ENABLED=true` +
   `COLLECTOR_BROWSER_ENDPOINT=http://<browserless>:3000` +
   `COLLECTOR_BROWSER_TOKEN=<token>`。三者缺一（或 enabled=false）即保持纯 http。

3. **配置 COS 归档截图**（可选）：填 `COS_SECRET_ID` / `COS_SECRET_KEY` /
   `COS_BUCKET`（含 appid 后缀）/ `COS_REGION`；`COS_ENDPOINT` 可填 CDN 域名，
   `COS_PATH_PREFIX` 默认 `screenshots`。截图在 worker 处理 URL 文章时上传并写回
   `articles.screenshot_url`；上传失败只记日志，不阻断文章处理。

> 渲染单页可达 20s+。服务 `WriteTimeout` 在渲染启用时自动取
> `COLLECTOR_BROWSER_TIMEOUT + 20s`，避免同步的采集 Preview 请求被掐断；关闭渲染
> 时维持 15s 基线。

### 尚未接线的配置项

`SESSION_SECRET` 目前**只被配置校验使用**（强制必填且检查长度 ≥32），代码尚未
实际消费它。Session ID 由密码学随机源生成并存于服务端，暂不需要签名密钥。
保留为必填是为了在引入 CSRF Token 或 Cookie 签名前就把它准备好，届时无需再调整
部署配置。

### 由代码固定、不可配置的项

| 项 | 当前值 | 位置 |
|---|---|---|
| 连接池上下限 | `MaxConns=20` / `MinConns=2` | `internal/repository/postgres.go` |
| 连接寿命 / 空闲超时 | 30 分钟 / 10 分钟 | 同上 |
| HTTP 读写 / 空闲超时 | 15s / 15s / 60s；渲染启用时写超时 = `COLLECTOR_BROWSER_TIMEOUT + 20s` | `cmd/aiknow-server/main.go` |
| 向量维度 | 1024（与 bge-large-zh-v1.5 绑定） | 迁移 SQL 与 Provider |
| 模型名 | `BAAI/bge-large-zh-v1.5`、`BAAI/bge-reranker-v2-m3` | `internal/service/provider/` |
| Session 存储 | 进程内存 | `internal/service/auth.go` |

> Session 存于进程内存，因此**重启即全部失效**，且多实例部署时无法共享。
> 需要水平扩容前须先将 Session 迁至 Redis 或数据库。

---

## 四、版本注意事项

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
2. **先备份**（见第六章"备份与恢复"）
3. 修改 `docker-compose.yml` 中的镜像 tag，以及本文档与 `AGENTS.md` 中的版本记录
4. `docker compose up -d`，观察 `docker compose logs postgres`
5. 若语法有变更，需同步修改 `internal/repository/migrations/006_paradedb_search.sql`
   与 `internal/repository/search_repo.go`，并新增迁移重建索引
6. 验证检索功能（中文查询、BM25 打分、关键词加权）

> 跨多个 minor 版本升级时，逐版本阅读 release notes，不要直接跳到最新。

---

## 五、待验证事项

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

## 六、运维须知

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

## 七、云托管的限制

pg_search 是自建专属扩展，阿里云 / 腾讯云等托管 RDS **不支持**（它们支持 zhparser）。
选用 ParadeDB 意味着数据库需自建。若未来计划迁移至托管 RDS，需重新评估检索方案。

此外 pg_search 采用 **AGPL-3.0** 许可。自用无影响；若涉及对外分发或 SaaS 交付，
建议提前进行许可合规确认。

---

## 八、生产部署补充建议

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
- **监控告警**：对第六章"关键观测指标"中的死信数量、队列深度、
  `failed` 文章数设置告警
