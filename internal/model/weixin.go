package model

import "time"

// WeixinBotAccount 是微信 iLink Bot 的账号记录：扫码得到的凭据 + 长轮询状态。
//
// 凭据与游标都落库（见 migrations/012_weixin_bot.sql 的注释）：
// bot_token 属 L3 凭据不进 .env，游标不落库则重启会重收消息。
type WeixinBotAccount struct {
	BotID    string `json:"bot_id" db:"bot_id"`
	BotToken string `json:"-" db:"bot_token"` // 凭据，永不出现在 API 响应里
	BaseURL  string `json:"base_url" db:"base_url"`
	// OwnerUserID 扫码者，也是**唯一被放行**的用户。
	OwnerUserID string `json:"owner_user_id" db:"owner_user_id"`

	// Cursor 是 get_updates_buf，长轮询的消费位点。
	Cursor string `json:"-" db:"get_updates_cursor"`
	// LastContextToken 是最近一条 inbound 消息的 context_token，
	// 主动补发结果时用它（被动回复则直接用当条消息的 token）。
	LastContextToken string `json:"-" db:"last_context_token"`

	LastSeenAt *time.Time `json:"last_seen_at" db:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
}
