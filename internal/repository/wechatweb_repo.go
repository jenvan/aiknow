package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jenvan/aiknow/internal/model"
)

// WechatWebRepositoryImpl 文件传输助手会话仓库（v1 单账号，固定 id='default'）。
type WechatWebRepositoryImpl struct {
	db *PostgresDB
}

// NewWechatWebRepository 创建仓库。
func NewWechatWebRepository(db *PostgresDB) *WechatWebRepositoryImpl {
	return &WechatWebRepositoryImpl{db: db}
}

// Get 取会话；未登录返回 (nil, nil)。
func (r *WechatWebRepositoryImpl) Get(ctx context.Context) (*model.WechatWebSession, error) {
	var s model.WechatWebSession
	err := r.db.pool.QueryRow(ctx, `
		SELECT id, creds_enc, user_name, sync_key, last_sync_at, created_at, updated_at
		FROM wechatweb_sessions WHERE id = 'default'
	`).Scan(&s.ID, &s.CredsEnc, &s.UserName, &s.SyncKey, &s.LastSyncAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get wechatweb session: %w", err)
	}
	return &s, nil
}

// Save 保存（覆盖）会话凭据与初始游标。登录时调用一次。
func (r *WechatWebRepositoryImpl) Save(ctx context.Context, credsEnc []byte, userName string, syncKey []byte) error {
	_, err := r.db.pool.Exec(ctx, `
		INSERT INTO wechatweb_sessions (id, creds_enc, user_name, sync_key, updated_at)
		VALUES ('default', $1, $2, $3, now())
		ON CONFLICT (id) DO UPDATE SET
			creds_enc  = EXCLUDED.creds_enc,
			user_name  = EXCLUDED.user_name,
			sync_key   = EXCLUDED.sync_key,
			updated_at = now()
	`, credsEnc, userName, syncKey)
	if err != nil {
		return fmt.Errorf("save wechatweb session: %w", err)
	}
	return nil
}

// UpdateSyncKey 推进同步游标。
//
// 热路径（每收到一批消息调一次），只碰 sync_key / last_sync_at，不碰凭据。
func (r *WechatWebRepositoryImpl) UpdateSyncKey(ctx context.Context, syncKey []byte) error {
	_, err := r.db.pool.Exec(ctx, `
		UPDATE wechatweb_sessions
		SET sync_key = $1, last_sync_at = now(), updated_at = now()
		WHERE id = 'default'
	`, syncKey)
	if err != nil {
		return fmt.Errorf("update wechatweb sync key: %w", err)
	}
	return nil
}

// Touch 只更新最近活跃时间（用于状态展示）。
func (r *WechatWebRepositoryImpl) Touch(ctx context.Context) error {
	_, err := r.db.pool.Exec(ctx,
		`UPDATE wechatweb_sessions SET last_sync_at = now() WHERE id = 'default'`)
	if err != nil {
		return fmt.Errorf("touch wechatweb session: %w", err)
	}
	return nil
}

// Delete 删除会话（登出）。
func (r *WechatWebRepositoryImpl) Delete(ctx context.Context) error {
	_, err := r.db.pool.Exec(ctx, `DELETE FROM wechatweb_sessions WHERE id = 'default'`)
	if err != nil {
		return fmt.Errorf("delete wechatweb session: %w", err)
	}
	return nil
}
