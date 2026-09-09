package service

import (
	"context"
	"testing"

	"github.com/jenvan/aiknow/internal/model"
)

// fakeAPIKeyRepo 内存实现，避免单测依赖数据库
type fakeAPIKeyRepo struct {
	keys      map[string]*model.APIKey // key: id
	byHash    map[string]*model.APIKey
	touched   []string
	createErr error
}

func newFakeAPIKeyRepo() *fakeAPIKeyRepo {
	return &fakeAPIKeyRepo{
		keys:   map[string]*model.APIKey{},
		byHash: map[string]*model.APIKey{},
	}
}

func (f *fakeAPIKeyRepo) Create(ctx context.Context, key *model.APIKey) error {
	if f.createErr != nil {
		return f.createErr
	}
	if key.ID == "" {
		key.ID = "id-" + key.KeyPrefix
	}
	// 原样存储，不再哈希——与真实仓库的契约一致
	cp := *key
	f.keys[key.ID] = &cp
	f.byHash[key.KeyHash] = &cp
	return nil
}

func (f *fakeAPIKeyRepo) GetByID(ctx context.Context, id string) (*model.APIKey, error) {
	return f.keys[id], nil
}

func (f *fakeAPIKeyRepo) GetByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	return f.byHash[hash], nil
}

func (f *fakeAPIKeyRepo) ListAll(ctx context.Context) ([]model.APIKey, error) {
	out := make([]model.APIKey, 0, len(f.keys))
	for _, k := range f.keys {
		out = append(out, *k)
	}
	return out, nil
}

func (f *fakeAPIKeyRepo) Update(ctx context.Context, key *model.APIKey) error {
	f.keys[key.ID] = key
	f.byHash[key.KeyHash] = key
	return nil
}

func (f *fakeAPIKeyRepo) TouchLastUsed(ctx context.Context, id string) error {
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeAPIKeyRepo) Delete(ctx context.Context, id string) error {
	if k := f.keys[id]; k != nil {
		delete(f.byHash, k.KeyHash)
	}
	delete(f.keys, id)
	return nil
}

func newAuthWithKeys(t *testing.T) (*authServiceImpl, *fakeAPIKeyRepo) {
	t.Helper()
	repo := newFakeAPIKeyRepo()
	s := newTestAuth(t, AuthConfig{
		PasswordHash: testHash(t, "pw"),
		APIKeyRepo:   repo,
	})
	return s, repo
}

// 回归：ValidateKey 此前只查内存切片，而其唯一写入方 AddAPIKey 全局零调用，
// 数据库中的 Key 从不生效，任何 Key 都返回 401
func TestValidateKeyReadsFromRepository(t *testing.T) {
	s, _ := newAuthWithKeys(t)
	ctx := context.Background()

	created, err := s.CreateKey(ctx, model.APIKeyCreateInput{
		Description: "测试",
		Scope:       ScopeReadWrite,
	}, "admin")
	if err != nil {
		t.Fatalf("创建 Key 失败: %v", err)
	}

	got, err := s.ValidateKey(ctx, created.Key)
	if err != nil {
		t.Fatalf("校验 Key 失败: %v", err)
	}
	if got == nil {
		t.Fatal("刚创建的 Key 校验返回 nil，说明未从仓库读取")
	}
	if got.Scope != ScopeReadWrite {
		t.Errorf("scope = %q, 期望 %q", got.Scope, ScopeReadWrite)
	}
}

// 明文不得落库，只存 SHA-256
func TestCreateKeyStoresHashNotPlaintext(t *testing.T) {
	s, repo := newAuthWithKeys(t)
	ctx := context.Background()

	created, err := s.CreateKey(ctx, model.APIKeyCreateInput{Description: "测试"}, "admin")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	stored := repo.keys[created.ID]
	if stored == nil {
		t.Fatal("仓库中找不到该 Key")
	}
	if stored.KeyHash == created.Key {
		t.Error("明文被直接存储，应存 SHA-256 哈希")
	}
	if stored.KeyHash != HashAPIKey(created.Key) {
		t.Error("存储的哈希与 HashAPIKey(明文) 不一致——服务层与仓库层的哈希契约不匹配")
	}
}

func TestValidateKeyRejectsUnknownAndInactive(t *testing.T) {
	s, repo := newAuthWithKeys(t)
	ctx := context.Background()

	if got, _ := s.ValidateKey(ctx, "ak_nonexistent"); got != nil {
		t.Error("未知 Key 应返回 nil")
	}

	created, _ := s.CreateKey(ctx, model.APIKeyCreateInput{Description: "停用测试"}, "admin")
	repo.keys[created.ID].IsActive = false

	if _, err := s.ValidateKey(ctx, created.Key); err == nil {
		t.Error("已停用的 Key 应返回错误")
	}
}

