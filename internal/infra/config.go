package infra

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config 為整個應用的設定根結構。
type Config struct {
	App      AppConfig
	Server   ServerConfig
	Database DatabaseConfig
	Cache    RedisConfig // Redis DB 0
	Stream   RedisConfig // Redis DB 1
}

// AppConfig 為應用基礎設定。
type AppConfig struct {
	Env      string `mapstructure:"APP_ENV"`
	LogLevel string `mapstructure:"APP_LOG_LEVEL"`
}

// ServerConfig 為 HTTP 服務設定。
type ServerConfig struct {
	Port    string `mapstructure:"PORT"`
	BaseURL string `mapstructure:"SERVER_BASE_URL"`
}

// DatabaseConfig 為 PostgreSQL 連線設定。
type DatabaseConfig struct {
	DSN          string `mapstructure:"DB_DSN"`
	MaxOpenConns int    `mapstructure:"DB_MAX_OPEN_CONNS"`
	MaxIdleConns int    `mapstructure:"DB_MAX_IDLE_CONNS"`
}

// RedisConfig 為 Redis 連線設定（Cache 與 Stream 共用此 struct）。
type RedisConfig struct {
	URL string
}

// Load 依照優先順序載入設定：
// 內建預設值 → .env 檔 → 系統環境變數。
func Load() (*Config, error) {
	v := viper.New()

	// 1. 設定預設值
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("APP_LOG_LEVEL", "info")
	v.SetDefault("PORT", "8080")
	v.SetDefault("SERVER_BASE_URL", "http://localhost:8080")
	v.SetDefault("DB_MAX_OPEN_CONNS", 10)
	v.SetDefault("DB_MAX_IDLE_CONNS", 5)
	v.SetDefault("REDIS_CACHE_URL", "redis://localhost:6379/0")
	v.SetDefault("REDIS_STREAM_URL", "redis://localhost:6379/1")

	// 2. 嘗試讀取 .env 檔（本地開發用，不存在時忽略）
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	_ = v.ReadInConfig() // 忽略 "file not found" error，生產環境不需要此檔

	// 3. 自動映射系統環境變數（環境變數優先於 .env）
	v.AutomaticEnv()

	// 4. 解析至結構體
	cfg := &Config{}
	cfg.App = AppConfig{
		Env:      v.GetString("APP_ENV"),
		LogLevel: v.GetString("APP_LOG_LEVEL"),
	}
	cfg.Server = ServerConfig{
		Port:    v.GetString("PORT"),
		BaseURL: v.GetString("SERVER_BASE_URL"),
	}
	cfg.Database = DatabaseConfig{
		DSN:          v.GetString("DB_DSN"),
		MaxOpenConns: v.GetInt("DB_MAX_OPEN_CONNS"),
		MaxIdleConns: v.GetInt("DB_MAX_IDLE_CONNS"),
	}
	cfg.Cache = RedisConfig{
		URL: v.GetString("REDIS_CACHE_URL"),
	}
	cfg.Stream = RedisConfig{
		URL: v.GetString("REDIS_STREAM_URL"),
	}

	// 5. 驗證必填欄位
	if cfg.Database.DSN == "" {
		return nil, fmt.Errorf("DB_DSN is required")
	}

	return cfg, nil
}
