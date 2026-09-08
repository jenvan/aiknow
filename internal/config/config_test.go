package config

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withWorkDir 切换到临时工作目录，使 .env / config.yaml 的读取相互隔离
func withWorkDir(t *testing.T) string {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("切换目录失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

// clearManaged 清空所有受管键的环境变量，避免外部环境干扰
func clearManaged(t *testing.T) {
	t.Helper()
	for _, k := range managedKeys {
		if old, ok := os.LookupEnv(k); ok {
			t.Cleanup(func() { _ = os.Setenv(k, old) })
			_ = os.Unsetenv(k)
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(k) })
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("写入 %s 失败: %v", name, err)
	}
}

// 回归：PORT 与 APP_ENV 此前只出现在默认值分支，任何来源都无法覆盖
func TestPortAndAppEnvFromEnvVar(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("PORT", "9999")
	t.Setenv("APP_ENV", "production")

	c := NewConfig()

	if port, src := c.Get("PORT"); port != "9999" {
		t.Errorf("PORT = %q（来源 %v），期望 9999", port, src)
	}
	if env, src := c.Get("APP_ENV"); env != "production" {
		t.Errorf("APP_ENV = %q（来源 %v），期望 production", env, src)
	}
	if !c.IsProduction() {
		t.Error("IsProduction() 应为 true")
	}
}

func TestPortFromEnvFile(t *testing.T) {
	dir := withWorkDir(t)
	clearManaged(t)

	writeFile(t, dir, ".env", "PORT=7777\nAPP_ENV=staging\n")

	c := NewConfig()

	if port, src := c.Get("PORT"); port != "7777" || src != ConfigSourceEnvFile {
		t.Errorf("PORT = %q（来源 %v），期望 7777（来源 .env）", port, src)
	}
	if env, _ := c.Get("APP_ENV"); env != "staging" {
		t.Errorf("APP_ENV = %q，期望 staging", env)
	}
}

// 回归：config.yaml.example 使用嵌套结构，而旧解析器期望扁平键，导致整个 YAML 层失效
func TestNestedYAMLIsParsed(t *testing.T) {
	dir := withWorkDir(t)
	clearManaged(t)

	writeFile(t, dir, "config.yaml", `
database:
  url: "postgres://u:p@h:5432/db"
server:
  port: 6060
app:
  env: "production"
session:
  secret: "yaml-secret-that-is-long-enough-32ch"
  lifetime: "12h"
admin:
  username: "yamladmin"
  password_hash: "$2a$10$abcdefghijklmnopqrstuv"
ai:
  llm_provider: "qwen"
  embedding_provider: "baai"
  reranker_provider: "baai"
qwen:
  api_key: "qwen-key"
  base_url: "https://qwen.example.com"
baai:
  api_key: "baai-key"
  base_url: "https://baai.example.com"
`)

	c := NewConfig()

	cases := map[string]string{
		"DATABASE_URL":        "postgres://u:p@h:5432/db",
		"PORT":                "6060",
		"APP_ENV":             "production",
		"SESSION_SECRET":      "yaml-secret-that-is-long-enough-32ch",
		"SESSION_LIFETIME":    "12h",
		"ADMIN_USERNAME":      "yamladmin",
		"ADMIN_PASSWORD_HASH": "$2a$10$abcdefghijklmnopqrstuv",
		"LLM_PROVIDER":        "qwen",
		"EMBEDDING_PROVIDER":  "baai",
		"RERANKER_PROVIDER":   "baai",
		"QWEN_API_KEY":        "qwen-key",
		"QWEN_BASE_URL":       "https://qwen.example.com",
		"BAAI_API_KEY":        "baai-key",
		"BAAI_BASE_URL":       "https://baai.example.com",
	}

	for key, want := range cases {
		got, src := c.Get(key)
		if got != want {
			t.Errorf("%s = %q（来源 %v），期望 %q", key, got, src, want)
		}
		if src != ConfigSourceYAML {
			t.Errorf("%s 来源 = %v，期望 yaml", key, src)
		}
	}
}

// 优先级：环境变量 > .env > config.yaml > 默认值
func TestPrecedenceEnvVarOverEnvFileOverYAML(t *testing.T) {
	dir := withWorkDir(t)
	clearManaged(t)

	writeFile(t, dir, "config.yaml", "database:\n  url: \"from-yaml\"\nserver:\n  port: 1111\napp:\n  env: \"yaml-env\"\n")
	writeFile(t, dir, ".env", "DATABASE_URL=from-envfile\nPORT=2222\n")
	t.Setenv("DATABASE_URL", "from-envvar")

	c := NewConfig()

	// 三处都有 → 环境变量胜出
	if got, src := c.Get("DATABASE_URL"); got != "from-envvar" || src != ConfigSourceEnvVar {
		t.Errorf("DATABASE_URL = %q（来源 %v），期望 from-envvar（来源 env）", got, src)
	}
	// .env 与 yaml 都有 → .env 胜出
	if got, src := c.Get("PORT"); got != "2222" || src != ConfigSourceEnvFile {
		t.Errorf("PORT = %q（来源 %v），期望 2222（来源 .env）", got, src)
	}
	// 仅 yaml 有 → yaml 生效
	if got, src := c.Get("APP_ENV"); got != "yaml-env" || src != ConfigSourceYAML {
		t.Errorf("APP_ENV = %q（来源 %v），期望 yaml-env（来源 yaml）", got, src)
	}
}

func TestDefaultsWhenNothingProvided(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	c := NewConfig()

	cases := map[string]string{
		"PORT":             "8080",
		"APP_ENV":          "development",
		"ADMIN_USERNAME":   "admin",
		"SESSION_LIFETIME": "24h",
	}
	for key, want := range cases {
		got, src := c.Get(key)
		if got != want || src != ConfigSourceDefault {
			t.Errorf("%s = %q（来源 %v），期望 %q（来源 default）", key, got, src, want)
		}
	}
	if c.IsProduction() {
		t.Error("缺省应为 development，IsProduction() 应为 false")
	}
}

// YAML 中 port 未设置（零值）时不应覆盖默认值
func TestYAMLZeroPortDoesNotOverride(t *testing.T) {
	dir := withWorkDir(t)
	clearManaged(t)

	writeFile(t, dir, "config.yaml", "database:\n  url: \"x\"\n")

	c := NewConfig()

	if got, src := c.Get("PORT"); got != "8080" || src != ConfigSourceDefault {
		t.Errorf("PORT = %q（来源 %v），期望 8080（来源 default）", got, src)
	}
}

func TestGetDuration(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("SESSION_LIFETIME", "45m")
	c := NewConfig()

	if got := c.GetDuration("SESSION_LIFETIME", time.Hour); got != 45*time.Minute {
		t.Errorf("GetDuration = %v，期望 45m", got)
	}
	// 未设置的键回落到 fallback
	if got := c.GetDuration("NOT_SET_KEY", 3*time.Second); got != 3*time.Second {
		t.Errorf("未设置的键应回落 fallback，实际 %v", got)
	}
}

func TestGetDurationInvalidFallsBack(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("SESSION_LIFETIME", "不是时长")
	c := NewConfig()

	if got := c.GetDuration("SESSION_LIFETIME", time.Hour); got != time.Hour {
		t.Errorf("非法时长应回落 fallback，实际 %v", got)
	}
}

func TestGetOr(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	c := NewConfig()
	if got := c.GetOr("LLM_PROVIDER", "mock"); got != "mock" {
		t.Errorf("GetOr = %q，期望 mock", got)
	}

	t.Setenv("LLM_PROVIDER", "qwen")
	c2 := NewConfig()
	if got := c2.GetOr("LLM_PROVIDER", "mock"); got != "qwen" {
		t.Errorf("GetOr = %q，期望 qwen", got)
	}
}

func TestValidateRequiresDatabaseURL(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("SESSION_SECRET", "0123456789012345678901234567890123")
	t.Setenv("ADMIN_PASSWORD", "dev")

	if err := NewConfig().Validate(); err == nil {
		t.Fatal("缺少 DATABASE_URL 应报错")
	}
}

func TestValidateRejectsShortSessionSecret(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SESSION_SECRET", "too-short")
	t.Setenv("ADMIN_PASSWORD", "dev")

	err := NewConfig().Validate()
	if err == nil {
		t.Fatal("过短的 SESSION_SECRET 应报错")
	}
}

func TestValidateRequiresAdminCredential(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SESSION_SECRET", "0123456789012345678901234567890123")

	if err := NewConfig().Validate(); err == nil {
		t.Fatal("既无 ADMIN_PASSWORD_HASH 也无 ADMIN_PASSWORD 应报错")
	}
}

// 生产环境禁止明文密码
func TestValidateRejectsPlaintextPasswordInProduction(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SESSION_SECRET", "0123456789012345678901234567890123")
	t.Setenv("ADMIN_PASSWORD", "plaintext")
	t.Setenv("APP_ENV", "production")

	if err := NewConfig().Validate(); err == nil {
		t.Fatal("生产环境使用明文 ADMIN_PASSWORD 应报错")
	}
}

func TestValidatePassesWithHashInProduction(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SESSION_SECRET", "0123456789012345678901234567890123")
	t.Setenv("ADMIN_PASSWORD_HASH", "$2a$10$abcdefghijklmnopqrstuv")
	t.Setenv("APP_ENV", "production")

	if err := NewConfig().Validate(); err != nil {
		t.Fatalf("生产环境提供哈希应通过校验，实际报错: %v", err)
	}
}

func TestValidateRejectsInvalidSessionLifetime(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SESSION_SECRET", "0123456789012345678901234567890123")
	t.Setenv("ADMIN_PASSWORD", "dev")
	t.Setenv("SESSION_LIFETIME", "24小时")

	if err := NewConfig().Validate(); err == nil {
		t.Fatal("非法 SESSION_LIFETIME 应报错")
	}
}

// 确保示例配置文件与解析器结构保持一致（防止再次漂移）
func TestExampleYAMLMatchesParser(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(orig, "..", "..", "config.yaml.example"))
	if err != nil {
		t.Skipf("未找到 config.yaml.example: %v", err)
	}

	dir := withWorkDir(t)
	clearManaged(t)
	writeFile(t, dir, "config.yaml", string(data))

	c := NewConfig()

	// 示例文件中已填值的字段应能被解析到
	for _, key := range []string{
		"DATABASE_URL", "PORT", "APP_ENV", "SESSION_SECRET", "SESSION_LIFETIME",
		"ADMIN_USERNAME", "LLM_PROVIDER", "EMBEDDING_PROVIDER", "RERANKER_PROVIDER",
		"QWEN_BASE_URL", "BAAI_BASE_URL",
	} {
		if got, src := c.Get(key); got == "" || src != ConfigSourceYAML {
			t.Errorf("示例配置中的 %s 未被解析（值 %q，来源 %v）", key, got, src)
		}
	}
}

