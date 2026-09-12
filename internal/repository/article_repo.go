package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jenvan/aiknow/internal/model"
)

// ArticleRepositoryImpl Article 仓库实现
type ArticleRepositoryImpl struct {
	db *PostgresDB
}

// NewArticleRepository 创建文章仓库
func NewArticleRepository(db *PostgresDB) *ArticleRepositoryImpl {
	return &ArticleRepositoryImpl{db: db}
}

// Create 创建文章
func (r *ArticleRepositoryImpl) Create(ctx context.Context, article *model.Article) error {
	article.ID = GenerateUUID()
	article.CreatedAt = time.Now()
	article.UpdatedAt = time.Now()

	// 计算内容 hash
	hash := sha256.Sum256([]byte(article.Content))
	contentHash := hex.EncodeToString(hash[:])

	// 插入文章
	_, err := r.db.pool.Exec(ctx, `
		INSERT INTO articles (
			id, title, content, source_type, source_url, 
			processing_status, published, view_count,
			deleted_at, created_at, updated_at, content_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, article.ID, article.Title, article.Content, article.SourceType,
		article.SourceURL, "pending", article.Published, 0,
		article.DeletedAt, article.CreatedAt, article.UpdatedAt, contentHash)

	if err != nil {
		return fmt.Errorf("insert article: %w", err)
	}

	return nil
}

// GetByID 根据 ID 查询文章
func (r *ArticleRepositoryImpl) GetByID(ctx context.Context, id string) (*model.Article, error) {
	article := &model.Article{}

	err := r.db.pool.QueryRow(ctx, `
		SELECT 
			a.id, a.title, a.content, a.source_type, a.source_url,
			a.processing_status, a.summary, a.category_id, a.published,
			a.view_count, a.embedding_model, a.embedding_dim, a.indexed_at,
			a.deleted_at, a.retry_count, a.last_error,
			a.created_at, a.updated_at
		FROM articles a
		WHERE a.id = $1 AND a.deleted_at IS NULL
	`, id).Scan(
		&article.ID, &article.Title, &article.Content, &article.SourceType,
		&article.SourceURL, &article.ProcessingStatus, &article.Summary,
		&article.CategoryID, &article.Published, &article.ViewCount,
		&article.EmbeddingModel, &article.EmbeddingDim, &article.IndexedAt,
		&article.DeletedAt, &article.RetryCount, &article.LastError,
		&article.CreatedAt, &article.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query article: %w", err)
	}

	// 查询分类名
	var categoryName string
	if article.CategoryID != nil {
		err = r.db.pool.QueryRow(ctx, "SELECT name FROM categories WHERE id = $1", *article.CategoryID).Scan(&categoryName)
		if err == nil {
			article.CategoryName = categoryName
		}
	}

	// 查询标签
	tags, err := r.getTagsByArticleID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	article.Tags = tags

	return article, nil
}

// List 分页查询文章列表
func (r *ArticleRepositoryImpl) List(ctx context.Context, query model.ArticleListQuery) (*model.ArticleListResult, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}

	offset := (query.Page - 1) * query.PageSize

	// 构建 WHERE 子句
	conditions := []string{"a.deleted_at IS NULL"}
	args := []interface{}{}
	argIdx := 1

	if query.CategoryID != nil && *query.CategoryID != "" {
		conditions = append(conditions, fmt.Sprintf("a.category_id = $%d", argIdx))
		args = append(args, *query.CategoryID)
		argIdx++
	}

	if len(query.TagIDs) > 0 {
		placeholders := make([]string, len(query.TagIDs))
		for i, id := range query.TagIDs {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, id)
			argIdx++
		}
		conditions = append(conditions, fmt.Sprintf("a.id IN (SELECT article_id FROM article_tags WHERE tag_id IN (%s))", strings.Join(placeholders, ",")))
	}

	if query.Status != nil {
		conditions = append(conditions, fmt.Sprintf("a.processing_status = $%d", argIdx))
		args = append(args, *query.Status)
		argIdx++
	}

	if query.Search != nil && *query.Search != "" {
		// 逗号分隔多个关键词，每个词独立匹配（AND 语义）：
		// - 含中文的关键词用 ILIKE 精确匹配，避免 jieba 拆分
		// - 纯英文关键词用 |||，利用 jieba 分词召回
		parts := strings.Split(*query.Search, ",")
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				if hasChinese(t) {
					// 中文：ILIKE 精确匹配，避免分词
					conditions = append(conditions,
						fmt.Sprintf("(a.title ILIKE $%d OR a.summary ILIKE $%d OR a.content ILIKE $%d)", argIdx, argIdx, argIdx))
					args = append(args, "%"+t+"%")
				} else {
					// 英文：||| 分词匹配
					conditions = append(conditions,
						fmt.Sprintf("(a.title ||| $%d OR a.summary ||| $%d OR a.content ||| $%d)", argIdx, argIdx, argIdx))
					args = append(args, t)
				}
				argIdx++
			}
		}
	}

	if query.DateFrom != "" {
		conditions = append(conditions, fmt.Sprintf("a.created_at >= $%d::date", argIdx))
		args = append(args, query.DateFrom)
		argIdx++
	}

	if query.DateTo != "" {
		conditions = append(conditions, fmt.Sprintf("a.created_at <= $%d::date", argIdx))
		args = append(args, query.DateTo)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// 排序
	validSortFields := map[string]string{
		"created_at": "a.created_at",
		"updated_at": "a.updated_at",
		"view_count": "a.view_count",
	}
	sortField := validSortFields[query.SortBy]
	if sortField == "" {
		sortField = "a.created_at"
	}
	sortOrder := "DESC"
	if query.SortOrder == "asc" {
		sortOrder = "ASC"
	}

	// 查询总数
	var total int
	err := r.db.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM articles a WHERE %s", whereClause), args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("count articles: %w", err)
	}

	pages := (total + query.PageSize - 1) / query.PageSize

	// 查询列表
	rows, err := r.db.pool.Query(ctx, fmt.Sprintf(`
		SELECT 
			a.id, a.title, a.content, a.source_type, a.source_url,
			a.processing_status, a.summary, a.category_id, a.published,
			a.view_count, a.embedding_model, a.embedding_dim, a.indexed_at,
			a.deleted_at, a.retry_count, a.last_error,
			a.created_at, a.updated_at
		FROM articles a
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortField, sortOrder, argIdx, argIdx+1),
		// 顺序必须与 LIMIT/OFFSET 占位符一致：曾误传 (offset, pageSize)，
		// 首页 offset=0 使 LIMIT 0，列表恒为空而 total 正常
		append(args, query.PageSize, offset)...)
	if err != nil {
		return nil, fmt.Errorf("query articles: %w", err)
	}
	defer rows.Close()

	articles := make([]model.Article, 0)
	for rows.Next() {
		var article model.Article
		err := rows.Scan(
			&article.ID, &article.Title, &article.Content, &article.SourceType,
			&article.SourceURL, &article.ProcessingStatus, &article.Summary,
			&article.CategoryID, &article.Published, &article.ViewCount,
			&article.EmbeddingModel, &article.EmbeddingDim, &article.IndexedAt,
			&article.DeletedAt, &article.RetryCount, &article.LastError,
			&article.CreatedAt, &article.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan article: %w", err)
		}

		// 查询分类名
		var categoryName string
		if article.CategoryID != nil {
			_ = r.db.pool.QueryRow(ctx, "SELECT name FROM categories WHERE id = $1", *article.CategoryID).Scan(&categoryName)
			article.CategoryName = categoryName
		}

		// 查询标签
		tags, err := r.getTagsByArticleID(ctx, article.ID)
		if err == nil {
			article.Tags = tags
		}

		articles = append(articles, article)
	}

	return &model.ArticleListResult{
		Total:    total,
		Pages:    pages,
		Articles: articles,
	}, nil
}

