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

// VectorSearch 向量检索（pgvector 余弦相似度）
func (r *SearchRepositoryImpl) VectorSearch(ctx context.Context, query []float32, topK int) ([]model.SearchChunk, error) {
	if topK <= 0 {
		topK = 20
	}
	// 使用 <=> （余弦距离），与 002_vector.sql 中 HNSW 索引的 vector_cosine_ops
	// 保持一致；此前误用 <-> （L2 距离）导致索引失效、退化为全表扫描。
	queryVec := pgvector.NewVector(query)
	rows, err := r.db.pool.Query(ctx, `
		SELECT c.id, c.article_id, c.chunk_index, c.content, c.embedding,
		       1 - (c.embedding <=> $1) AS similarity
		FROM chunks c
		WHERE c.embedding IS NOT NULL
		  AND c.deleted = false
		ORDER BY c.embedding <=> $1
		LIMIT $2
	`, queryVec, topK)
	if err != nil {
		return nil, fmt.Errorf("向量检索: %w", err)
	}
	defer rows.Close()

	var results []model.SearchChunk
	for rows.Next() {
		var emb pgvector.Vector
		var c model.SearchChunk
		var similarity float64
		if err := rows.Scan(&c.ID, &c.ArticleID, &c.ChunkIndex, &c.Content, &emb, &similarity); err != nil {
			return nil, fmt.Errorf("扫描 chunk: %w", err)
		}
		c.Embedding = emb.Slice()
		c.Score = similarity
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历向量检索结果: %w", err)
	}
	return results, nil
}

// BM25Search BM25 全文检索（ParadeDB pg_search + jieba 中文分词）
//
// 语法说明（pg_search 0.25.x）：
//   - ||| 为析取匹配算子，查询串按字段索引时的分词器（jieba）切分后匹配任一词元
//   - pdb.score(<key_field>) 取 BM25 相关性分值，参数须为索引声明的 key_field
//   - deleted 已随索引一同建立，过滤条件可下推，topK 不会因过滤而失真
func (r *SearchRepositoryImpl) BM25Search(ctx context.Context, query string, topK int) ([]model.SearchChunk, error) {
	if topK <= 0 {
		topK = 20
	}
	// 空查询无意义，直接返回；否则 ||| 会因空词元列表报错
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	rows, err := r.db.pool.Query(ctx, `
		SELECT id, article_id, chunk_index, content, pdb.score(id) AS score
		FROM chunks
		WHERE content ||| $1
		  AND deleted = false
		ORDER BY pdb.score(id) DESC, id ASC
		LIMIT $2
	`, query, topK)
	if err != nil {
		return nil, fmt.Errorf("BM25 检索: %w", err)
	}
	defer rows.Close()

	var results []model.SearchChunk
	for rows.Next() {
		var c model.SearchChunk
		var score float64
		if err := rows.Scan(&c.ID, &c.ArticleID, &c.ChunkIndex, &c.Content, &score); err != nil {
			return nil, fmt.Errorf("扫描 chunk: %w", err)
		}
		c.Score = score
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 BM25 检索结果: %w", err)
	}
	return results, nil
}

// KeywordSearch 关键词加权检索
//
// 按字段权重对文章打分：标签 > 分类 > 标题 > 摘要 > 正文。
// 标题/摘要/正文由 articles 上的 paradedb 索引以 BM25 打分（jieba 分词），
// 标签与分类为精确匹配（不分词），命中即计权重。
func (r *SearchRepositoryImpl) KeywordSearch(ctx context.Context, keywords []string, params model.KeywordSearchParams) ([]model.KeywordSearchResult, error) {
	// 过滤空关键词
	cleaned := make([]string, 0, len(keywords))
	for _, k := range keywords {
		if k = strings.TrimSpace(k); k != "" {
			cleaned = append(cleaned, k)
		}
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("关键词为空")
	}

	// 权重缺省值
	if params.TagWeight <= 0 {
		params.TagWeight = 3.0
	}
	if params.CategoryWeight <= 0 {
		params.CategoryWeight = 2.5
	}
	if params.TitleWeight <= 0 {
		params.TitleWeight = 2.0
	}
	if params.SummaryWeight <= 0 {
		params.SummaryWeight = 1.5
	}
	if params.ContentWeight <= 0 {
		params.ContentWeight = 1.0
	}
	if params.TopK <= 0 {
		params.TopK = 10
	}

	// 关键词以空格连接供 ||| 分词匹配；同时以数组形式做标签/分类精确匹配
	joined := strings.Join(cleaned, " ")
	lowered := make([]string, len(cleaned))
	for i, k := range cleaned {
		lowered[i] = strings.ToLower(k)
	}

	rows, err := r.db.pool.Query(ctx, `
		WITH text_hits AS (
			SELECT id,
			       pdb.score(id) AS bm25_score,
			       CASE WHEN title   ||| $1 THEN $3::float8 ELSE 0 END +
			       CASE WHEN summary ||| $1 THEN $4::float8 ELSE 0 END +
			       CASE WHEN content ||| $1 THEN $5::float8 ELSE 0 END AS field_weight
			FROM articles
			WHERE (title ||| $1 OR summary ||| $1 OR content ||| $1)
			  AND published = true
			LIMIT 500
		),
		tag_hits AS (
			SELECT at.article_id AS id, $6::float8 AS w
			FROM article_tags at
			JOIN tags t ON t.id = at.tag_id
			WHERE lower(t.name) = ANY($2)
		),
		category_hits AS (
			SELECT a.id, $7::float8 AS w
			FROM articles a
			JOIN categories c ON c.id = a.category_id
			WHERE lower(c.name) = ANY($2)
		),
		scored AS (
			SELECT id, sum(score) AS total FROM (
				SELECT id, bm25_score * field_weight AS score FROM text_hits
				UNION ALL SELECT id, w FROM tag_hits
				UNION ALL SELECT id, w FROM category_hits
			) s GROUP BY id
		)
		SELECT a.id, a.title, a.summary, a.category_id, a.source_type, a.source_url,
		       a.processing_status, a.published, a.view_count, a.created_at, a.updated_at,
		       s.total
		FROM scored s
		JOIN articles a ON a.id = s.id
		WHERE a.deleted_at IS NULL
		ORDER BY s.total DESC, a.created_at DESC
		LIMIT $8
	`, joined, lowered,
		params.TitleWeight, params.SummaryWeight, params.ContentWeight,
		params.TagWeight, params.CategoryWeight, params.TopK)
	if err != nil {
		return nil, fmt.Errorf("关键词检索: %w", err)
	}
	defer rows.Close()

	var articles []model.Article
	for rows.Next() {
		var a model.Article
		var score float64
		if err := rows.Scan(
			&a.ID, &a.Title, &a.Summary, &a.CategoryID, &a.SourceType, &a.SourceURL,
			&a.ProcessingStatus, &a.Published, &a.ViewCount, &a.CreatedAt, &a.UpdatedAt,
			&score,
		); err != nil {
			return nil, fmt.Errorf("扫描文章: %w", err)
		}
		articles = append(articles, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历关键词检索结果: %w", err)
	}

	return []model.KeywordSearchResult{{
		Total:    len(articles),
		Articles: articles,
	}}, nil
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
