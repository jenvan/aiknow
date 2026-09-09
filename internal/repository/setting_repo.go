package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/google/uuid"
)

// SettingRepositoryImpl 设置仓库实现
type SettingRepositoryImpl struct {
	db *PostgresDB
}

// NewSettingRepository 创建设置仓库
func NewSettingRepository(db *PostgresDB) *SettingRepositoryImpl {
	return &SettingRepositoryImpl{db: db}
}

// GetByKey 根据 key 获取设置
func (r *SettingRepositoryImpl) GetByKey(ctx context.Context, key string) (*model.Setting, error) {
	setting := &model.Setting{}
	var valueBytes []byte

	err := r.db.pool.QueryRow(ctx, `
		SELECT id, "key", value, description, created_at, updated_at
		FROM settings WHERE "key" = $1
	`, key).Scan(&setting.ID, &setting.Key, &valueBytes, &setting.Description, &setting.CreatedAt, &setting.UpdatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get setting: %w", err)
	}

	if err := json.Unmarshal(valueBytes, &setting.Value); err != nil {
		return nil, fmt.Errorf("unmarshal setting value: %w", err)
	}

	return setting, nil
}

// Upsert 插入或更新设置
func (r *SettingRepositoryImpl) Upsert(ctx context.Context, setting *model.Setting) error {
	if setting.ID == "" {
		setting.ID = uuid.New().String()
	}
	setting.UpdatedAt = time.Now()

	valueBytes, err := json.Marshal(setting.Value)
	if err != nil {
		return fmt.Errorf("marshal setting value: %w", err)
	}

	_, err = r.db.pool.Exec(ctx, `
		INSERT INTO settings (id, "key", value, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT ("key") DO UPDATE SET
			value = $3,
			updated_at = $6
	`, setting.ID, setting.Key, valueBytes, setting.Description, setting.CreatedAt, setting.UpdatedAt)

	if err != nil {
		return fmt.Errorf("upsert setting: %w", err)
	}

	return nil
}

// ListAll 获取所有设置
func (r *SettingRepositoryImpl) ListAll(ctx context.Context) ([]model.Setting, error) {
	rows, err := r.db.pool.Query(ctx, `
		SELECT id, "key", value, description, created_at, updated_at
		FROM settings ORDER BY "key"
	`)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	defer rows.Close()

	var settings []model.Setting
	for rows.Next() {
		var s model.Setting
		var valueBytes []byte
		if err := rows.Scan(&s.ID, &s.Key, &valueBytes, &s.Description, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		if err := json.Unmarshal(valueBytes, &s.Value); err != nil {
			return nil, fmt.Errorf("unmarshal setting value: %w", err)
		}
		settings = append(settings, s)
	}

	return settings, nil
}

// DeleteByKey 删除设置
func (r *SettingRepositoryImpl) DeleteByKey(ctx context.Context, key string) error {
	_, err := r.db.pool.Exec(ctx, `
		DELETE FROM settings WHERE "key" = $1
	`, key)

	if err != nil {
		return fmt.Errorf("delete setting: %w", err)
	}

	return nil
}

// GetHistory 获取设置变更历史
func (r *SettingRepositoryImpl) GetHistory(ctx context.Context, settingID string, limit int) ([]model.SettingHistory, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := r.db.pool.Query(ctx, `
		SELECT id, setting_id, old_value, new_value, change_reason, created_at
		FROM setting_history
		WHERE setting_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, settingID, limit)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	defer rows.Close()

	var history []model.SettingHistory
	for rows.Next() {
		var h model.SettingHistory
		var oldValueBytes, newValueBytes []byte
		if err := rows.Scan(&h.ID, &h.SettingID, &oldValueBytes, &newValueBytes, &h.ChangeReason, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		json.Unmarshal(oldValueBytes, &h.OldValue)
		json.Unmarshal(newValueBytes, &h.NewValue)
		history = append(history, h)
	}

	return history, nil
}
