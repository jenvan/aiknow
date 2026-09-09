-- 配置层级梳理（v0.6.2）
--
-- 背景：管理后台（L2 settings 表）此前混入了两类不该出现的配置——
--
--   1. 与表结构/启动参数绑定的固定配置：
--      embedding.dimension 与 VECTOR(1024) 表结构绑定，改动需重建表与重灌向量，
--      不支持运行时修改，应彻底移出 L2（保持代码硬编码 1024）。
--
--   2. 启动期即固定的采集参数（http.Client 在 serve() 启动时创建，时超时与
--      重定向次数等已写入连接池，运行时改 settings 表并不生效）：
--      collector.timeout_ms / collector.max_redirects / collector.min_content_length
--      应移入 L1 config.yaml（COLLECTOR_TIMEOUT / COLLECTOR_MAX_REDIRECTS /
--      COLLECTOR_MIN_CONTENT_LENGTH），需重启生效。
--
-- 同时为 L2Config.LLM.Model 播种空值：默认"未覆盖"，实际模型名取自 L1
-- QWEN_MODEL；后台显式设置后即覆盖（优先级最高）。

DELETE FROM settings WHERE "key" IN (
    'embedding.dimension',
    'collector.timeout_ms',
    'collector.max_redirects',
    'collector.min_content_length'
);

INSERT INTO settings ("key", value, description) VALUES
('llm.model', '""', 'LLM 使用的模型名称，留空使用启动配置 QWEN_MODEL')
ON CONFLICT ("key") DO NOTHING;