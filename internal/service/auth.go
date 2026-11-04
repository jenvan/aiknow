package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
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
	// APIKeyRepo API Key 仓库。为空时 ValidateKey 一律拒绝，
	// 避免以"任何 Key 都无效"的静默状态运行。
	APIKeyRepo repository.APIKeyRepository
}

// session 已登录会话
type session struct {
	username  string
	csrfToken string
	expiresAt time.Time
}

// 登录失败锁定策略（AGENTS.md：5 次失败锁定 15 分钟）
const (
	maxLoginAttempts = 5
	lockoutDuration  = 15 * time.Minute
)

// loginAttempts 记录某用户名的连续失败情况
type loginAttempts struct {
	count       int
	lockedUntil time.Time
}

// APIKeyScope 取值
const (
	ScopeReadOnly  = "readonly"
	ScopeReadWrite = "readwrite"
)

// apiKeyPlaintextPrefix API Key 明文的固定前缀，便于在日志/代码库中识别误泄漏
const apiKeyPlaintextPrefix = "ak_"

// lastUsedThrottle last_used_at 的最小回写间隔。
// 每次请求都写库会让热点 Key 产生大量无谓 UPDATE。
const lastUsedThrottle = 1 * time.Minute

// authServiceImpl 认证服务
type authServiceImpl struct {
	username     string
	passwordHash []byte
	lifetime     time.Duration

	sessions   map[string]session
	sessionsMu sync.RWMutex

	attempts   map[string]*loginAttempts
	attemptsMu sync.Mutex

	apiKeyRepo repository.APIKeyRepository

	// lastUsed 记录各 Key 最近一次回写 last_used_at 的时间，用于节流
	lastUsed   map[string]time.Time
	lastUsedMu sync.Mutex
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
		return nil, fmt.Errorf("须提供 ADMIN_PASSWORD_HASH 或 ADMIN_PASSWORD")
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
		attempts:     make(map[string]*loginAttempts),
		apiKeyRepo:   cfg.APIKeyRepo,
		lastUsed:     make(map[string]time.Time),
	}, nil
}

// ErrAccountLocked 连续登录失败过多，账号临时锁定
var ErrAccountLocked = fmt.Errorf("账号已临时锁定，请稍后再试")

// Login 校验管理员凭据并创建 Session
func (s *authServiceImpl) Login(ctx context.Context, username, password string) (string, error) {
	if username == "" || password == "" {
		return "", fmt.Errorf("用户名与密码不能为空")
	}

	if locked := s.isLocked(username); locked {
		return "", ErrAccountLocked
	}

	// 用户名用恒定时间比较，避免通过响应时间推断用户名是否存在
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(s.username)) == 1
	// 无论用户名是否匹配都执行 bcrypt 比较，使失败路径耗时一致
	pwdErr := bcrypt.CompareHashAndPassword(s.passwordHash, []byte(password))

	if !userOK || pwdErr != nil {
		s.recordFailure(username)
		return "", fmt.Errorf("凭据无效")
	}

	sessionID, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("生成 session 失败: %w", err)
	}
	csrfToken, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("生成 csrf token 失败: %w", err)
	}

	s.sessionsMu.Lock()
	s.sessions[sessionID] = session{
		username:  username,
		csrfToken: csrfToken,
		expiresAt: time.Now().Add(s.lifetime),
	}
	s.sessionsMu.Unlock()

	s.clearFailures(username)

	return sessionID, nil
}

// isLocked 判断用户名当前是否处于锁定期
func (s *authServiceImpl) isLocked(username string) bool {
	s.attemptsMu.Lock()
	defer s.attemptsMu.Unlock()

	a, ok := s.attempts[username]
	if !ok {
		return false
	}
	if time.Now().Before(a.lockedUntil) {
		return true
	}
	// 锁定期已过，重新计数
	if !a.lockedUntil.IsZero() {
		delete(s.attempts, username)
	}
	return false
}

// recordFailure 累计一次失败，达到阈值则锁定
func (s *authServiceImpl) recordFailure(username string) {
	s.attemptsMu.Lock()
	defer s.attemptsMu.Unlock()

	a, ok := s.attempts[username]
	if !ok {
		a = &loginAttempts{}
		s.attempts[username] = a
	}
	a.count++
	if a.count >= maxLoginAttempts {
		a.lockedUntil = time.Now().Add(lockoutDuration)
		a.count = 0
	}
}

