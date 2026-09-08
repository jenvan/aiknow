-- 已废弃：原 PGMQ 扩展方案
--
-- PGMQ 需要编译安装且不在 ParadeDB 镜像内，改用数据表 + FOR UPDATE SKIP LOCKED，
-- 见 007_job_queue.sql。
-- 本文件保留为空操作，原因同 003_pg_textsearch_bm25.sql。
DO $$ BEGIN END $$;
