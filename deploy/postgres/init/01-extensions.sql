-- ParadeDB 扩展初始化（Docker Compose 自动执行）
--
-- 由 Postgres 官方镜像的 docker-entrypoint-initdb.d 机制执行，
-- 仅在数据目录为空（首次初始化）时运行一次。
--
-- 若数据卷已存在（非首次启动）而扩展缺失，需手动执行：
--   docker compose exec -T postgres psql -U postgres -d aiknow \
--     < deploy/postgres/000_init.sql

\ir ../000_init.sql