// Update 更新文章
func (r *ArticleRepositoryImpl) Update(ctx context.Context, article *model.Article) error {
	article.UpdatedAt = time.Now()

	// indexed_at / embedding_model / embedding_dim / last_error 必须一并写入：
	// AI 流水线正是通过本方法保存这几项，此前 SET 列表遗漏，
	// 处理完成后索引时间与向量模型信息始终为空，失败原因也无处落库
	_, err := r.db.pool.Exec(ctx, `
		UPDATE articles SET
			title = $1, content = $2, source_url = $3,
			processing_status = $4, summary = $5, category_id = $6,
			published = $7, indexed_at = $8, embedding_model = $9,
			embedding_dim = $10, last_error = $11, updated_at = $12
		WHERE id = $13 AND deleted_at IS NULL
	`, article.Title, article.Content, article.SourceURL,
		article.ProcessingStatus, article.Summary, article.CategoryID,
		article.Published, article.IndexedAt, article.EmbeddingModel,
		article.EmbeddingDim, article.LastError, article.UpdatedAt, article.ID)

	if err != nil {
		return fmt.Errorf("update article: %w", err)
	}

	return nil
}

// SoftDelete 软删除文章
func (r *ArticleRepositoryImpl) SoftDelete(ctx context.Context, id string, deletedBy *string) error {
	_, err := r.db.pool.Exec(ctx, `
		UPDATE articles SET
			deleted_at = now(),
			deleted_by = $1
		WHERE id = $2 AND deleted_at IS NULL
	`, deletedBy, id)

	if err != nil {
		return fmt.Errorf("soft delete article: %w", err)
	}

	return nil
}

