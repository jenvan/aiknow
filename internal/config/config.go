package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// managedKeys 本配置管理器接管的全部键。
//
// 三个加载来源（.env / config.yaml / 环境变量）共用此清单，新增配置项只需在
// 此处登记一次。此前各来源各自维护一份硬编码列表，导致 PORT 与 APP_ENV 只在
// 默认值分支出现、任何来源都无法覆盖。
var managedKeys = []string{
	// 数据库：DATABASE_URL 优先；缺省时由下列分项拼装
	"DATABASE_URL",
	"POSTGRES_HOST",
	"POSTGRES_PORT",
	"POSTGRES_USER",
	"POSTGRES_PASSWORD",
	"POSTGRES_DB",
	"POSTGRES_SSLMODE",

	// 服务器
	"PORT",
	"APP_ENV",

	// 会话
	"SESSION_SECRET",
	"SESSION_LIFETIME",

	// 管理员
	"ADMIN_USERNAME",
	"ADMIN_PASSWORD",
	"ADMIN_PASSWORD_HASH",

	// AI Provider 选择
	"LLM_PROVIDER",
	"EMBEDDING_PROVIDER",
	"RERANKER_PROVIDER",

	// AI 服务凭据
	"QWEN_API_KEY",
	"QWEN_BASE_URL",
	"QWEN_MODEL",
	"BAAI_API_KEY",
	"BAAI_BASE_URL",
	"BAAI_EMBEDDING_MODEL",
	"BAAI_RERANKER_MODEL",

	// 网页采集
	"COLLECTOR_USER_AGENT",
	"COLLECTOR_TIMEOUT",
	"COLLECTOR_MAX_REDIRECTS",
	"COLLECTOR_MIN_CONTENT_LENGTH",

	// 无头浏览器渲染（采集 SPA 页面的 fallback）
	"COLLECTOR_BROWSER_ENABLED",
	"COLLECTOR_BROWSER_ENDPOINT",
	"COLLECTOR_BROWSER_TOKEN",
	"COLLECTOR_BROWSER_TIMEOUT",

	// 腾讯云 COS（采集文章截图归档，防源站失效）
	"COS_SECRET_ID",
	"COS_SECRET_KEY",
	"COS_BUCKET",
	"COS_REGION",
	"COS_ENDPOINT",
	"COS_PATH_PREFIX",
}

// defaultValues 任何来源都未提供时的兜底值
var defaultValues = map[string]string{
	"PORT":             "8080",
	"APP_ENV":          "development",
	"ADMIN_USERNAME":   "admin",
	"SESSION_LIFETIME": "24h",

	// 数据库分项的缺省值。
	// HOST 默认为 postgres：分项拼装主要服务于 docker compose 部署，
	// 那里应用通过服务名访问数据库。直跑二进制通常直接给 DATABASE_URL。
	"POSTGRES_HOST":    "postgres",
	"POSTGRES_PORT":    "5432",
	"POSTGRES_USER":    "postgres",
	"POSTGRES_DB":      "aiknow",
	"POSTGRES_SSLMODE": "disable",

	"QWEN_MODEL": "qwen-plus",

	// BAAI 模型名默认沿用项目约定的 bge 系列；留空时 provider 内部兜底
	"BAAI_EMBEDDING_MODEL": "BAAI/bge-large-zh-v1.5",
	"BAAI_RERANKER_MODEL":  "BAAI/bge-reranker-v2-m3",

	// 网页采集默认值（与 internal/service/collector.go DefaultCollectorConfig 一致）
	"COLLECTOR_TIMEOUT":            "10s",
	"COLLECTOR_MAX_REDIRECTS":      "3",
	"COLLECTOR_MIN_CONTENT_LENGTH": "200",
	"COLLECTOR_USER_AGENT":         "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",

	// 无头浏览器渲染默认值（与 internal/service/collector.go 渲染配置一致）
	// ENABLED 是总开关（默认 false）；ENDPOINT 缺省为空。二者都满足才真正渲染，
	// 否则采集退化为纯 http。
	"COLLECTOR_BROWSER_ENABLED":  "false",
	"COLLECTOR_BROWSER_ENDPOINT": "",
	"COLLECTOR_BROWSER_TOKEN":    "",
	"COLLECTOR_BROWSER_TIMEOUT":  "30s",

	// 腾讯云 COS：缺省全空，未配置时不启用截图归档（不影响采集正文）。
	"COS_SECRET_ID":   "",
	"COS_SECRET_KEY":  "",
	"COS_BUCKET":      "",
	"COS_REGION":      "",
	"COS_ENDPOINT":    "",
	"COS_PATH_PREFIX": "screenshots",
}

