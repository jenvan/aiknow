package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// L1Config L1 启动配置
type L1Config struct {
	// 数据库
	DatabaseURL string `yaml:"database_url" env:"DATABASE_URL"`

	// AI 服务
	QwenAPIKey    string `yaml:"qwen_api_key" env:"QWEN_API_KEY"`
	QwenBaseURL   string `yaml:"qwen_base_url" env:"QWEN_BASE_URL"`
	BAAIAPIKey    string `yaml:"baai_api_key" env:"BAAI_API_KEY"`
	BAAIBaseURL   string `yaml:"baai_base_url" env:"BAAI_BASE_URL"`

	// 服务器
	Port int `yaml:"port" env:"PORT"`

	// 会话
	SessionSecret string `yaml:"session_secret" env:"SESSION_SECRET"`

	// 管理员
	AdminUsername     string `yaml:"admin_username" env:"ADMIN_USERNAME"`
	AdminPassword     string `yaml:"admin_password" env:"ADMIN_PASSWORD"`
	AdminPasswordHash string `yaml:"admin_password_hash" env:"ADMIN_PASSWORD_HASH"`

	// 环境
	Env string `yaml:"env" env:"APP_ENV"`
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
type Config struct {
	mu       sync.RWMutex
	values   map[string]ConfigValue
	defaults L1Config
}

// NewConfig 创建配置管理器
func NewConfig() *Config {
	c := &Config{
		values: make(map[string]ConfigValue),
	}

	// 加载默认值
	c.loadDefaults()

	// 加载 .env 文件（不覆盖已有环境变量）
	c.loadEnvFile()

	// 加载 YAML 配置
	c.loadYAML()

	// 加载环境变量（优先级最高）
	c.loadEnvVars()

	return c
}

func (c *Config) loadDefaults() {
	c.defaults = L1Config{
		Port: 8080,
		Env:  "development",
	}
}

func (c *Config) loadEnvFile() {
	if _, err := os.Stat(".env"); err != nil {
		return // .env 不存在，静默跳过
	}

	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: failed to load .env file: %v\n", err)
		return
	}

	// 记录从 .env 文件加载的键（仅当环境变量未设置时）
	for _, key := range []string{"DATABASE_URL", "QWEN_API_KEY", "QWEN_BASE_URL", "BAAI_API_KEY", "BAAI_BASE_URL", "SESSION_SECRET", "ADMIN_PASSWORD_HASH", "ADMIN_USERNAME", "ADMIN_PASSWORD"} {
		if val := os.Getenv(key); val != "" {
			c.values[key] = ConfigValue{Value: val, Source: ConfigSourceEnvFile}
		}
	}
}

func (c *Config) loadYAML() {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return // config.yaml 不存在，使用默认值
	}

	var cfg L1Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("Warning: failed to parse config.yaml: %v\n", err)
		return
	}

	// 仅记录非空且环境变量未覆盖的字段
	if cfg.DatabaseURL != "" {
		c.values["DATABASE_URL"] = ConfigValue{Value: cfg.DatabaseURL, Source: ConfigSourceYAML}
	}
	if cfg.QwenAPIKey != "" {
		c.values["QWEN_API_KEY"] = ConfigValue{Value: cfg.QwenAPIKey, Source: ConfigSourceYAML}
	}
	if cfg.QwenBaseURL != "" {
		c.values["QWEN_BASE_URL"] = ConfigValue{Value: cfg.QwenBaseURL, Source: ConfigSourceYAML}
	}
	if cfg.BAAIAPIKey != "" {
		c.values["BAAI_API_KEY"] = ConfigValue{Value: cfg.BAAIAPIKey, Source: ConfigSourceYAML}
	}
	if cfg.BAAIBaseURL != "" {
		c.values["BAAI_BASE_URL"] = ConfigValue{Value: cfg.BAAIBaseURL, Source: ConfigSourceYAML}
	}
	if cfg.SessionSecret != "" {
		c.values["SESSION_SECRET"] = ConfigValue{Value: cfg.SessionSecret, Source: ConfigSourceYAML}
	}
	if cfg.AdminPasswordHash != "" {
		c.values["ADMIN_PASSWORD_HASH"] = ConfigValue{Value: cfg.AdminPasswordHash, Source: ConfigSourceYAML}
	}
	if cfg.AdminUsername != "" {
		c.values["ADMIN_USERNAME"] = ConfigValue{Value: cfg.AdminUsername, Source: ConfigSourceYAML}
	}
}

func (c *Config) loadEnvVars() {
	// 环境变量优先级最高，覆盖所有其他来源
	for _, key := range []string{"DATABASE_URL", "QWEN_API_KEY", "QWEN_BASE_URL", "BAAI_API_KEY", "BAAI_BASE_URL", "SESSION_SECRET", "ADMIN_PASSWORD_HASH", "ADMIN_USERNAME", "ADMIN_PASSWORD"} {
		if val := os.Getenv(key); val != "" {
			c.values[key] = ConfigValue{Value: val, Source: ConfigSourceEnvVar}
		}
	}
}

// Get 获取配置值
func (c *Config) Get(key string) (string, ConfigSource) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if v, ok := c.values[key]; ok {
		return v.Value, v.Source
	}

	// 返回默认值
	switch key {
	case "PORT":
		return fmt.Sprintf("%d", c.defaults.Port), ConfigSourceDefault
	case "APP_ENV":
		return c.defaults.Env, ConfigSourceDefault
	default:
		return "", ConfigSourceDefault
	}
}

// MustGet 获取必需的配置值，不存在时 panic
func (c *Config) MustGet(key string) string {
	val, _ := c.Get(key)
	if val == "" {
		panic(fmt.Sprintf("missing required config: %s", key))
	}
	return val
}

// Validate 验证必填配置
func (c *Config) Validate() error {
	required := []string{
		"DATABASE_URL",
		"SESSION_SECRET",
		"ADMIN_PASSWORD_HASH",
	}

	for _, key := range required {
		val, source := c.Get(key)
		if val == "" {
			return fmt.Errorf("missing required configuration: %s (set via %s or environment)",
				key, source)
		}
	}

	// ADMIN_PASSWORD_HASH 和 ADMIN_PASSWORD 至少有一个
	hashVal, _ := c.Get("ADMIN_PASSWORD_HASH")
	pwdVal, _ := c.Get("ADMIN_PASSWORD")
	if hashVal == "" && pwdVal == "" {
		return fmt.Errorf("missing ADMIN_PASSWORD_HASH or ADMIN_PASSWORD")
	}

	return nil
}

// LogSources 打印配置来源（不打印值）
func (c *Config) LogSources() {
	fmt.Println("Configuration sources:")
	for key, val := range c.values {
		fmt.Printf("  %s: %s\n", key, val.Source)
	}
}
