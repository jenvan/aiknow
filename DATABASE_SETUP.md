# PostgreSQL 扩展安装指南

## 测试服务器环境
- 主机：192.168.1.234
- 端口：5432
- 数据库：aiknow

## 方案一：使用 ParadeDB 镜像（最推荐）

ParadeDB 是 pg_textsearch 的现代替代品，内置所有需要的扩展：

```bash
# 拉取镜像
docker pull paradedb/paradedb:pg17

# 运行容器
docker run -d \
  --name aiknow-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=aiknow \
  -p 5432:5432 \
  paradedb/paradedb:pg17
```

ParadeDB 内置扩展：
- ✅ pgvector（向量搜索）
- ✅ pg_bm25（BM25 全文搜索，替代 pg_textsearch）
- ✅ pg_jieba（中文分词，替代 zhparser）
- ✅ pgmq（消息队列）

## 方案二：使用 TimescaleDB 镜像

```bash
# 拉取镜像
docker pull timescale/timescaledb:latest-pg17

# 运行容器
docker run -d \
  --name aiknow-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=aiknow \
  -p 5432:5432 \
  timescale/timescaledb:latest-pg17
```

## 方案三：使用官方镜像手动安装

```bash
# 拉取官方 PostgreSQL 17 镜像
docker pull postgres:17

# 运行容器
docker run -d \
  --name aiknow-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=aiknow \
  -p 5432:5432 \
  postgres:17

# 进入容器安装扩展
docker exec -it aiknow-postgres bash

# 在容器内执行：
apt-get update && apt-get install -y git make gcc postgresql-server-dev-17

# 安装 pgvector
git clone https://github.com/pgvector/pgvector.git
cd pgvector
make
make install

# 创建扩展
psql -U postgres -d aiknow -c "CREATE EXTENSION vector;"
psql -U postgres -d aiknow -c "CREATE EXTENSION pgmq;"

# 安装 pg_textsearch（TimescaleDB 版本）
# 注意：需要编译安装，或使用 ParadeDB 替代
```

## 验证扩展

```bash
# 连接到数据库
psql -h 192.168.1.234 -U postgres -d aiknow

# 检查扩展
\dx

# 应该看到：
# vector
# pg_textsearch 或 pg_bm25
# pgmq
```

## 环境变量配置

在 `.env` 文件中配置：

```bash
DATABASE_URL=postgres://postgres:password@192.168.1.234:5432/aiknow?sslmode=disable
```

## 迁移执行

启动服务后，迁移会自动执行。如果遇到扩展缺失错误，检查扩展是否正确安装。
