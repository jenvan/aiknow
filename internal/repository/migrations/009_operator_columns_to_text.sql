-- 操作者列由 UUID 改为 TEXT
--
-- 背景：api_keys.created_by / setting_history.changed_by / articles.deleted_by
-- 三列建表时定为 UUID，隐含"存在用户表、操作者有 UUID 主键"的前提。
-- 但 AGENT.md 决策 #10 明确 v1 单管理员、凭据来自配置文件、**无用户表**，
-- 系统中根本不存在可填入的用户 UUID。
--
-- 实际后果：写入 created_by 时报
--   ERROR: invalid input syntax for type uuid (SQLSTATE 22P02)
-- 此前未暴露仅仅因为这三列从未被写入过。
--
-- 改为 TEXT 存用户名（如 'admin'、'cli'）。待 v2 引入用户表后，
-- 可再迁移为对 users(id) 的外键。
--
-- USING 子句：三列现均为空，转换不会丢数据。

ALTER TABLE api_keys
    ALTER COLUMN created_by TYPE TEXT USING created_by::TEXT;

ALTER TABLE setting_history
    ALTER COLUMN changed_by TYPE TEXT USING changed_by::TEXT;

ALTER TABLE articles
    ALTER COLUMN deleted_by TYPE TEXT USING deleted_by::TEXT;
