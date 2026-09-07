package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/aiknow-org/aiknow/internal/model"
)

// authServiceImpl 认证服务
type authServiceImpl struct {
	// 内存存储 sessions，生产环境应使用 Redis/DB
	sessions  map[string]string
	sessionsMu sync.RWMutex
	// 内存存储 API keys，生产环境应使用 DB
	apiKeys  []*model.APIKey
	keysMu   sync.RWMutex
}

// NewAuthService 创建认证服务
func NewAuthService() *authServiceImpl {
	return &authServiceImpl{
		sessions: make(map[string]string),
		apiKeys:  make([]*model.APIKey, 0),
	}
}

// Login 登录
func (s *authServiceImpl) Login(ctx context.Context, username, password string) (string, error) {
	if username == "" || password == "" {
		return "", fmt.Errorf("username and password are required")
	}
	// TODO: 实际生产环境应从数据库验证凭据
	if username != "admin" || password != "admin" {
		return "", fmt.Errorf("invalid credentials")
	}
	sessionID := generateSessionID()
	s.sessionsMu.Lock()
	s.sessions[sessionID] = username
	s.sessionsMu.Unlock()
	return sessionID, nil
}

// Logout 登出
func (s *authServiceImpl) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	s.sessionsMu.Lock()
	delete(s.sessions, sessionID)
	s.sessionsMu.Unlock()
	return nil
}

// GetSession 获取 Session
func (s *authServiceImpl) GetSession(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	s.sessionsMu.RLock()
	username, ok := s.sessions[sessionID]
	s.sessionsMu.RUnlock()
	if !ok {
		return "", fmt.Errorf("session not found")
	}
	return username, nil
}

// ValidateKey 验证 API Key
func (s *authServiceImpl) ValidateKey(ctx context.Context, key string) (*model.APIKey, error) {
	if key == "" {
		return nil, nil
	}
	// 计算 key 的 hash
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	s.keysMu.RLock()
	defer s.keysMu.RUnlock()

	for _, k := range s.apiKeys {
		if k.KeyHash == keyHash {
			if !k.IsActive {
				return nil, fmt.Errorf("api key is inactive")
			}
			now := time.Now()
			k.LastUsedAt = &now
			return k, nil
		}
	}
	return nil, nil
}

// AddAPIKey 添加 API Key（用于测试/初始化）
// plaintext 为原始密钥明文，会被 hash 后存储
func (s *authServiceImpl) AddAPIKey(key *model.APIKey, plaintext string) {
	hash := sha256.Sum256([]byte(plaintext))
	key.KeyHash = hex.EncodeToString(hash[:])
	s.keysMu.Lock()
	s.apiKeys = append(s.apiKeys, key)
	s.keysMu.Unlock()
}

// generateSessionID 生成随机 session ID
func generateSessionID() string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(time.Now().UnixNano() + int64(i))
	}
	return hex.EncodeToString(b)
}