// YAMLConfig config.yaml 的结构。
//
// 采用嵌套形式，与 config.yaml.example 保持一致；解析后由 toFlat 映射为扁平键。
type YAMLConfig struct {
	Database struct {
		// URL 完整连接串，优先于下列分项
		URL string `yaml:"url"`
		// 以下分项在 URL 为空时用于拼装连接串
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Name     string `yaml:"name"`
		SSLMode  string `yaml:"sslmode"`
	} `yaml:"database"`

	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`

	App struct {
		Env string `yaml:"env"`
	} `yaml:"app"`

	Session struct {
		Secret   string `yaml:"secret"`
		Lifetime string `yaml:"lifetime"`
	} `yaml:"session"`

	Admin struct {
		Username     string `yaml:"username"`
		Password     string `yaml:"password"`
		PasswordHash string `yaml:"password_hash"`
	} `yaml:"admin"`

	AI struct {
		LLMProvider       string `yaml:"llm_provider"`
		EmbeddingProvider string `yaml:"embedding_provider"`
		RerankerProvider  string `yaml:"reranker_provider"`
	} `yaml:"ai"`

	Qwen struct {
		APIKey  string `yaml:"api_key"`
		BaseURL string `yaml:"base_url"`
		Model   string `yaml:"model"`
	} `yaml:"qwen"`

	BAAI struct {
		APIKey         string `yaml:"api_key"`
		BaseURL        string `yaml:"base_url"`
		EmbeddingModel string `yaml:"embedding_model"`
		RerankerModel  string `yaml:"reranker_model"`
	} `yaml:"baai"`

	Collector struct {
		Timeout          string `yaml:"timeout"`
		MaxRedirects     int    `yaml:"max_redirects"`
		MinContentLength int    `yaml:"min_content_length"`
		UserAgent        string `yaml:"user_agent"`
	} `yaml:"collector"`

	// 无头浏览器渲染（采集 SPA 页面的 fallback）
	Render struct {
		// 总开关。对应 env COLLECTOR_BROWSER_ENABLED，默认 false。
		// 需与 endpoint 同时满足才真正渲染
		Enabled bool `yaml:"enabled"`
		// browserless 端点，如 http://1.2.3.4:3000。缺省为空则渲染关闭
		Endpoint string `yaml:"endpoint"`
		// browserless 访问令牌。对应 env COLLECTOR_BROWSER_TOKEN
		Token string `yaml:"token"`
		// 单页渲染超时。对应 env COLLECTOR_BROWSER_TIMEOUT，默认 30s
		Timeout string `yaml:"timeout"`
	} `yaml:"render"`

	// 腾讯云 COS（采集文章截图归档）
	COS struct {
		// 密钥。对应 env COS_SECRET_ID / COS_SECRET_KEY
		SecretID  string `yaml:"secret_id"`
		SecretKey string `yaml:"secret_key"`
		// 存储桶名（含 appid 后缀），如 aiknow-1300000000。对应 env COS_BUCKET
		Bucket string `yaml:"bucket"`
		// 地域，如 ap-guangzhou。对应 env COS_REGION
		Region string `yaml:"region"`
		// 自定义访问域名（CDN）；为空用 COS 默认域名。对应 env COS_ENDPOINT
		Endpoint string `yaml:"endpoint"`
		// 对象键前缀，默认 screenshots。对应 env COS_PATH_PREFIX
		PathPrefix string `yaml:"path_prefix"`
	} `yaml:"cos"`
}

// toFlat 将嵌套结构映射为扁平配置键；空值不参与映射，交由下一优先级来源提供
func (y *YAMLConfig) toFlat() map[string]string {
	flat := map[string]string{
		"DATABASE_URL":         y.Database.URL,
		"POSTGRES_HOST":        y.Database.Host,
		"POSTGRES_USER":        y.Database.User,
		"POSTGRES_PASSWORD":    y.Database.Password,
		"POSTGRES_DB":          y.Database.Name,
		"POSTGRES_SSLMODE":     y.Database.SSLMode,
		"APP_ENV":              y.App.Env,
		"SESSION_SECRET":       y.Session.Secret,
		"SESSION_LIFETIME":     y.Session.Lifetime,
		"ADMIN_USERNAME":       y.Admin.Username,
		"ADMIN_PASSWORD":       y.Admin.Password,
		"ADMIN_PASSWORD_HASH":  y.Admin.PasswordHash,
		"LLM_PROVIDER":         y.AI.LLMProvider,
		"EMBEDDING_PROVIDER":   y.AI.EmbeddingProvider,
		"RERANKER_PROVIDER":    y.AI.RerankerProvider,
		"QWEN_API_KEY":         y.Qwen.APIKey,
		"QWEN_BASE_URL":        y.Qwen.BaseURL,
		"QWEN_MODEL":           y.Qwen.Model,
		"BAAI_API_KEY":         y.BAAI.APIKey,
		"BAAI_BASE_URL":        y.BAAI.BaseURL,
		"BAAI_EMBEDDING_MODEL": y.BAAI.EmbeddingModel,
		"BAAI_RERANKER_MODEL":  y.BAAI.RerankerModel,
		"COLLECTOR_USER_AGENT": y.Collector.UserAgent,
		"COLLECTOR_TIMEOUT":    y.Collector.Timeout,
		// 无头浏览器渲染
		"COLLECTOR_BROWSER_ENDPOINT": y.Render.Endpoint,
		"COLLECTOR_BROWSER_TOKEN":    y.Render.Token,
		"COLLECTOR_BROWSER_TIMEOUT":  y.Render.Timeout,
		// 腾讯云 COS
		"COS_SECRET_ID":   y.COS.SecretID,
		"COS_SECRET_KEY":  y.COS.SecretKey,
		"COS_BUCKET":      y.COS.Bucket,
		"COS_REGION":      y.COS.Region,
		"COS_ENDPOINT":    y.COS.Endpoint,
		"COS_PATH_PREFIX": y.COS.PathPrefix,
	}

	if y.Collector.MaxRedirects != 0 {
		flat["COLLECTOR_MAX_REDIRECTS"] = strconv.Itoa(y.Collector.MaxRedirects)
	}
	if y.Collector.MinContentLength != 0 {
		flat["COLLECTOR_MIN_CONTENT_LENGTH"] = strconv.Itoa(y.Collector.MinContentLength)
	}
	// 布尔开关：即使 false 也要写入，以允许在 yaml 中显式关闭浏览器
	flat["COLLECTOR_BROWSER_ENABLED"] = strconv.FormatBool(y.Render.Enabled)

	// 0 表示 YAML 中未设置该项，不写入以免覆盖更低优先级来源
	if y.Server.Port != 0 {
		flat["PORT"] = strconv.Itoa(y.Server.Port)
	}
	if y.Database.Port != 0 {
		flat["POSTGRES_PORT"] = strconv.Itoa(y.Database.Port)
	}

	for k, v := range flat {
		// 布尔开关即使为 "false" 也需保留（显式在 yaml 中关闭），
		// 其余空值不写入以免覆盖更低优先级来源
		if v == "" && k != "COLLECTOR_BROWSER_ENABLED" {
			delete(flat, k)
		}
	}
	return flat
}

// ConfigSource 配置来源
type ConfigSource int

const (
	ConfigSourceDefault ConfigSource = iota
	ConfigSourceYAML
	ConfigSourceEnvFile
	ConfigSourceEnvVar
)

func (s ConfigSource) String() string {
	switch s {
	case ConfigSourceDefault:
		return "default"
	case ConfigSourceYAML:
		return "yaml"
	case ConfigSourceEnvFile:
		return ".env"
	case ConfigSourceEnvVar:
		return "env"
	default:
		return "unknown"
	}
}

// ConfigValue 带来源的配置值
type ConfigValue struct {
	Value  string
	Source ConfigSource
}

// Config 配置管理器
//
// 优先级：环境变量 > .env > config.yaml > 默认值
type Config struct {
	mu     sync.RWMutex
	values map[string]ConfigValue
}

// NewConfig 创建配置管理器
func NewConfig() *Config {
	c := &Config{values: make(map[string]ConfigValue)}

	// 在加载 .env 之前快照真实环境变量：godotenv.Load 会把文件内容注入进程环境，
	// 之后便无法区分某个值究竟来自环境变量还是 .env
	preexisting := make(map[string]string, len(managedKeys))
	for _, key := range managedKeys {
		if val, ok := os.LookupEnv(key); ok && val != "" {
			preexisting[key] = val
		}
	}

	// 按优先级由低到高加载，后者覆盖前者
	c.loadYAML()
	c.loadEnvFile()
	c.applyEnvVars(preexisting)

	return c
}

// loadYAML 加载 config.yaml（最低优先级，默认值除外）
func (c *Config) loadYAML() {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return // 不存在则跳过
	}

	var cfg YAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("警告: 解析 config.yaml 失败: %v\n", err)
		return
	}

	for key, val := range cfg.toFlat() {
		c.values[key] = ConfigValue{Value: val, Source: ConfigSourceYAML}
	}
}

// loadEnvFile 加载 .env 文件
//
// 用 Read 单独取得文件内容以准确判断来源；同时调用 Load 将其注入进程环境，
// 保持既有行为（第三方库可能直接读 os.Getenv）。Load 不覆盖已存在的环境变量。
func (c *Config) loadEnvFile() {
	if _, err := os.Stat(".env"); err != nil {
		return // .env 不存在，静默跳过
	}

	fileVals, err := godotenv.Read()
	if err != nil {
		fmt.Printf("警告: 读取 .env 失败: %v\n", err)
		return
	}
	if err := godotenv.Load(); err != nil {
		fmt.Printf("警告: 加载 .env 失败: %v\n", err)
	}

	for _, key := range managedKeys {
		if val, ok := fileVals[key]; ok && val != "" {
			c.values[key] = ConfigValue{Value: val, Source: ConfigSourceEnvFile}
		}
	}
}

// applyEnvVars 应用真实环境变量（最高优先级）
//
// preexisting 为加载 .env 之前的环境变量快照，避免把 .env 注入的值误判为环境变量。
func (c *Config) applyEnvVars(preexisting map[string]string) {
	for _, key := range managedKeys {
		if val, ok := preexisting[key]; ok {
			c.values[key] = ConfigValue{Value: val, Source: ConfigSourceEnvVar}
		}
	}
}

// Get 获取配置值及其来源
func (c *Config) Get(key string) (string, ConfigSource) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if v, ok := c.values[key]; ok {
		return v.Value, v.Source
	}
	if def, ok := defaultValues[key]; ok {
		return def, ConfigSourceDefault
	}
	return "", ConfigSourceDefault
}

// GetOr 获取配置值，为空时返回 fallback
func (c *Config) GetOr(key, fallback string) string {
	if val, _ := c.Get(key); val != "" {
		return val
	}
	return fallback
}

// GetDuration 获取时长配置，解析失败时返回 fallback
func (c *Config) GetDuration(key string, fallback time.Duration) time.Duration {
	val, _ := c.Get(key)
	if val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		fmt.Printf("警告: %s 不是合法时长（%q），使用默认值 %s\n", key, val, fallback)
		return fallback
	}
	return d
}

// GetBool 获取布尔配置，解析失败或为空时返回 fallback
func (c *Config) GetBool(key string, fallback bool) bool {
	val, _ := c.Get(key)
	if val == "" {
		return fallback
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return fallback
	}
	return b
}

// DatabaseURL 解析数据库连接串。
//
// 优先使用 DATABASE_URL；为空时由 POSTGRES_* 分项拼装。这样两种部署形态各用
// 一套配置，无需在 .env 中同时维护：
//
//   - 本地开发 / 生产：直接给 DATABASE_URL
//   - docker compose：只给 POSTGRES_USER / POSTGRES_PASSWORD / POSTGRES_DB，
//     由编排注入，主机名取服务名 postgres
//
// 分项拼装时密码经 URL 编码，避免密码中的 @ : / ? # 等字符破坏 DSN 结构。
func (c *Config) DatabaseURL() (string, ConfigSource, error) {
	if dsn, src := c.Get("DATABASE_URL"); dsn != "" {
		return dsn, src, nil
	}

	password, pwdSrc := c.Get("POSTGRES_PASSWORD")
	if password == "" {
		return "", ConfigSourceDefault, fmt.Errorf(
			"缺少数据库配置：请提供 DATABASE_URL，或提供 POSTGRES_PASSWORD 等分项（可通过环境变量、.env 或 config.yaml）")
	}

	host, _ := c.Get("POSTGRES_HOST")
	port, _ := c.Get("POSTGRES_PORT")
	user, _ := c.Get("POSTGRES_USER")
	dbname, _ := c.Get("POSTGRES_DB")
	sslmode, _ := c.Get("POSTGRES_SSLMODE")

	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + dbname,
	}
	u.RawQuery = url.Values{"sslmode": []string{sslmode}}.Encode()

	return u.String(), pwdSrc, nil
}

// MustGet 获取必需的配置值，不存在时 panic
func (c *Config) MustGet(key string) string {
	val, _ := c.Get(key)
	if val == "" {
		panic(fmt.Sprintf("缺少必需配置: %s", key))
	}
	return val
}

// IsProduction 是否为生产环境
func (c *Config) IsProduction() bool {
	env, _ := c.Get("APP_ENV")
	return env == "production"
}

// minSessionSecretLen SESSION_SECRET 的最小长度
const minSessionSecretLen = 32

// Validate 验证必填配置
func (c *Config) Validate() error {
	// 数据库配置两种形态皆可：DATABASE_URL，或 POSTGRES_* 分项
	if _, _, err := c.DatabaseURL(); err != nil {
		return err
	}

	if val, _ := c.Get("SESSION_SECRET"); val == "" {
		return fmt.Errorf("缺少必需配置: SESSION_SECRET（可通过环境变量、.env 或 config.yaml 提供）")
	}

	if secret, _ := c.Get("SESSION_SECRET"); len(secret) < minSessionSecretLen {
		return fmt.Errorf("SESSION_SECRET 长度不足：需至少 %d 个字符，当前 %d",
			minSessionSecretLen, len(secret))
	}

	// 密码哈希与明文密码二者至少提供一个
	hashVal, _ := c.Get("ADMIN_PASSWORD_HASH")
	pwdVal, _ := c.Get("ADMIN_PASSWORD")
	if hashVal == "" && pwdVal == "" {
		return fmt.Errorf("缺少 ADMIN_PASSWORD_HASH 或 ADMIN_PASSWORD（二者至少提供一个）")
	}

	// 生产环境不允许使用明文密码
	if c.IsProduction() && hashVal == "" {
		return fmt.Errorf("生产环境必须使用 ADMIN_PASSWORD_HASH，不可使用明文 ADMIN_PASSWORD")
	}

	if _, err := time.ParseDuration(c.GetOr("SESSION_LIFETIME", "24h")); err != nil {
		return fmt.Errorf("SESSION_LIFETIME 不是合法时长: %w", err)
	}

	return nil
}

// LogSources 打印各配置项的来源（不打印值）
func (c *Config) LogSources() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	fmt.Println("配置来源：")
	keys := make([]string, 0, len(managedKeys))
	for _, k := range managedKeys {
		if _, ok := c.values[k]; ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-20s %s\n", k, c.values[k].Source)
	}
}