// Restore 恢复文章
func (r *ArticleRepositoryImpl) Restore(ctx context.Context, id string) error {
	_, err := r.db.pool.Exec(ctx, `
		UPDATE articles SET
			deleted_at = NULL,
			deleted_by = NULL
		WHERE id = $1
	`, id)

	if err != nil {
		return fmt.Errorf("restore article: %w", err)
	}

	return nil
}

// CountByStatus 按状态分组计数
func (r *ArticleRepositoryImpl) CountByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.pool.Query(ctx, `
		SELECT processing_status, COUNT(*) 
		FROM articles 
		WHERE deleted_at IS NULL
		GROUP BY processing_status
	`)
	if err != nil {
		return nil, fmt.Errorf("count by status: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan count: %w", err)
		}
		result[status] = count
	}

	return result, nil
}

// FindByURL 根据 URL 查找文章（去重）
func (r *ArticleRepositoryImpl) FindByURL(ctx context.Context, url string) (*model.Article, error) {
	article := &model.Article{}
	err := r.db.pool.QueryRow(ctx, `
		SELECT id, title, source_type, source_url, processing_status, created_at
		FROM articles
		WHERE source_url = $1 AND deleted_at IS NULL
		LIMIT 1
	`, url).Scan(
		&article.ID, &article.Title, &article.SourceType, &article.SourceURL,
		&article.ProcessingStatus, &article.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find by URL: %w", err)
	}

	return article, nil
}

// FindByContentHash 根据内容 hash 查找文章（去重）
func (r *ArticleRepositoryImpl) FindByContentHash(ctx context.Context, hash string) (*model.Article, error) {
	article := &model.Article{}
	err := r.db.pool.QueryRow(ctx, `
		SELECT id, title, source_type, source_url, processing_status, created_at
		FROM articles
		WHERE content_hash = $1 AND deleted_at IS NULL
		LIMIT 1
	`, hash).Scan(
		&article.ID, &article.Title, &article.SourceType, &article.SourceURL,
		&article.ProcessingStatus, &article.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find by content hash: %w", err)
	}

	return article, nil
}