// ---- DatabaseURL：DSN 优先，缺省时由分项拼装 ----

func TestDatabaseURLPrefersDSN(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("DATABASE_URL", "postgres://dsn-wins@host:5432/db")
	t.Setenv("POSTGRES_PASSWORD", "ignored")
	t.Setenv("POSTGRES_DB", "ignored")

	got, _, err := NewConfig().DatabaseURL()
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if got != "postgres://dsn-wins@host:5432/db" {
		t.Errorf("DATABASE_URL 应优先，实际 %q", got)
	}
}

func TestDatabaseURLAssembledFromParts(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("POSTGRES_USER", "alice")
	t.Setenv("POSTGRES_PASSWORD", "pw")
	t.Setenv("POSTGRES_HOST", "db.internal")
	t.Setenv("POSTGRES_PORT", "6543")
	t.Setenv("POSTGRES_DB", "mydb")
	t.Setenv("POSTGRES_SSLMODE", "require")

	got, _, err := NewConfig().DatabaseURL()
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	want := "postgres://alice:pw@db.internal:6543/mydb?sslmode=require"
	if got != want {
		t.Errorf("拼装结果 = %q，期望 %q", got, want)
	}
}

// 分项缺省值：仅给密码即可拼出可用 DSN（compose 场景）
func TestDatabaseURLPartsDefaults(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("POSTGRES_PASSWORD", "secret")

	got, _, err := NewConfig().DatabaseURL()
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	want := "postgres://postgres:secret@postgres:5432/aiknow?sslmode=disable"
	if got != want {
		t.Errorf("拼装结果 = %q，期望 %q", got, want)
	}
}

