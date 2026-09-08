-- ================================================================================
-- AiKnow 数据库结构定义（完整 DDL）
-- ================================================================================
--
-- 用途：本地开发环境建表（需先执行 000_init.sql 创建扩展）
--
-- 使用方式：
--   psql -h 127.0.0.1 -U postgres -d aiknow -f deploy/postgres/schema.sql
--
-- 说明：
--   1. 本文件合并了项目迁移中的所有建表 DDL（001/005/007 + 002/006 的索引部分）
--   2. 生产环境仍使用 internal/repository/migrations/ 中的迁移文件（已通过
--      go:embed 打包进二进制），本文件仅用于开发期快速重建数据库
--   3. chunks.deleted 列与触发器用于保持与 articles.deleted_at 同步，使 BM25
--      过滤条件可下推进索引
-- ================================================================================

-- ============================================================
-- 核心业务表
-- ============================================================

-- 分类表（需先创建，articles 引用它）
CREATE TABLE IF NOT EXISTS categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 文章表
CREATE TABLE IF NOT EXISTS articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(500) NOT NULL,
    content TEXT NOT NULL,
    source_type VARCHAR(20) NOT NULL DEFAULT 'manual', -- 'manual' | 'url'
    source_url TEXT,
    content_hash VARCHAR(64),
    processing_status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending | processing | completed | failed
    summary TEXT,
    category_id UUID REFERENCES categories(id),
    published BOOLEAN NOT NULL DEFAULT true,
    view_count INTEGER NOT NULL DEFAULT 0,
    embedding_model VARCHAR(100),
    embedding_dim INTEGER,
    indexed_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 标签表
CREATE TABLE IF NOT EXISTS tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 文章-标签关联表
CREATE TABLE IF NOT EXISTS article_tags (
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (article_id, tag_id)
);

-- Chunk 表（文章分块，用于向量检索与 BM25）
CREATE TABLE IF NOT EXISTS chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    word_count INTEGER NOT NULL,
    embedding VECTOR(1024),
    -- deleted 冗余列：与 articles.deleted_at 同步，使 BM25 过滤可下推进索引
    deleted BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(article_id, chunk_index)
);

-- ============================================================
-- 系统配置与管理
-- ============================================================

-- 系统配置表（L2 热配置）
CREATE TABLE IF NOT EXISTS settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    "key" VARCHAR(100) NOT NULL UNIQUE,
    value JSONB NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 配置变更历史
CREATE TABLE IF NOT EXISTS setting_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    setting_id UUID REFERENCES settings(id) ON DELETE CASCADE,
    changed_by UUID,
    old_value JSONB,
    new_value JSONB NOT NULL,
    change_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- API Key 表
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash VARCHAR(64) NOT NULL UNIQUE,
    key_prefix VARCHAR(10) NOT NULL,
    description TEXT NOT NULL,
    scope VARCHAR(20) NOT NULL DEFAULT 'readonly', -- 'readonly' | 'readwrite'
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- MCP 请求日志
CREATE TABLE IF NOT EXISTS mcp_request_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    method VARCHAR(100) NOT NULL,
    params JSONB,
    result JSONB,
    error TEXT,
    duration_ms INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 任务队列（替代 PGMQ 扩展）
-- ============================================================

