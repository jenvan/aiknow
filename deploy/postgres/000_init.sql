-- ================================================================================
-- AiKnow 数据库初始化脚本（基础设施）
-- ================================================================================
--
-- 用途：本地开发环境首次初始化
--
-- 使用方式：
--   psql -h 127.0.0.1 -U postgres -d aiknow -f deploy/postgres/000_init.sql
--
-- 说明：
--   1. 扩展必须在建表前创建（pg_search 0.25.0+ 依赖 vector 类型）
--   2. 项目迁移执行器会跳过扩展创建语句，因此扩展只能由此脚本或
--      docker-compose 的 init 脚本创建
--   3. shared_preload_libraries 需在 postgresql.conf 中设置并重启，
--      ParadeDB 镜像已预配置
-- ================================================================================

-- ---- 扩展 ----

-- pgvector：向量存储与 HNSW 索引
CREATE EXTENSION IF NOT EXISTS vector;

-- pg_search：BM25 全文检索 + jieba 中文分词（底层 Tantivy）
-- 自 0.25.0 起依赖 pgvector 的 vector 类型，需先创建 vector
CREATE EXTENSION IF NOT EXISTS pg_search;

-- 验证扩展已加载
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

    RAISE NOTICE '✓ 扩展已就绪：vector + pg_search';
END $$;

-- 验证 jieba 分词器可用
DO $$
DECLARE
    tokens TEXT[];
BEGIN
    -- 测试分词：应返回词级切分结果，而非逐字切分
    SELECT '中文分词测试'::pdb.jieba::text[] INTO tokens;

    IF array_length(tokens, 1) IS NULL OR array_length(tokens, 1) = 0 THEN
        RAISE EXCEPTION 'jieba 分词器不可用';
    END IF;

    RAISE NOTICE '✓ jieba 分词器可用，测试输入切分为 % 个词元', array_length(tokens, 1);
END $$;
