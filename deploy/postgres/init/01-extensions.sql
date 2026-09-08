-- ParadeDB 扩展初始化
--
-- 由 Postgres 官方镜像的 docker-entrypoint-initdb.d 机制执行，
-- 仅在数据目录为空（首次初始化）时运行一次。
--
-- 必须放在此处而非项目迁移中：迁移执行器会跳过一切扩展创建语句
-- （见 internal/repository/migrations.go），因此扩展无法由迁移自动创建。
--
-- 若数据卷已存在（非首次启动）而扩展缺失，需手动执行本文件内容：
--   docker compose exec -T postgres psql -U postgres -d aiknow < deploy/postgres/init/01-extensions.sql

-- pg_search 自 0.25.0 起依赖 pgvector 的 vector 类型，需先创建
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_search;
