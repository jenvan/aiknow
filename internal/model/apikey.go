package model

import "time"

// APIKey API Key 模型
type APIKey struct {
	ID           string     `json:"id" db:"id"`
	KeyHash      string     `json:"-" db:"key_hash"`
	KeyPrefix    string     `json:"key_prefix" db:"key_prefix"`
	Description  string     `json:"description" db:"description"`
	Scope        string     `json:"scope" db:"scope"`       // readonly | readwrite
	IsActive     bool       `json:"is_active" db:"is_active"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty" db:"last_used_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	CreatedBy    *string    `json:"created_by,omitempty" db:"created_by"`
}

// APIKeyCreateInput API Key 创建输入
type APIKeyCreateInput struct {
	Description string `json:"description" validate:"required"`
	Scope       string `json:"scope" validate:"oneof=readonly readwrite"`
}

// APIKeyPlaintext API Key 创建时返回的明文
type APIKeyPlaintext struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	KeyPrefix   string    `json:"key_prefix"`
	Description string    `json:"description"`
	Scope       string    `json:"scope"`
	CreatedAt   time.Time `json:"created_at"`
}
