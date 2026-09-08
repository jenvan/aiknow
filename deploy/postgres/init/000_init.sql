-- ================================================================================
-- AiKnow 数据库基础设施初始化（仅扩展，不含建表）
-- ================================================================================
--
-- 本文件是扩展创建的唯一来源，同时服务两种用法：
--
--   1. Docker Compose：挂载到 /docker-entrypoint-initdb.d，容器首次初始化
--      （数据卷为空时）自动执行一次
--
--   2. 本地开发 / 已有数据库：手动执行
--        psql -h 127.0.0.1 -U postgres -d aiknow -f deploy/postgres/init/000_init.sql
--
-- 为什么扩展不放在项目迁移里：
--   迁移执行器会跳过一切扩展创建语句（见 internal/repository/migrations.go），
--   因此扩展只能由本文件创建。
--
-- 建表 DDL 不在此处：
--   表结构的唯一来源是 internal/repository/migrations/，通过
--   `aiknow-server migrate` 或服务启动时自动执行。此处不复制任何建表语句，
--   以免两处定义产生漂移。
--
-- 注意：shared_preload_libraries 需在 postgresql.conf 中设置并重启，
--       ParadeDB 镜像已预配置，自建环境需手动设置。
-- ================================================================================

-- ---- 扩展 ----

-- pgvector：向量存储与 HNSW 索引
CREATE EXTENSION IF NOT EXISTS vector;

-- pg_search：BM25 全文检索 + jieba 中文分词（底层 Tantivy）
-- 自 0.25.0 起依赖 pgvector 的 vector 类型，需先创建 vector
CREATE EXTENSION IF NOT EXISTS pg_search;

-- ---- 验证 ----
--
-- 以下校验失败会抛出异常。在 Docker 初始化场景下，这会使容器启动中止，
-- 从而在部署阶段立即暴露镜像或版本问题，而不是等到应用运行时才报错。

DO $$
DECLARE
    ext_count INT;
BEGIN
    SELECT count(*) INTO ext_count
    FROM pg_extension
    WHERE extname IN ('vector', 'pg_search');

    IF ext_count < 2 THEN
        RAISE EXCEPTION '扩展创建失败：期望 2 个（vector + pg_search），实际 %', ext_count;
    END IF;

    RAISE NOTICE '扩展已就绪：vector + pg_search';
END $$;

-- 验证 jieba 分词器可用（部分早期版本不含 jieba）
DO $$
DECLARE
    tokens TEXT[];
BEGIN
    -- 应返回词级切分结果，而非逐字切分
    SELECT '中文分词测试'::pdb.jieba::text[] INTO tokens;

    IF tokens IS NULL OR array_length(tokens, 1) IS NULL THEN
        RAISE EXCEPTION 'jieba 分词器不可用，请确认 pg_search 版本';
    END IF;

    RAISE NOTICE 'jieba 分词器可用，测试输入切分为 % 个词元', array_length(tokens, 1);
END $$;
