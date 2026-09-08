package service

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// testHash 用最低 cost 生成哈希，避免测试因 bcrypt 计算而变慢
func testHash(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成测试哈希失败: %v", err)
	}
	return string(h)
}

func newTestAuth(t *testing.T, cfg AuthConfig) *authServiceImpl {
	t.Helper()
	if cfg.Username == "" {
		cfg.Username = "admin"
	}
	s, err := NewAuthService(cfg)
	if err != nil {
		t.Fatalf("创建认证服务失败: %v", err)
	}
	return s
}

// 回归：Login 此前硬编码 admin/admin，配置中的凭据从未生效
func TestLoginUsesConfiguredCredentials(t *testing.T) {
	s := newTestAuth(t, AuthConfig{
		Username:     "alice",
		PasswordHash: testHash(t, "s3cret"),
	})

	ctx := context.Background()

	if _, err := s.Login(ctx, "alice", "s3cret"); err != nil {
		t.Errorf("配置的凭据应能登录，实际报错: %v", err)
	}

	// 旧实现硬编码的 admin/admin 不应再被接受
	if _, err := s.Login(ctx, "admin", "admin"); err == nil {
		t.Error("硬编码的 admin/admin 不应能登录")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "correct")})

	if _, err := s.Login(context.Background(), "admin", "wrong"); err == nil {
		t.Error("密码错误应被拒绝")
	}
}

func TestLoginRejectsWrongUsername(t *testing.T) {
	s := newTestAuth(t, AuthConfig{Username: "alice", PasswordHash: testHash(t, "pw")})

	if _, err := s.Login(context.Background(), "bob", "pw"); err == nil {
		t.Error("用户名错误应被拒绝")
	}
}

func TestLoginRejectsEmptyInput(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})
	ctx := context.Background()

	if _, err := s.Login(ctx, "", "pw"); err == nil {
		t.Error("空用户名应被拒绝")
	}
	if _, err := s.Login(ctx, "admin", ""); err == nil {
		t.Error("空密码应被拒绝")
	}
}

// 明文密码在构造时哈希，登录时可正常校验
func TestPlaintextPasswordPath(t *testing.T) {
	s := newTestAuth(t, AuthConfig{Password: "devpass"})

	if _, err := s.Login(context.Background(), "admin", "devpass"); err != nil {
		t.Errorf("明文密码应能登录，实际报错: %v", err)
	}
	if _, err := s.Login(context.Background(), "admin", "devpas"); err == nil {
		t.Error("错误密码应被拒绝")
	}
}

// 哈希优先于明文
func TestPasswordHashTakesPrecedence(t *testing.T) {
	s := newTestAuth(t, AuthConfig{
		PasswordHash: testHash(t, "fromhash"),
		Password:     "fromplain",
	})
	ctx := context.Background()

	if _, err := s.Login(ctx, "admin", "fromhash"); err != nil {
		t.Errorf("应使用 PasswordHash，实际报错: %v", err)
	}
	if _, err := s.Login(ctx, "admin", "fromplain"); err == nil {
		t.Error("PasswordHash 存在时不应接受明文密码")
	}
}

func TestNewAuthServiceRejectsMissingCredential(t *testing.T) {
	if _, err := NewAuthService(AuthConfig{Username: "admin"}); err == nil {
		t.Error("哈希与明文都为空时应报错，而非以无法登录的状态启动")
	}
}

func TestNewAuthServiceRejectsEmptyUsername(t *testing.T) {
	if _, err := NewAuthService(AuthConfig{PasswordHash: testHash(t, "pw")}); err == nil {
		t.Error("空用户名应报错")
	}
}

func TestNewAuthServiceRejectsMalformedHash(t *testing.T) {
	if _, err := NewAuthService(AuthConfig{
		Username:     "admin",
		PasswordHash: "not-a-bcrypt-hash",
	}); err == nil {
		t.Error("非法 bcrypt 哈希应在启动时报错，而非每次登录都失败")
	}
}