// 密码含 DSN 保留字符时必须转义，否则连接串结构被破坏
func TestDatabaseURLEscapesSpecialCharsInPassword(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("POSTGRES_PASSWORD", "p@ss:w/rd?#x")

	got, _, err := NewConfig().DatabaseURL()
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}

	// 裸密码不应出现在结果中
	if strings.Contains(got, "p@ss:w/rd?#x") {
		t.Errorf("密码未被转义: %q", got)
	}
	// 解析回来应还原为原始密码，且主机名未被污染
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("拼装出的 DSN 无法解析: %v（%q）", err, got)
	}
	pw, _ := u.User.Password()
	if pw != "p@ss:w/rd?#x" {
		t.Errorf("解析回的密码 = %q，期望原值", pw)
	}
	if u.Host != "postgres:5432" {
		t.Errorf("Host = %q，密码中的字符污染了主机名", u.Host)
	}
	if u.Path != "/aiknow" {
		t.Errorf("Path = %q，期望 /aiknow", u.Path)
	}
}

func TestDatabaseURLErrorsWhenNothingProvided(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	if _, _, err := NewConfig().DatabaseURL(); err == nil {
		t.Fatal("既无 DATABASE_URL 也无 POSTGRES_PASSWORD 时应报错")
	}
}

// Validate 应接受「仅分项」这一形态（compose 部署）
func TestValidateAcceptsPostgresPartsOnly(t *testing.T) {
	withWorkDir(t)
	clearManaged(t)

	t.Setenv("POSTGRES_PASSWORD", "secret")
	t.Setenv("SESSION_SECRET", "0123456789012345678901234567890123")
	t.Setenv("ADMIN_PASSWORD", "dev")

	if err := NewConfig().Validate(); err != nil {
		t.Fatalf("仅提供分项应通过校验，实际报错: %v", err)
	}
}

// YAML 的 database 分项应被解析
func TestYAMLDatabasePartsParsed(t *testing.T) {
	dir := withWorkDir(t)
	clearManaged(t)

	writeFile(t, dir, "config.yaml", `
database:
  host: "yamlhost"
  port: 7777
  user: "yamluser"
  password: "yamlpw"
  name: "yamldb"
  sslmode: "verify-full"
`)

	got, _, err := NewConfig().DatabaseURL()
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	want := "postgres://yamluser:yamlpw@yamlhost:7777/yamldb?sslmode=verify-full"
	if got != want {
		t.Errorf("拼装结果 = %q，期望 %q", got, want)
	}
}

// YAML 中 database.port 为 0（未设置）时不应覆盖默认值
func TestYAMLZeroDatabasePortDoesNotOverride(t *testing.T) {
	dir := withWorkDir(t)
	clearManaged(t)

	writeFile(t, dir, "config.yaml", "database:\n  password: \"pw\"\n")

	if got, src := NewConfig().Get("POSTGRES_PORT"); got != "5432" || src != ConfigSourceDefault {
		t.Errorf("POSTGRES_PORT = %q（来源 %v），期望 5432（来源 default）", got, src)
	}
}
