package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jenvan/aiknow/internal/model"
)

// CategoryRepositoryImpl Category 仓库实现
type CategoryRepositoryImpl struct {
	db *PostgresDB
}

// NewCategoryRepository 创建分类仓库
func NewCategoryRepository(db *PostgresDB) *CategoryRepositoryImpl {
	return &CategoryRepositoryImpl{db: db}
}

// Create 创建分类
func (r *CategoryRepositoryImpl) Create(ctx context.Context, category *model.Category) (*model.Category, error) {
	category.ID = GenerateUUID()
	category.CreatedAt = time.Now()

	_, err := r.db.pool.Exec(ctx, `
		INSERT INTO categories (id, name, description, created_at)
		VALUES ($1, $2, $3, $4)
	`, category.ID, category.Name, category.Description, category.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("insert category: %w", err)
	}

	return category, nil
}

// GetByID 根据 ID 查询分类
func (r *CategoryRepositoryImpl) GetByID(ctx context.Context, id string) (*model.Category, error) {
	category := &model.Category{}
	err := r.db.pool.QueryRow(ctx,
		"SELECT id, name, description, created_at FROM categories WHERE id = $1", id).
		Scan(&category.ID, &category.Name, &category.Description, &category.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("query category: %w", err)
	}

	return category, nil
}

// GetByName 根据名称查询分类（大小写不敏感）
func (r *CategoryRepositoryImpl) GetByName(ctx context.Context, name string) (*model.Category, error) {
	category := &model.Category{}
	err := r.db.pool.QueryRow(ctx,
		"SELECT id, name, description, created_at FROM categories WHERE lower(name) = lower($1)", name).
		Scan(&category.ID, &category.Name, &category.Description, &category.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("query category by name: %w", err)
	}

	return category, nil
}

// ListAll 查询所有分类
func (r *CategoryRepositoryImpl) ListAll(ctx context.Context) ([]model.Category, error) {
	rows, err := r.db.pool.Query(ctx,
		"SELECT id, name, description, created_at FROM categories ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("query categories: %w", err)
	}
	defer rows.Close()

	var categories []model.Category
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, c)
	}

	return categories, nil
}

// GetOrCreateByName 获取或创建分类
func (r *CategoryRepositoryImpl) GetOrCreateByName(ctx context.Context, name string) (*model.Category, error) {
	category, err := r.GetByName(ctx, name)
	if err == nil && category != nil {
		return category, nil
	}

	category = &model.Category{Name: name}
	return r.Create(ctx, category)
}