func TestCreateKeyValidatesScope(t *testing.T) {
	s, _ := newAuthWithKeys(t)
	ctx := context.Background()

	if _, err := s.CreateKey(ctx, model.APIKeyCreateInput{
		Description: "非法 scope", Scope: "superuser",
	}, "admin"); err == nil {
		t.Error("非法 scope 应被拒绝")
	}

	// 留空时取最小权限，与建表默认值一致
	created, err := s.CreateKey(ctx, model.APIKeyCreateInput{Description: "默认 scope"}, "admin")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if created.Scope != ScopeReadOnly {
		t.Errorf("默认 scope = %q, 期望 %q", created.Scope, ScopeReadOnly)
	}
}

// 缺仓库时必须报错，不能静默放行或静默拒绝
func TestValidateKeyWithoutRepositoryErrors(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})
	if _, err := s.ValidateKey(context.Background(), "ak_whatever"); err == nil {
		t.Error("未接线仓库时应返回错误")
	}
}

func TestScopeAllows(t *testing.T) {
	cases := []struct {
		granted, required string
		want              bool
	}{
		{ScopeReadOnly, ScopeReadOnly, true},
		{ScopeReadOnly, ScopeReadWrite, false}, // 只读不得写
		{ScopeReadWrite, ScopeReadOnly, true},  // 读写隐含只读
		{ScopeReadWrite, ScopeReadWrite, true},
		{"", ScopeReadOnly, false},
	}
	for _, c := range cases {
		if got := ScopeAllows(c.granted, c.required); got != c.want {
			t.Errorf("ScopeAllows(%q, %q) = %v, 期望 %v", c.granted, c.required, got, c.want)
		}
	}
}

// ---- CSRF ----

func TestCSRFTokenBoundToSession(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})
	ctx := context.Background()

	sid, err := s.Login(ctx, "admin", "pw")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	token, err := s.CSRFToken(ctx, sid)
	if err != nil {
		t.Fatalf("取 CSRF Token 失败: %v", err)
	}
	if len(token) != 64 {
		t.Errorf("Token 长度 = %d, 期望 64（32 字节 hex）", len(token))
	}

	if err := s.ValidateCSRF(ctx, sid, token); err != nil {
		t.Errorf("正确的 Token 校验失败: %v", err)
	}
	if err := s.ValidateCSRF(ctx, sid, "wrong"); err == nil {
		t.Error("错误的 Token 应被拒绝")
	}
	if err := s.ValidateCSRF(ctx, sid, ""); err == nil {
		t.Error("空 Token 应被拒绝")
	}
}

// 不同会话的 Token 必须互不通用，否则等同于没有 CSRF 防护
func TestCSRFTokenNotReusableAcrossSessions(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})
	ctx := context.Background()

	sid1, _ := s.Login(ctx, "admin", "pw")
	sid2, _ := s.Login(ctx, "admin", "pw")

	token1, _ := s.CSRFToken(ctx, sid1)
	token2, _ := s.CSRFToken(ctx, sid2)

	if token1 == token2 {
		t.Fatal("两个会话拿到了相同的 CSRF Token")
	}
	if err := s.ValidateCSRF(ctx, sid2, token1); err == nil {
		t.Error("会话 1 的 Token 在会话 2 上通过了校验")
	}
}

func TestCSRFAfterLogoutFails(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})
	ctx := context.Background()

	sid, _ := s.Login(ctx, "admin", "pw")
	token, _ := s.CSRFToken(ctx, sid)
	_ = s.Logout(ctx, sid)

	if err := s.ValidateCSRF(ctx, sid, token); err == nil {
		t.Error("登出后 Token 仍然有效")
	}
}

// ---- 登录失败锁定 ----

func TestLoginLockoutAfterMaxAttempts(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})
	ctx := context.Background()

	for i := 0; i < maxLoginAttempts; i++ {
		if _, err := s.Login(ctx, "admin", "wrong"); err == nil {
			t.Fatalf("第 %d 次错误密码竟然登录成功", i+1)
		}
	}

	// 锁定期内即使密码正确也应拒绝
	_, err := s.Login(ctx, "admin", "pw")
	if err == nil {
		t.Fatal("锁定期内用正确密码登录成功了")
	}
	if err != ErrAccountLocked {
		t.Errorf("错误 = %v, 期望 ErrAccountLocked", err)
	}
}

// 登录成功须清空失败计数，否则跨越多次会话的零星失败会累积到锁定
func TestSuccessfulLoginClearsFailureCount(t *testing.T) {
	s := newTestAuth(t, AuthConfig{PasswordHash: testHash(t, "pw")})
	ctx := context.Background()

	for i := 0; i < maxLoginAttempts-1; i++ {
		_, _ = s.Login(ctx, "admin", "wrong")
	}

	if _, err := s.Login(ctx, "admin", "pw"); err != nil {
		t.Fatalf("阈值前的正确登录失败: %v", err)
	}

	// 计数已清零，再来 maxLoginAttempts-1 次失败仍不应锁定
	for i := 0; i < maxLoginAttempts-1; i++ {
		_, _ = s.Login(ctx, "admin", "wrong")
	}
	if _, err := s.Login(ctx, "admin", "pw"); err != nil {
		t.Errorf("失败计数未在登录成功后清零: %v", err)
	}
}