// clearFailures 登录成功后清除失败记录
func (s *authServiceImpl) clearFailures(username string) {
	s.attemptsMu.Lock()
	delete(s.attempts, username)
	s.attemptsMu.Unlock()
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
	sess, err := s.lookupSession(sessionID)
	if err != nil {
		return "", err
	}
	return sess.username, nil
}

// CSRFToken 返回 Session 绑定的 CSRF Token。
//
// 采用同步器令牌模式（token 存于服务端 Session）而非纯双提交 Cookie：
// 后者在存在子域名接管风险时可被写入伪造 Cookie 绕过。
func (s *authServiceImpl) CSRFToken(ctx context.Context, sessionID string) (string, error) {
	sess, err := s.lookupSession(sessionID)
	if err != nil {
		return "", err
	}
	return sess.csrfToken, nil
}

// ValidateCSRF 校验请求携带的 CSRF Token 是否与 Session 绑定值一致
func (s *authServiceImpl) ValidateCSRF(ctx context.Context, sessionID, token string) error {
	if token == "" {
		return fmt.Errorf("缺少 csrf token")
	}
	sess, err := s.lookupSession(sessionID)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(sess.csrfToken), []byte(token)) != 1 {
		return fmt.Errorf("csrf token 不匹配")
	}
	return nil
}

// lookupSession 读取并校验 Session 有效期
func (s *authServiceImpl) lookupSession(sessionID string) (session, error) {
	if sessionID == "" {
		return session{}, fmt.Errorf("session id 不能为空")
	}

	s.sessionsMu.RLock()
	sess, ok := s.sessions[sessionID]
	s.sessionsMu.RUnlock()

	if !ok {
		return session{}, fmt.Errorf("session 不存在")
	}

	if time.Now().After(sess.expiresAt) {
		// 顺带清理已过期条目
		s.sessionsMu.Lock()
		if cur, still := s.sessions[sessionID]; still && cur.expiresAt.Equal(sess.expiresAt) {
			delete(s.sessions, sessionID)
		}
		s.sessionsMu.Unlock()
		return session{}, fmt.Errorf("session 已过期")
	}

	return sess, nil
}

// SessionLifetime 返回 Session 有效期（供 Cookie MaxAge 使用）
func (s *authServiceImpl) SessionLifetime() time.Duration {
	return s.lifetime
}

// ValidateKey 验证 API Key。
//
// 按 SHA-256 哈希直接查库（`idx_api_keys_key_hash` 索引命中），
// 这正是 AGENTS.md 决策 #7 选择 SHA-256 而非 bcrypt 的原因：
// bcrypt 每条记录盐值不同，无法按哈希检索，只能全表遍历逐个比对。
//
// 此前实现只遍历内存中的 apiKeys 切片，而其唯一写入方 AddAPIKey 全局零调用，
// 导致数据库中的 Key 从不生效、任何 Key 都返回 401。
func (s *authServiceImpl) ValidateKey(ctx context.Context, key string) (*model.APIKey, error) {
	if key == "" {
		return nil, nil
	}
	if s.apiKeyRepo == nil {
		return nil, fmt.Errorf("api key 仓库未接线")
	}

	k, err := s.apiKeyRepo.GetByHash(ctx, HashAPIKey(key))
	if err != nil {
		return nil, fmt.Errorf("查询 api key: %w", err)
	}
	if k == nil {
		return nil, nil
	}
	if !k.IsActive {
		return nil, fmt.Errorf("api key 已停用")
	}

	s.touchLastUsed(ctx, k)
	return k, nil
}

// touchLastUsed 按节流间隔回写 last_used_at，失败不影响鉴权结果
func (s *authServiceImpl) touchLastUsed(ctx context.Context, k *model.APIKey) {
	now := time.Now()

	s.lastUsedMu.Lock()
	prev, ok := s.lastUsed[k.ID]
	if ok && now.Sub(prev) < lastUsedThrottle {
		s.lastUsedMu.Unlock()
		return
	}
	s.lastUsed[k.ID] = now
	s.lastUsedMu.Unlock()

	k.LastUsedAt = &now
	// 用独立 context：请求 context 可能在回写完成前被取消
	go func(id string) {
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.apiKeyRepo.TouchLastUsed(writeCtx, id); err != nil {
			fmt.Printf("警告: 回写 api key %s 的 last_used_at 失败: %v\n", id, err)
		}
	}(k.ID)
}

