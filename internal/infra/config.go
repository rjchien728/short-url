package infra

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config is the root configuration structure for the application.
type Config struct {
	App      AppConfig
	Server   ServerConfig
	Database DatabaseConfig
	Cache    RedisConfig // Redis DB 0
	Stream   RedisConfig // Redis DB 1
}

// AppConfig holds basic application settings.
type AppConfig struct {
	Env      string `mapstructure:"APP_ENV"`
	LogLevel string `mapstructure:"APP_LOG_LEVEL"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port    string `mapstructure:"PORT"`
	BaseURL string `mapstructure:"SERVER_BASE_URL"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	DSN          string `mapstructure:"DB_DSN"`
	MaxOpenConns int    `mapstructure:"DB_MAX_OPEN_CONNS"`
	MaxIdleConns int    `mapstructure:"DB_MAX_IDLE_CONNS"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL string
}

// Load configuration following this priority:
// Default values -> .env file -> Environment variables.
func Load() (*Config, error) {
	v := viper.New()

	// 1. Set default values
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("APP_LOG_LEVEL", "info")
	v.SetDefault("PORT", "8080")
	v.SetDefault("SERVER_BASE_URL", "http://localhost:8080")
	v.SetDefault("DB_MAX_OPEN_CONNS", 10)
	v.SetDefault("DB_MAX_IDLE_CONNS", 5)
	v.SetDefault("REDIS_CACHE_URL", "redis://localhost:6379/0")
	v.SetDefault("REDIS_STREAM_URL", "redis://localhost:6379/1")

	// 2. Read .env file (optional, for local development)
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	_ = v.ReadInConfig() // Ignore "file not found" error as .env is not required in production

	// 3. Automatically map environment variables (overrides .env)
	v.AutomaticEnv()

	// 4. Parse configuration into structs
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
		MaxIdleConns: v.GetInt("DB_MAX_IDLE_CONns"),
	}
	cfg.Cache = RedisConfig{
		URL: v.GetString("REDIS_CACHE_URL"),
	}
	cfg.Stream = RedisConfig{
		URL: v.GetString("REDIS_STREAM_URL"),
	}

	// 5. Validate required fields
	if cfg.Database.DSN == "" {
		return nil, fmt.Errorf("DB_DSN is required")
	}

	return cfg, nil
}