// bcrypt 只取前 72 字节，超长密码会被静默截断，应显式拒绝
func TestNewAuthServiceRejectsOverlongPlaintext(t *testing.T) {
	if _, err := NewAuthService(AuthConfig{
		Username: "admin",
		Password: strings.Repeat("a", 73),
	}); err == nil {
		t.Error("超过 72 字节的明文密码应报错")
	}
}

// 回归：旧实现以 time.Now().UnixNano() 逐字节填充，session ID 可预测
func TestSessionIDIsUnpredictable(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})
	ctx := context.Background()

	const n = 200
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		id, err := s.Login(ctx, "admin", "pw")
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}
		if seen[id] {
			t.Fatalf("第 %d 次登录出现重复 session ID", i)
		}
		seen[id] = true
	}

	// 连续两次生成的 ID 应几乎完全不同；旧实现仅个别字节递增
	a, _ := s.Login(ctx, "admin", "pw")
	b, _ := s.Login(ctx, "admin", "pw")
	ba, err := hex.DecodeString(a)
	if err != nil {
		t.Fatalf("session ID 不是合法 hex: %v", err)
	}
	bb, _ := hex.DecodeString(b)

	if len(ba) != 32 {
		t.Errorf("session ID 应为 32 字节（256 位），实际 %d", len(ba))
	}

	same := 0
	for i := range ba {
		if ba[i] == bb[i] {
			same++
		}
	}
	// 随机情况下期望约 1/256 字节相同；超过一半必然不是随机源
	if same > len(ba)/2 {
		t.Errorf("连续两个 session ID 有 %d/%d 字节相同，随机性不足", same, len(ba))
	}
}

func TestGetSessionReturnsUsername(t *testing.T) {
	s := newTestAuth(t, AuthConfig{Username: "alice", PasswordHash: testHash(t, "pw")})
	ctx := context.Background()

	id, err := s.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	user, err := s.GetSession(ctx, id)
	if err != nil {
		t.Fatalf("获取 session 失败: %v", err)
	}
	if user != "alice" {
		t.Errorf("session 用户名 = %q，期望 alice", user)
	}
}

func TestGetSessionRejectsUnknownID(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})

	if _, err := s.GetSession(context.Background(), "nonexistent"); err == nil {
		t.Error("未知 session ID 应被拒绝")
	}
	if _, err := s.GetSession(context.Background(), ""); err == nil {
		t.Error("空 session ID 应被拒绝")
	}
}

// Session 过期后不再有效（旧实现没有过期概念）
func TestSessionExpires(t *testing.T) {
	s := newTestAuth(t, AuthConfig{
		PasswordHash:    testHash(t, "pw"),
		SessionLifetime: 30 * time.Millisecond,
	})
	ctx := context.Background()

	id, err := s.Login(ctx, "admin", "pw")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	if _, err := s.GetSession(ctx, id); err != nil {
		t.Fatalf("刚创建的 session 应有效: %v", err)
	}

	time.Sleep(60 * time.Millisecond)

	if _, err := s.GetSession(ctx, id); err == nil {
		t.Error("已过期的 session 应失效")
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})
	ctx := context.Background()

	id, _ := s.Login(ctx, "admin", "pw")
	if err := s.Logout(ctx, id); err != nil {
		t.Fatalf("登出失败: %v", err)
	}

	if _, err := s.GetSession(ctx, id); err == nil {
		t.Error("登出后 session 应失效")
	}
}

func TestDefaultSessionLifetimeApplied(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})

	if got := s.SessionLifetime(); got != DefaultSessionLifetime {
		t.Errorf("SessionLifetime = %v，期望默认值 %v", got, DefaultSessionLifetime)
	}
}

func TestNegativeLifetimeFallsBackToDefault(t *testing.T) {
	s := newTestAuth(t, AuthConfig{
		PasswordHash:    testHash(t, "pw"),
		SessionLifetime: -time.Hour,
	})

	if got := s.SessionLifetime(); got != DefaultSessionLifetime {
		t.Errorf("非正有效期应回落默认值，实际 %v", got)
	}
}
