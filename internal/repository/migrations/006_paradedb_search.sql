-- ParadeDB (pg_search) BM25 检索，替换 zhparser + tsvector
--
-- 前置条件（不在本迁移内完成）：
--   pg_search 与 vector 扩展须由 DBA 或容器初始化脚本预先创建，
--   因为本项目的迁移执行器会跳过一切扩展创建语句。详见 docs/DEPLOY.md。
--
-- 注意：本文件中禁止出现扩展创建语句的英文字面量（含注释），
-- 否则迁移执行器的跳过逻辑会误伤紧随其后的语句。

-- 1. 清理旧的 zhparser / tsvector 对象
DROP INDEX IF EXISTS idx_articles_title_tsv;
DROP INDEX IF EXISTS idx_articles_summary_tsv;
DROP INDEX IF EXISTS idx_articles_content_tsv;
DROP TEXT SEARCH CONFIGURATION IF EXISTS chinese CASCADE;

-- 2. chunks 表增加冗余的 deleted 标记
--
-- 原因：BM25 过滤条件必须能下推进 pg_search 索引，否则要先 JOIN articles 再过滤，
-- topK 与分页会失真（取回的候选集在过滤后不足 K 条）。
-- 该列由下方触发器与 articles.deleted_at 保持同步。
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS deleted BOOLEAN NOT NULL DEFAULT false;

-- 回填历史数据
UPDATE chunks c
SET deleted = true
FROM articles a
WHERE c.article_id = a.id
  AND a.deleted_at IS NOT NULL
  AND c.deleted = false;

-- 3. 同步触发器
--
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

-- 4. chunks BM25 索引
--
-- 语法说明（pg_search 0.25.x）：
--   - 访问方法为 paradedb（0.25.0 起由 bm25 更名，bm25 保留为兼容别名）
--   - 分词器通过类型转换声明：(content::pdb.jieba)
--   - key_field 须唯一、位于列表首位；每表只允许一个 paradedb 索引
--   - deleted / article_id 作为可过滤字段一并入索引，使过滤条件可下推
--
-- 待验证：官方文档未明确 uuid 可否作 key_field（三项要求 uuid 均满足，
-- 但无明文背书）。若 CREATE INDEX 报类型错误，则需为 chunks 增加
-- bigint 代理键（GENERATED ALWAYS AS IDENTITY）并改用该列作 key_field。
DROP INDEX IF EXISTS chunks_search_idx;
CREATE INDEX chunks_search_idx ON chunks
USING paradedb (id, (content::pdb.jieba), deleted, article_id, chunk_index)
WITH (key_field = 'id');

-- 5. articles BM25 索引（供关键词加权检索使用，替代原 title/summary/content 三个 GIN 索引）
DROP INDEX IF EXISTS articles_search_idx;
CREATE INDEX articles_search_idx ON articles
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

-- 6. 保留标签与分类的精确匹配索引（原 003 中的部分，与全文检索无关）
CREATE INDEX IF NOT EXISTS idx_tags_name_lower ON tags(lower(name));
CREATE INDEX IF NOT EXISTS idx_categories_name_lower ON categories(lower(name));
