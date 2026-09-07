-- 启用 pgvector 扩展
CREATE EXTENSION IF NOT EXISTS vector;

-- 为 chunks 表创建 HNSW 向量索引
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw ON chunks USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);
