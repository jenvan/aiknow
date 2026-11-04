package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jenvan/aiknow/internal/model"
)

// WeixinBotRepositoryImpl 微信 iLink Bot 账号仓库
type WeixinBotRepositoryImpl struct {
	db *PostgresDB
}

// NewWeixinBotRepository 创建仓库
func NewWeixinBotRepository(db *PostgresDB) *WeixinBotRepositoryImpl {
	return &WeixinBotRepositoryImpl{db: db}
}

// Get 取当前 bot 账号（v1 单 bot：最近更新的一行）。无记录返回 (nil, nil)。
//
// 返回 nil 而非 sql.ErrNoRows：「还没扫码」是正常状态，调用方据此提示
// 执行 `aiknow-server weixin login`，不应被当成错误往上抛。
func (r *WeixinBotRepositoryImpl) Get(ctx context.Context) (*model.WeixinBotAccount, error) {
	var a model.WeixinBotAccount
	err := r.db.pool.QueryRow(ctx, `
		SELECT bot_id, bot_token, base_url, owner_user_id, allow_from,
		       get_updates_cursor, last_context_token, last_seen_at, created_at, updated_at
		FROM weixin_bot_accounts
		ORDER BY updated_at DESC
		LIMIT 1
	`).Scan(&a.BotID, &a.BotToken, &a.BaseURL, &a.OwnerUserID, &a.AllowFrom,
		&a.Cursor, &a.LastContextToken, &a.LastSeenAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weixin bot account: %w", err)
	}
	return &a, nil
}

// SaveLogin 保存扫码得到的凭据（同 bot_id 覆盖）。
//
// 只在扫码时调用：续期 token 不该重置游标，所以这里显式保留已有游标。
func (r *WeixinBotRepositoryImpl) SaveLogin(ctx context.Context, a *model.WeixinBotAccount) error {
	now := time.Now()
	_, err := r.db.pool.Exec(ctx, `
		INSERT INTO weixin_bot_accounts (
			bot_id, bot_token, base_url, owner_user_id, allow_from, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (bot_id) DO UPDATE SET
			bot_token     = EXCLUDED.bot_token,
			base_url      = EXCLUDED.base_url,
			owner_user_id = EXCLUDED.owner_user_id,
			updated_at    = EXCLUDED.updated_at
	`, a.BotID, a.BotToken, a.BaseURL, a.OwnerUserID, a.AllowFrom, now)
	if err != nil {
		return fmt.Errorf("save weixin bot login: %w", err)
	}
	return nil
}

// UpdateProgress 推进消费位点（游标）并记录最近一次 context_token。
//
// 单独成方法而非整行 upsert：热路径每次轮询都会调用，绝不能碰到 bot_token。
func (r *WeixinBotRepositoryImpl) UpdateProgress(ctx context.Context, botID, cursor, contextToken string) error {
	_, err := r.db.pool.Exec(ctx, `
		UPDATE weixin_bot_accounts
		SET get_updates_cursor = $2,
		    last_context_token  = CASE WHEN $3 = '' THEN last_context_token ELSE $3 END,
		    last_seen_at = now(),
		    updated_at   = now()
		WHERE bot_id = $1
	`, botID, cursor, contextToken)
	if err != nil {
		return fmt.Errorf("update weixin bot progress: %w", err)
	}
	return nil
}

// UpdateAllowFrom 更新白名单（后台页面编辑）。
func (r *WeixinBotRepositoryImpl) UpdateAllowFrom(ctx context.Context, botID, allowFrom string) error {
	_, err := r.db.pool.Exec(ctx, `
		UPDATE weixin_bot_accounts
		SET allow_from = $2, updated_at = now()
		WHERE bot_id = $1
	`, botID, allowFrom)
	if err != nil {
		return fmt.Errorf("update weixin bot allow_from: %w", err)
	}
	return nil
}

// Delete 删除账号（重新登录 / 停用 bot 时使用）。
func (r *WeixinBotRepositoryImpl) Delete(ctx context.Context, botID string) error {
	_, err := r.db.pool.Exec(ctx, `DELETE FROM weixin_bot_accounts WHERE bot_id = $1`, botID)
	if err != nil {
		return fmt.Errorf("delete weixin bot account: %w", err)
	}
	return nil
}
