# PostgreSQL 扩展安装指南

## 测试服务器环境
- 主机：192.168.1.234
- 端口：5432
- 数据库：aiknow

## 方案一：使用 pre-installed 镜像（推荐）

### 1. 使用 pgvector + pg_textsearch 官方镜像

```bash
# 拉取带 pgvector 的 PostgreSQL 镜像
docker pull ankane/pgvector:pg17

# 运行容器
docker run -d \
  --name aiknow-postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=aiknow \
  -p 5432:5432 \
  ankane/pgvector:pg17
```

### 2. 使用 TimescaleDB 镜像（含 pg_textsearch）

```bash
# TimescaleDB 镜像内置 pg_textsearch
docker pull timescale/timescaledb:latest-pg17

# 运行容器
docker run -d \
  --name aiknow-postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=aiknow \
  -p 5432:5432 \
  timescale/timescaledb:latest-pg17
```

### 3. 使用官方镜像手动安装扩展

```bash
# 拉取 PostgreSQL 17 镜像
docker pull postgres:17

# 运行容器
docker run -d \
  --name aiknow-postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=aiknow \
  -p 5432:5432 \
  postgres:17

# 进入容器安装扩展
docker exec -it aiknow-postgres bash

# 在容器内执行
apt-get update && apt-get install -y postgresql-contrib postgresql-server-dev-17

# 安装 pgvector
git clone https://github.com/pgvector/pgvector.git
cd pgvector
make
make install

# 安装 pg_textsearch（TimescaleDB 版本）
# 注意：需要编译安装，或使用 ParadeDB 替代

# 创建扩展
psql -U postgres -d aiknow -c "CREATE EXTENSION vector;"
psql -U postgres -d aiknow -c "CREATE EXTENSION pg_textsearch;"
psql -U postgres -d aiknow -c "CREATE EXTENSION pgmq;"
```

## 方案二：在现有 PostgreSQL 上安装扩展

如果您的测试服务器已有 PostgreSQL 17，可以执行：

```sql
-- 1. 安装 pgvector
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. 安装 pg_textsearch（TimescaleDB 官方扩展）
-- 需要先添加 TimescaleDB 源
CREATE EXTENSION IF NOT EXISTS pg_textsearch;

-- 3. 安装 zhparser（中文分词）
-- 需要编译安装
-- 下载地址：https://github.com/amutu/zhparser

-- 4. 安装 PGMQ
CREATE EXTENSION IF NOT EXISTS pgmq;
```

## 方案三：使用 ParadeDB 替代方案（推荐）

ParadeDB 是 pg_textsearch 的现代替代品，提供更强大的全文搜索：

```bash
# 使用 ParadeDB 镜像
docker pull paradedb/paradedb:pg17

docker run -d \
  --name aiknow-postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=aiknow \
  -p 5432:5432 \
  paradedb/paradedb:pg17
```

ParadeDB 内置：
- pg_bm25（BM25 全文搜索）
- pgvector（向量搜索）
- pg_jieba（中文分词）

## 环境变量配置

根据您使用的方案，修改 `.env` 文件中的 `DATABASE_URL`：

```bash
# 方案一/二：本地 PostgreSQL
DATABASE_URL=postgres://postgres:password@192.168.1.234:5432/aiknow?sslmode=disable

# 方案三：ParadeDB
DATABASE_URL=postgres://postgres:password@192.168.1.234:5432/aiknow?sslmode=disable
```

## 验证扩展安装

```bash
# 连接到数据库
psql -h 192.168.1.234 -U postgres -d aiknow

# 检查扩展
\dx

# 应该看到以下扩展：
# vector
# pg_textsearch 或 pg_bm25
# pgmq
```

## 迁移执行

启动服务后，迁移会自动执行。如果遇到扩展安装失败，可以在迁移文件中注释掉相关语句。
