-- 已废弃：原 zhparser + tsvector 全文检索方案
--
-- 架构调整后改用 ParadeDB (pg_search) 的 jieba 分词 + BM25，见 006_paradedb_search.sql。
-- 本文件保留为空操作：已有开发库中 003 已记录为"已应用"，原地改写不会重跑，
-- 因此新逻辑必须放在新版本号里。旧对象的清理在 006 中以 IF EXISTS 方式完成。
DO $$ BEGIN END $$;
