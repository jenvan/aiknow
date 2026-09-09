-- 补齐 articles 表的处理重试列
--
-- 背景：internal/model/article.go 的 Article.RetryCount / LastError 与
-- ArticleStatus 自始就存在，article_repo.go 的 GetByID / List / GetArticleStatus
-- 三处 SELECT 也一直在读 a.retry_count 与 a.last_error，但 001 建表时漏掉了这两列，
-- 导致文章列表与详情接口报 42703 column does not exist。
--
-- 不在 001 里原地补列：001 已在各环境记录为"已应用"，改动不会重跑。
--
-- retry_count 扫入非指针 int，必须 NOT NULL DEFAULT 0；
-- last_error 扫入 *string，可空。
--
-- 当前仅被读取，尚无写入方：AI 处理流水线的重试计数目前记在 job_queue 表
-- （见 007），失败信息未回写 articles。后续若要在文章详情页展示处理失败原因，
-- 需由 worker 在转 failed 时写入这两列。

ALTER TABLE articles ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE articles ADD COLUMN IF NOT EXISTS last_error TEXT;
