package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/pgvector/pgvector-go"
)

// SearchRepositoryImpl 检索仓库实现
type SearchRepositoryImpl struct {
	db *PostgresDB
}

// NewSearchRepository 创建检索仓库
func NewSearchRepository(db *PostgresDB) *SearchRepositoryImpl {
	return &SearchRepositoryImpl{db: db}
}

// VectorSearch 向量检索（pgvector cosine similarity）
func (r *SearchRepositoryImpl) VectorSearch(ctx context.Context, query []float32, topK int) ([]model.SearchChunk, error) {
	if topK <= 0 {
		topK = 20
	}
	// pgvector <-> 操作符：cosine distance，越小越相似
	queryVec := pgvector.NewVector(query)
	rows, err := r.db.pool.Query(ctx, `
		SELECT id, article_id, chunk_index, content, embedding
		FROM chunks
		WHERE embedding IS NOT NULL
		ORDER BY embedding <-> $1
		LIMIT $2
	`, queryVec, topK)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []model.SearchChunk
	for rows.Next() {
		var emb pgvector.Vector
		var c model.SearchChunk
		if err := rows.Scan(&c.ID, &c.ArticleID, &c.ChunkIndex, &c.Content, &emb); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		c.Embedding = emb.Slice()
		results = append(results, c)
	}
	return results, nil
}

// BM25Search BM25 全文检索（利用 PostgreSQL ts_rank）
func (r *SearchRepositoryImpl) BM25Search(ctx context.Context, query string, topK int) ([]model.SearchChunk, error) {
	if topK <= 0 {
		topK = 20
	}
	// 将中文查询转换为 tsquery（简单实现：空格分隔词）
	// 生产环境应接入 zhparser 分词
	_ = strings.Fields(query) // TODO: zhparser 分词
	// 构建 tsquery：A & B & C（保留结构以便后续接入 zhparser）
	var tsQuery strings.Builder
	for i, q := range strings.Fields(query) {
		// 清理特殊字符
		clean := strings.TrimSpace(q)
		if clean == "" {
			continue
		}
		if i > 0 {
			tsQuery.WriteString(" & ")
		}
		tsQuery.WriteString(fmt.Sprintf("to_tsquery('simple', '%s')", clean))
	}
	_ = tsQuery.String() // 保留 tsquery 构建逻辑，实际 SQL 使用 plainto_tsquery

	rows, err := r.db.pool.Query(ctx, `
		SELECT id, article_id, chunk_index, content,
		       ts_rank(to_tsvector('simple', content), plainto_tsquery('simple', $1)) AS rank
		FROM chunks
		WHERE to_tsvector('simple', content) @@ plainto_tsquery('simple', $1)
		ORDER BY rank DESC
		LIMIT $2
	`, query, topK)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}
	defer rows.Close()

	var results []model.SearchChunk
	for rows.Next() {
		var c model.SearchChunk
		var rank float64
		if err := rows.Scan(&c.ID, &c.ArticleID, &c.ChunkIndex, &c.Content, &rank); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		c.Score = rank
		results = append(results, c)
	}
	return results, nil
}

// KeywordSearch 关键词检索
func (r *SearchRepositoryImpl) KeywordSearch(ctx context.Context, keywords []string, params model.KeywordSearchParams) ([]model.KeywordSearchResult, error) {
	// TODO: 完整实现关键词加权检索
	var results []model.KeywordSearchResult
	return results, nil
}

// GetArticleByID 根据 ID 查询文章
func (r *SearchRepositoryImpl) GetArticleByID(ctx context.Context, id string) (*model.Article, error) {
	return (&ArticleRepositoryImpl{db: r.db}).GetByID(ctx, id)
}

// GetCategories 获取所有分类
func (r *SearchRepositoryImpl) GetCategories(ctx context.Context) ([]model.Category, error) {
	return (&CategoryRepositoryImpl{db: r.db}).ListAll(ctx)
}

// GetTags 获取所有标签
func (r *SearchRepositoryImpl) GetTags(ctx context.Context) ([]model.Tag, error) {
	return (&TagRepositoryImpl{db: r.db}).ListAll(ctx)
}
