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
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID
);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);

-- MCP 请求日志表
CREATE TABLE IF NOT EXISTS mcp_request_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(64) NOT NULL,
    method VARCHAR(50) NOT NULL,
    tool_name VARCHAR(100),
    request_body JSONB NOT NULL,
    response_body JSONB,
    response_headers JSONB,
    error_message TEXT,
    query_time_ms INTEGER,
    api_key_id UUID REFERENCES api_keys(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mcp_logs_trace_id ON mcp_request_logs(trace_id);
CREATE INDEX IF NOT EXISTS idx_mcp_logs_created_at ON mcp_request_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mcp_logs_tool_name ON mcp_request_logs(tool_name);
CREATE INDEX IF NOT EXISTS idx_mcp_logs_api_key_id ON mcp_request_logs(api_key_id);

-- 插入默认配置（L2 默认值）
INSERT INTO settings ("key", value, description) VALUES
('llm.temperature', '0.3', 'LLM 生成温度，0.0-2.0'),
('llm.max_tokens', '2000', 'LLM 最大生成 token 数，100-8000'),
('llm.retry_count', '3', 'LLM 调用失败重试次数'),
('llm.timeout_ms', '30000', 'LLM 调用超时毫秒'),
('embedding.dimension', '1024', '向量维度，与 bge-large-zh-v1.5 匹配'),
('embedding.batch_size', '10', 'Embedding 批量请求大小'),
('reranker.top_k', '20', 'Reranker 候选数量'),
('search.top_k', '5', '最终返回结果数'),
('search.candidate_k', '20', 'RRF 召回候选数'),
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