// CreateKey 创建 API Key，返回仅此一次可见的明文。
//
// 明文不落库，只存 SHA-256 哈希；调用方未妥善保存则只能重建。
func (s *authServiceImpl) CreateKey(
	ctx context.Context, input model.APIKeyCreateInput, createdBy string,
) (*model.APIKeyPlaintext, error) {
	if s.apiKeyRepo == nil {
		return nil, fmt.Errorf("api key 仓库未接线")
	}
	if strings.TrimSpace(input.Description) == "" {
		return nil, fmt.Errorf("描述不能为空")
	}

	scope := input.Scope
	if scope == "" {
		// 默认最小权限，与建表默认值保持一致
		scope = ScopeReadOnly
	}
	if scope != ScopeReadOnly && scope != ScopeReadWrite {
		return nil, fmt.Errorf("scope 必须为 %s 或 %s", ScopeReadOnly, ScopeReadWrite)
	}

	plaintext, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("生成 api key 失败: %w", err)
	}

	key := &model.APIKey{
		KeyHash: HashAPIKey(plaintext),
		// key_prefix 列宽 10，取明文前 9 位用于列表展示
		KeyPrefix:   plaintext[:9],
		Description: strings.TrimSpace(input.Description),
		Scope:       scope,
		IsActive:    true,
	}
	if createdBy != "" {
		key.CreatedBy = &createdBy
	}

	if err := s.apiKeyRepo.Create(ctx, key); err != nil {
		return nil, fmt.Errorf("保存 api key 失败: %w", err)
	}

	return &model.APIKeyPlaintext{
		ID:          key.ID,
		Key:         plaintext,
		KeyPrefix:   key.KeyPrefix,
		Description: key.Description,
		Scope:       key.Scope,
		CreatedAt:   key.CreatedAt,
	}, nil
}

// ListKeys 列出所有 API Key（不含哈希，模型上 KeyHash 的 json tag 为 "-"）
func (s *authServiceImpl) ListKeys(ctx context.Context) ([]model.APIKey, error) {
	if s.apiKeyRepo == nil {
		return nil, fmt.Errorf("api key 仓库未接线")
	}
	keys, err := s.apiKeyRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询 api key 列表失败: %w", err)
	}
	return keys, nil
}

// SetKeyActive 启用/停用 API Key
func (s *authServiceImpl) SetKeyActive(ctx context.Context, id string, active bool) error {
	if s.apiKeyRepo == nil {
		return fmt.Errorf("api key 仓库未接线")
	}

	key, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("查询 api key 失败: %w", err)
	}
	if key == nil {
		return ErrAPIKeyNotFound
	}

	key.IsActive = active
	if err := s.apiKeyRepo.Update(ctx, key); err != nil {
		return fmt.Errorf("更新 api key 失败: %w", err)
	}
	return nil
}

// DeleteKey 删除 API Key
func (s *authServiceImpl) DeleteKey(ctx context.Context, id string) error {
	if s.apiKeyRepo == nil {
		return fmt.Errorf("api key 仓库未接线")
	}

	key, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("查询 api key 失败: %w", err)
	}
	if key == nil {
		return ErrAPIKeyNotFound
	}

	if err := s.apiKeyRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("删除 api key 失败: %w", err)
	}

	s.lastUsedMu.Lock()
	delete(s.lastUsed, id)
	s.lastUsedMu.Unlock()

	return nil
}

// ErrAPIKeyNotFound 指定的 API Key 不存在，Handler 应映射为 404
var ErrAPIKeyNotFound = fmt.Errorf("api key 不存在")

// HashAPIKey 计算 API Key 的存储哈希。
//
// 唯一的哈希入口：仓库层不再自行哈希，避免"服务层已哈希、仓库层又哈希一次"
// 导致写入值与校验值不一致。
func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// ScopeAllows 判断已授予的 scope 是否满足所需权限。
// readwrite 隐含 readonly 的全部能力。
func ScopeAllows(granted, required string) bool {
	if granted == ScopeReadWrite {
		return true
	}
	return granted == required
}

// generateAPIKey 生成带前缀的 API Key 明文
func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return apiKeyPlaintextPrefix + hex.EncodeToString(b), nil
}

// generateToken 生成 256 位随机令牌（Session ID / CSRF Token 共用）。
//
// 必须使用密码学安全随机源：此前的实现以 time.Now().UnixNano() 逐字节填充，
// 输出可被预测，攻击者能据此伪造他人 session。
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
