package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/google/uuid"
)

// APIKeyRepositoryImpl API Key 仓库实现
type APIKeyRepositoryImpl struct {
	db *PostgresDB
}

// NewAPIKeyRepository 创建 API Key 仓库
func NewAPIKeyRepository(db *PostgresDB) *APIKeyRepositoryImpl {
	return &APIKeyRepositoryImpl{db: db}
}

// Create 创建 API Key
func (r *APIKeyRepositoryImpl) Create(ctx context.Context, key *model.APIKey) error {
	key.ID = uuid.New().String()
	key.CreatedAt = time.Now()

	// 计算 key 哈希
	hash := sha256.Sum256([]byte(key.KeyHash))
	key.KeyHash = hex.EncodeToString(hash[:])

	_, err := r.db.pool.Exec(ctx, `
		INSERT INTO api_keys (id, key_hash, key_prefix, description, scope, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, key.ID, key.KeyHash, key.KeyPrefix, key.Description, key.Scope, key.IsActive, key.CreatedAt)

	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	return nil
}

// GetByID 根据 ID 获取 API Key
func (r *APIKeyRepositoryImpl) GetByID(ctx context.Context, id string) (*model.APIKey, error) {
	key := &model.APIKey{}
	err := r.db.pool.QueryRow(ctx, `
		SELECT id, key_hash, key_prefix, description, scope, is_active, last_used_at, created_at
		FROM api_keys WHERE id = $1
	`, id).Scan(&key.ID, &key.KeyHash, &key.KeyPrefix, &key.Description, &key.Scope, &key.IsActive, &key.LastUsedAt, &key.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get api key: %w", err)
	}

	return key, nil
}

// GetByHash 根据哈希获取 API Key
func (r *APIKeyRepositoryImpl) GetByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	key := &model.APIKey{}
	err := r.db.pool.QueryRow(ctx, `
		SELECT id, key_hash, key_prefix, description, scope, is_active, last_used_at, created_at
		FROM api_keys WHERE key_hash = $1
	`, hash).Scan(&key.ID, &key.KeyHash, &key.KeyPrefix, &key.Description, &key.Scope, &key.IsActive, &key.LastUsedAt, &key.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}

	return key, nil
}

// ListAll 获取所有 API Key
func (r *APIKeyRepositoryImpl) ListAll(ctx context.Context) ([]model.APIKey, error) {
	rows, err := r.db.pool.Query(ctx, `
		SELECT id, key_hash, key_prefix, description, scope, is_active, last_used_at, created_at
		FROM api_keys ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []model.APIKey
	for rows.Next() {
		var key model.APIKey
		if err := rows.Scan(&key.ID, &key.KeyHash, &key.KeyPrefix, &key.Description, &key.Scope, &key.IsActive, &key.LastUsedAt, &key.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, key)
	}

	return keys, nil
}

// Update 更新 API Key
func (r *APIKeyRepositoryImpl) Update(ctx context.Context, key *model.APIKey) error {
	_, err := r.db.pool.Exec(ctx, `
		UPDATE api_keys SET
			scope = $1,
			is_active = $2,
			description = $3
		WHERE id = $4
	`, key.Scope, key.IsActive, key.Description, key.ID)

	if err != nil {
		return fmt.Errorf("update api key: %w", err)
	}

	return nil
}

// Delete 删除 API Key
func (r *APIKeyRepositoryImpl) Delete(ctx context.Context, id string) error {
	_, err := r.db.pool.Exec(ctx, `
		DELETE FROM api_keys WHERE id = $1
	`, id)

	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}

	return nil
}
