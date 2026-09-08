package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jenvan/aiknow/internal/model"
)

// AuthConfig 认证服务配置
type AuthConfig struct {
	// Username 管理员用户名
	Username string
	// PasswordHash bcrypt 密码哈希（生产环境使用）
	PasswordHash string
	// Password 明文密码（仅开发环境；启动时在内存中哈希，不落盘）
	Password string
	// SessionLifetime Session 有效期
	SessionLifetime time.Duration
}

// session 已登录会话
type session struct {
	username  string
	expiresAt time.Time
}

// authServiceImpl 认证服务
type authServiceImpl struct {
	username     string
	passwordHash []byte
	lifetime     time.Duration

	sessions   map[string]session
	sessionsMu sync.RWMutex

	// 内存存储 API keys，生产环境应使用 DB
	apiKeys []*model.APIKey
	keysMu  sync.RWMutex
}

// DefaultSessionLifetime 默认 Session 有效期
const DefaultSessionLifetime = 24 * time.Hour

// NewAuthService 创建认证服务。
//
// 凭据来自 L1 配置：优先使用 PasswordHash；仅当其为空时，才将明文 Password
// 在内存中哈希后使用（开发便利，配置校验已禁止生产环境走这条路径）。
// 两者皆空将返回错误，避免服务以无法登录或凭据缺省的状态启动。
func NewAuthService(cfg AuthConfig) (*authServiceImpl, error) {
	if cfg.Username == "" {
		return nil, fmt.Errorf("管理员用户名不能为空")
	}

	var hash []byte
	switch {
	case cfg.PasswordHash != "":
		hash = []byte(cfg.PasswordHash)
		// 提前校验哈希格式，避免每次登录都因格式错误而失败
		if _, err := bcrypt.Cost(hash); err != nil {
			return nil, fmt.Errorf("ADMIN_PASSWORD_HASH 不是合法的 bcrypt 哈希: %w", err)
		}
	case cfg.Password != "":
		// bcrypt 只取前 72 字节，超长密码会被静默截断，此处显式拒绝
		if len(cfg.Password) > 72 {
			return nil, fmt.Errorf("明文密码过长（%d 字节），bcrypt 上限为 72 字节", len(cfg.Password))
		}
		generated, err := bcrypt.GenerateFromPassword([]byte(cfg.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("哈希明文密码失败: %w", err)
		}
		hash = generated
	default:
		return nil, fmt.Errorf("必须提供 ADMIN_PASSWORD_HASH 或 ADMIN_PASSWORD")
	}

	lifetime := cfg.SessionLifetime
	if lifetime <= 0 {
		lifetime = DefaultSessionLifetime
	}

	return &authServiceImpl{
		username:     cfg.Username,
		passwordHash: hash,
		lifetime:     lifetime,
		sessions:     make(map[string]session),
		apiKeys:      make([]*model.APIKey, 0),
	}, nil
}

// Login 校验管理员凭据并创建 Session
func (s *authServiceImpl) Login(ctx context.Context, username, password string) (string, error) {
	if username == "" || password == "" {
		return "", fmt.Errorf("用户名与密码不能为空")
	}

	// 用户名用恒定时间比较，避免通过响应时间推断用户名是否存在
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(s.username)) == 1
	// 无论用户名是否匹配都执行 bcrypt 比较，使失败路径耗时一致
	pwdErr := bcrypt.CompareHashAndPassword(s.passwordHash, []byte(password))

	if !userOK || pwdErr != nil {
		return "", fmt.Errorf("凭据无效")
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return "", fmt.Errorf("生成 session 失败: %w", err)
	}

	s.sessionsMu.Lock()
	s.sessions[sessionID] = session{
		username:  username,
		expiresAt: time.Now().Add(s.lifetime),
	}
	s.sessionsMu.Unlock()

	return sessionID, nil
}

// Logout 注销 Session
func (s *authServiceImpl) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session id 不能为空")
	}
	s.sessionsMu.Lock()
	delete(s.sessions, sessionID)
	s.sessionsMu.Unlock()
	return nil
}

// GetSession 获取 Session 对应的用户名，已过期视为无效
func (s *authServiceImpl) GetSession(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session id 不能为空")
	}

	s.sessionsMu.RLock()
	sess, ok := s.sessions[sessionID]
	s.sessionsMu.RUnlock()

	if !ok {
		return "", fmt.Errorf("session 不存在")
	}

	if time.Now().After(sess.expiresAt) {
		// 顺带清理已过期条目
		s.sessionsMu.Lock()
		if cur, still := s.sessions[sessionID]; still && cur.expiresAt.Equal(sess.expiresAt) {
			delete(s.sessions, sessionID)
		}
		s.sessionsMu.Unlock()
		return "", fmt.Errorf("session 已过期")
	}

	return sess.username, nil
}

// SessionLifetime 返回 Session 有效期（供 Cookie MaxAge 使用）
func (s *authServiceImpl) SessionLifetime() time.Duration {
	return s.lifetime
}

// ValidateKey 验证 API Key
func (s *authServiceImpl) ValidateKey(ctx context.Context, key string) (*model.APIKey, error) {
	if key == "" {
		return nil, nil
	}
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	s.keysMu.RLock()
	defer s.keysMu.RUnlock()

	for _, k := range s.apiKeys {
		if subtle.ConstantTimeCompare([]byte(k.KeyHash), []byte(keyHash)) == 1 {
			if !k.IsActive {
				return nil, fmt.Errorf("api key 已停用")
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

// generateSessionID 生成 256 位随机 session ID。
//
// 必须使用密码学安全随机源：此前的实现以 time.Now().UnixNano() 逐字节填充，
// 输出可被预测，攻击者能据此伪造他人 session。
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
