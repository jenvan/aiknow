package model

import "time"

// WechatWebSession 微信文件传输助手（网页版协议）的会话记录。
//
// 凭据是密文（AES-GCM），由 service 层加解密；仓库层只负责存取，不解密、不打日志。
type WechatWebSession struct {
	ID string `json:"id" db:"id"`
	// CredsEnc 加密后的凭据 JSON（wechatweb.Token）
	CredsEnc []byte `json:"-" db:"creds_enc"`
	// UserName 自己的 UserName，仅留档
	UserName string `json:"user_name" db:"user_name"`
	// SyncKey 同步游标（JSON），丢失后重登只能收到新消息
	SyncKey    []byte     `json:"-" db:"sync_key"`
	LastSyncAt *time.Time `json:"last_sync_at" db:"last_sync_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
}
