-- 启用 zhparser 扩展（中文分词）
-- 注意：需要在支持的环境中执行，扩展可能不可用时跳过
CREATE EXTENSION IF NOT EXISTS zhparser;
DROP TEXT SEARCH CONFIGURATION IF EXISTS chinese CASCADE;
CREATE TEXT SEARCH CONFIGURATION chinese (PARSER = zhparser);
ALTER TEXT SEARCH CONFIGURATION chinese
    ADD MAPPING FOR n,v,a,i,e,l,j,r,d,m,q,t,z WITH simple;

-- 创建全文检索索引
-- 为文章表创建全文检索索引
CREATE INDEX IF NOT EXISTS idx_articles_title_tsv ON articles USING gin(
    to_tsvector('chinese', title)
) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_articles_summary_tsv ON articles USING gin(
    to_tsvector('chinese', COALESCE(summary, ''))
) WHERE deleted_at IS NULL AND summary IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_articles_content_tsv ON articles USING gin(
    to_tsvector('chinese', content)
) WHERE deleted_at IS NULL;

-- 为标签和分类创建精确匹配索引
CREATE INDEX IF NOT EXISTS idx_tags_name_lower ON tags(lower(name));
CREATE INDEX IF NOT EXISTS idx_categories_name_lower ON categories(lower(name));
