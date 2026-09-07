-- 启用 PGMQ 扩展
CREATE EXTENSION IF NOT EXISTS pgmq;

-- 创建默认队列（应用层通过 pgmq.create_queue 创建，这里仅作验证）
-- 队列名：article_processing
-- 消息格式：
-- {
--   "article_id": "uuid",
--   "source_type": "manual|url",
--   "title": "string",
--   "content": "string",
--   "source_url": "string|null",
--   "category_id": "uuid|null",
--   "tags": ["string"]
-- }
