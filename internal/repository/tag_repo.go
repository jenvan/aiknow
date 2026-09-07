package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/aiknow-org/aiknow/internal/model"
)

// TagRepositoryImpl Tag 仓库实现
type TagRepositoryImpl struct {
	db *PostgresDB
}

// NewTagRepository 创建标签仓库
func NewTagRepository(db *PostgresDB) *TagRepositoryImpl {
	return &TagRepositoryImpl{db: db}
}

// Create 创建标签
func (r *TagRepositoryImpl) Create(ctx context.Context, tag *model.Tag) (*model.Tag, error) {
	tag.ID = GenerateUUID()
	tag.CreatedAt = time.Now()

	_, err := r.db.pool.Exec(ctx, `
		INSERT INTO tags (id, name, created_at)
		VALUES ($1, $2, $3)
	`, tag.ID, tag.Name, tag.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("insert tag: %w", err)
	}

	return tag, nil
}

// GetByID 根据 ID 查询标签
func (r *TagRepositoryImpl) GetByID(ctx context.Context, id string) (*model.Tag, error) {
	tag := &model.Tag{}
	err := r.db.pool.QueryRow(ctx,
		"SELECT id, name, created_at FROM tags WHERE id = $1", id).
		Scan(&tag.ID, &tag.Name, &tag.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("query tag: %w", err)
	}

	return tag, nil
}

// GetByName 根据名称查询标签（大小写不敏感）
func (r *TagRepositoryImpl) GetByName(ctx context.Context, name string) (*model.Tag, error) {
	tag := &model.Tag{}
	err := r.db.pool.QueryRow(ctx,
		"SELECT id, name, created_at FROM tags WHERE lower(name) = lower($1)", name).
		Scan(&tag.ID, &tag.Name, &tag.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("query tag by name: %w", err)
	}

	return tag, nil
}

// ListAll 查询所有标签
func (r *TagRepositoryImpl) ListAll(ctx context.Context) ([]model.Tag, error) {
	rows, err := r.db.pool.Query(ctx,
		"SELECT id, name, created_at FROM tags ORDER BY name")
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

// BulkCreate 批量创建标签
func (r *TagRepositoryImpl) BulkCreate(ctx context.Context, tags []*model.Tag) ([]model.Tag, error) {
	result := make([]model.Tag, 0, len(tags))

	for _, tag := range tags {
		created, err := r.GetOrCreateByName(ctx, tag.Name)
		if err != nil {
			return nil, err
		}
		result = append(result, *created)
	}

	return result, nil
}

// GetOrCreateByName 获取或创建标签
func (r *TagRepositoryImpl) GetOrCreateByName(ctx context.Context, name string) (*model.Tag, error) {
	tag, err := r.GetByName(ctx, name)
	if err == nil && tag != nil {
		return tag, nil
	}

	tag = &model.Tag{Name: name}
	return r.Create(ctx, tag)
}

// LinkTagsToArticle 关联标签到文章
func (r *TagRepositoryImpl) LinkTagsToArticle(ctx context.Context, articleID string, tagNames []string) error {
	tx, err := r.db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, name := range tagNames {
		tag, err := r.GetOrCreateByName(ctx, name)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO article_tags (article_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, articleID, tag.ID)

		if err != nil {
			return fmt.Errorf("link tag: %w", err)
		}
	}

	return tx.Commit(ctx)
}