-- 任务队列表
CREATE TABLE IF NOT EXISTS job_queue (
    id BIGSERIAL PRIMARY KEY,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending | processing | completed | failed
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    visible_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 任务归档表（completed / failed 任务移入此处）
CREATE TABLE IF NOT EXISTS job_archive (
    id BIGINT PRIMARY KEY,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL,
    retry_count INTEGER NOT NULL,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 索引
-- ============================================================

-- ---- 核心业务表索引 ----
CREATE INDEX IF NOT EXISTS idx_articles_category_id ON articles(category_id);
CREATE INDEX IF NOT EXISTS idx_articles_processing_status ON articles(processing_status);
CREATE INDEX IF NOT EXISTS idx_articles_created_at ON articles(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_deleted_at ON articles(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_articles_content_hash ON articles(content_hash) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_article_tags_article_id ON article_tags(article_id);
CREATE INDEX IF NOT EXISTS idx_article_tags_tag_id ON article_tags(tag_id);
CREATE INDEX IF NOT EXISTS idx_chunks_article_id ON chunks(article_id);
CREATE INDEX IF NOT EXISTS idx_chunks_word_count ON chunks(word_count);

-- 标签与分类的精确匹配索引（关键词检索用，大小写不敏感）
CREATE INDEX IF NOT EXISTS idx_tags_name_lower ON tags(lower(name));
CREATE INDEX IF NOT EXISTS idx_categories_name_lower ON categories(lower(name));

-- ---- pgvector HNSW 索引 ----
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw ON chunks
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- ---- pg_search BM25 索引 ----

-- chunks BM25 索引（混合检索 BM25 分支）
-- 语法说明（pg_search 0.25.x）：
--   - 访问方法为 paradedb（0.25.0 起由 bm25 更名，bm25 保留为兼容别名）
--   - 分词器通过类型转换声明：(content::pdb.jieba)
--   - key_field 须唯一、位于列表首位；每表只允许一个 paradedb 索引
--   - deleted / article_id 作为可过滤字段一并入索引，使过滤条件可下推
--
-- 待验证：官方文档未明确 uuid 可否作 key_field（三项要求 uuid 均满足，
-- 但无明文背书）。若 CREATE INDEX 报类型错误，则需为 chunks 增加
-- bigint 代理键（GENERATED ALWAYS AS IDENTITY）并改用该列作 key_field。
CREATE INDEX IF NOT EXISTS chunks_search_idx ON chunks
USING paradedb (id, (content::pdb.jieba), deleted, article_id, chunk_index)
WITH (key_field = 'id');

-- articles BM25 索引（关键词加权检索）
CREATE INDEX IF NOT EXISTS articles_search_idx ON articles
USING paradedb (
    id,
    (title::pdb.jieba),
    (summary::pdb.jieba),
    (content::pdb.jieba),
    published,
    category_id,
    processing_status,
    created_at
)
WITH (key_field = 'id');

-- ---- 任务队列索引 ----
-- 出队索引（按可见时间与创建时间排序，状态与重试次数过滤）
CREATE INDEX IF NOT EXISTS idx_job_queue_dequeue ON job_queue(visible_at, created_at)
WHERE status IN ('pending', 'processing') AND retry_count < max_retries;

-- 死信查询索引
CREATE INDEX IF NOT EXISTS idx_job_queue_failed ON job_queue(updated_at DESC)
WHERE status = 'failed';

-- 归档清理索引
CREATE INDEX IF NOT EXISTS idx_job_archive_archived_at ON job_archive(archived_at);

-- ---- 其他查询索引 ----
CREATE INDEX IF NOT EXISTS idx_mcp_request_logs_created_at ON mcp_request_logs(created_at DESC);

-- ============================================================
-- 触发器与函数
-- ============================================================

-- ---- chunks.deleted 同步触发器 ----

-- articles 软删除/恢复时，级联更新 chunks.deleted
CREATE OR REPLACE FUNCTION sync_chunks_deleted() RETURNS TRIGGER AS $$
BEGIN
    IF (OLD.deleted_at IS NULL) <> (NEW.deleted_at IS NULL) THEN
        UPDATE chunks
        SET deleted = (NEW.deleted_at IS NOT NULL)
        WHERE article_id = NEW.id
          AND deleted <> (NEW.deleted_at IS NOT NULL);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_chunks_deleted ON articles;
CREATE TRIGGER trg_sync_chunks_deleted
    AFTER UPDATE OF deleted_at ON articles
    FOR EACH ROW
    EXECUTE FUNCTION sync_chunks_deleted();

-- 新插入的 chunk 继承所属文章的删除状态（避免为已删文章补建 chunk 时漏标）
CREATE OR REPLACE FUNCTION inherit_chunk_deleted() RETURNS TRIGGER AS $$
BEGIN
    SELECT (a.deleted_at IS NOT NULL) INTO NEW.deleted
    FROM articles a WHERE a.id = NEW.article_id;
    IF NEW.deleted IS NULL THEN
        NEW.deleted := false;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_inherit_chunk_deleted ON chunks;
CREATE TRIGGER trg_inherit_chunk_deleted
    BEFORE INSERT ON chunks
    FOR EACH ROW
    EXECUTE FUNCTION inherit_chunk_deleted();

-- ---- job_queue.updated_at 自动维护 ----
CREATE OR REPLACE FUNCTION update_job_queue_timestamp() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_job_queue_updated_at ON job_queue;
CREATE TRIGGER trg_job_queue_updated_at
    BEFORE UPDATE ON job_queue
    FOR EACH ROW
    EXECUTE FUNCTION update_job_queue_timestamp();

-- ============================================================
-- 默认配置（L2 热配置）
-- ============================================================

INSERT INTO settings ("key", value, description) VALUES
('search.top_k', '5', 'RRF 融合后精排前保留条数'),
('search.vector_top_k', '20', '向量检索 TopK'),
('search.bm25_top_k', '20', 'BM25 检索 TopK'),
('search.rrf_k', '60', 'RRF 融合参数 k'),
('search.vector_weight', '0.5', '向量检索权重'),
('search.bm25_weight', '0.5', 'BM25 检索权重'),
('search.ef_search', '100', 'HNSW 查询时 ef_search'),
('chunk.size', '400', 'Chunk 大小（字符数），200-800'),
('chunk.overlap', '50', 'Chunk 重叠大小（字符数），< size'),
('mcp_log.retention_days', '30', 'MCP 日志保留天数，1-365'),
('keyword_search.tag_weight', '10.0', '标签精确匹配权重'),
('keyword_search.category_weight', '8.0', '分类精确匹配权重'),
('keyword_search.title_weight', '3.0', '标题匹配权重'),
('keyword_search.summary_weight', '1.5', '摘要匹配权重'),
('keyword_search.content_weight', '1.0', '正文匹配权重'),
('collector.timeout_ms', '10000', 'URL 采集超时毫秒'),
('collector.max_redirects', '3', '最大重定向次数'),
('collector.min_content_length', '200', '最小内容长度（字符）')
ON CONFLICT ("key") DO NOTHING;