// BulkUpdateStatus 批量更新状态
func (r *ArticleRepositoryImpl) BulkUpdateStatus(ctx context.Context, ids []string, status string) error {
	if len(ids) == 0 {
		return nil
	}

	args := make([]interface{}, len(ids)+1)
	args[0] = status
	for i, id := range ids {
		args[i+1] = id
	}

	_, err := r.db.pool.Exec(ctx,
		fmt.Sprintf("UPDATE articles SET processing_status = $1 WHERE id = ANY($%d) AND deleted_at IS NULL", len(ids)+1),
		args...)

	if err != nil {
		return fmt.Errorf("bulk update status: %w", err)
	}

	return nil
}

// BulkSoftDelete 批量软删除
func (r *ArticleRepositoryImpl) BulkSoftDelete(ctx context.Context, ids []string, deletedBy *string) error {
	if len(ids) == 0 {
		return nil
	}

	_, err := r.db.pool.Exec(ctx,
		"UPDATE articles SET deleted_at = now(), deleted_by = $1 WHERE id = ANY($2) AND deleted_at IS NULL",
		deletedBy, ids,
	)

	if err != nil {
		return fmt.Errorf("bulk soft delete: %w", err)
	}

	return nil
}

// getTagsByArticleID 查询文章的标签
func (r *ArticleRepositoryImpl) getTagsByArticleID(ctx context.Context, articleID string) ([]model.Tag, error) {
	rows, err := r.db.pool.Query(ctx, `
		SELECT t.id, t.name, t.created_at
		FROM tags t
		JOIN article_tags at ON t.id = at.tag_id
		WHERE at.article_id = $1
	`, articleID)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var tag model.Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

// GetArticleStatus 查询文章处理状态
func (r *ArticleRepositoryImpl) GetArticleStatus(ctx context.Context, id string) (*model.ArticleStatus, error) {
	status := &model.ArticleStatus{}
	err := r.db.pool.QueryRow(ctx, `
		SELECT id, processing_status, retry_count, last_error, indexed_at
		FROM articles
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&status.ID, &status.ProcessingStatus, &status.RetryCount,
		&status.LastError, &status.IndexedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query article status: %w", err)
	}

	return status, nil
}

// IncrementViewCount 增加浏览次数
func (r *ArticleRepositoryImpl) IncrementViewCount(ctx context.Context, id string) error {
	_, err := r.db.pool.Exec(ctx,
		"UPDATE articles SET view_count = view_count + 1 WHERE id = $1", id)
	return err
}

// getTagsByArticleIDs 批量查询多个文章的标签
func (r *ArticleRepositoryImpl) getTagsByArticleIDs(ctx context.Context, articleIDs []string) (map[string][]model.Tag, error) {
	if len(articleIDs) == 0 {
		return map[string][]model.Tag{}, nil
	}
	placeholders := make([]string, len(articleIDs))
	args := make([]interface{}, len(articleIDs))
	for i, id := range articleIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	rows, err := r.db.pool.Query(ctx, `
		SELECT at.article_id, t.id, t.name, t.created_at
		FROM article_tags at
		JOIN tags t ON t.id = at.tag_id
		WHERE at.article_id IN (`+strings.Join(placeholders, ",")+")", args...)
	if err != nil {
		return nil, fmt.Errorf("batch query tags: %w", err)
	}
	defer rows.Close()

	m := make(map[string][]model.Tag)
	for rows.Next() {
		var tag model.Tag
		var articleID string
		if err := rows.Scan(&articleID, &tag.ID, &tag.Name, &tag.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		m[articleID] = append(m[articleID], tag)
	}
	return m, nil
}

// hasChinese 检查字符串是否包含中文字符
func hasChinese(s string) bool {
	for _, r := range s {
		if r >= '一' && r <= '鿿' {
			return true
		}
	}
	return false
}
