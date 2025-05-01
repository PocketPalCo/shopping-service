package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

type Config struct {
	Environment       string     `mapstructure:"SSV_ENVIRONMENT"`
	ServerName        string     `mapstructure:"SSV_SERVER_NAME"`
	ServerAddress     string     `mapstructure:"SSV_SERVER_BIND_ADDR"`
	ServerReadTimeout int16      `mapstructure:"SSV_SERVER_READ_TIMEOUT"`
	LogFormat         string     `mapstructure:"SSV_LOG_FORMAT"` // text or json
	LogLevel          slog.Level `mapstructure:"SSV_LOG_LEVEL"`  // debug, info, warn, error
	RateLimitMax      int        `mapstructure:"SSV_RATE_LIMIT_MAX"`
	RateLimitWindow   int        `mapstructure:"SSV_RATE_LIMIT_WINDOW"`

	DbHost           string `mapstructure:"SSV_DB_HOST"`
	DbPort           int16  `mapstructure:"SSV_DB_PORT"`
	DbSSLMode        string `mapstructure:"SSV_DB_SSL"`
	DbUser           string `mapstructure:"SSV_DB_USER"`
	DbPassword       string `mapstructure:"SSV_DB_PASSWORD"`
	DbDatabaseName   string `mapstructure:"SSV_DB_DATABASE"`
	DbMaxConnections int    `mapstructure:"SSV_DB_MAX_CONNECTIONS"`

	OtlpEndpoint string `mapstructure:"SSV_OTLP_ENDPOINT"`
}

// DefaultConfig generates a config with sane defaults.
// See: The example .env file in the package docs for default values.
func DefaultConfig() Config {
	return Config{
		Environment:       "local",
		ServerAddress:     "0.0.0.0:3001",
		ServerReadTimeout: 60,
		LogFormat:         "text",
		LogLevel:          slog.LevelInfo,
		RateLimitMax:      100,
		RateLimitWindow:   30,

		DbHost:           "localhost",
		DbPort:           5432,
		DbSSLMode:        "disable",
		DbUser:           "postgres",
		DbPassword:       "postgres",
		DbDatabaseName:   "pocket-pal",
		DbMaxConnections: 100,

		OtlpEndpoint: "localhost:4317",
	}
}

// LoadConfig will attempt to load a configuration from the default file location and fallback to environment variables.
func LoadConfig() (Config, error) {
	envFile := os.Getenv("SSV_ENV_FILE")
	if envFile == "" {
		envFile = ".env"
	}

	var cfg Config
	var err error

	if _, err = os.Stat(envFile); errors.Is(err, os.ErrNotExist) {
		cfg, err = ConfigFromEnvironment()
	} else {
		// Load configuration
		cfg, err = ConfigFromFile(envFile)
	}

	return cfg, err
}

// ConfigFromEnvironment will look for the specified configuration from environment variables
// See package docs for a list of available environment variables.
func ConfigFromEnvironment() (config Config, err error) {
	// Set defaults
	config = DefaultConfig()
	viper.SetDefault("SSV_ENVIRONMENT", config.Environment)
	viper.SetDefault("SSV_SERVER_BIND_ADDR", config.ServerAddress)
	viper.SetDefault("SSV_SERVER_READ_TIMEOUT", config.ServerReadTimeout)
	viper.SetDefault("SSV_LOG_LEVEL", config.LogLevel)
	viper.SetDefault("SSV_LOG_FORMAT", config.LogFormat)
	viper.SetDefault("SSV_RATE_LIMIT_MAX", config.RateLimitMax)
	viper.SetDefault("SSV_RATE_LIMIT_WINDOW", config.RateLimitWindow)
	viper.SetDefault("SSV_DB_HOST", config.DbHost)
	viper.SetDefault("SSV_DB_PORT", config.DbPort)
	viper.SetDefault("SSV_DB_SSL", config.DbSSLMode)
	viper.SetDefault("SSV_DB_USER", config.DbUser)
	viper.SetDefault("SSV_DB_PASSWORD", config.DbPassword)
	viper.SetDefault("SSV_DB_DATABASE", config.DbDatabaseName)
	viper.SetDefault("SSV_DB_MAX_CONNECTIONS", config.DbMaxConnections)
	viper.SetDefault("SSV_OTLP_ENDPOINT", config.OtlpEndpoint)

	// Override config values with environment variables
	viper.AutomaticEnv()
	err = viper.Unmarshal(&config)
	return
}

// ConfigFromFile will look for the specified configuration file in the current directory and initialize
// a Config from it. Values provided by environment variables will override ones found in
// the file. See package docs for a list of available environment variables.
func ConfigFromFile(f string) (config Config, err error) {
	if config, err = ConfigFromEnvironment(); err != nil {
		return
	}

	viper.AddConfigPath(".")
	viper.SetConfigFile(f)
	viper.SetConfigType("env")

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	err = viper.Unmarshal(&config)

	return
}

// Fiber initializes and returns a Fiber config based on server config values.
// See https://docs.gofiber.io/api/fiber#config
func (c Config) Fiber() fiber.Config {
	// Return Fiber configuration.
	return fiber.Config{
		ReadTimeout: time.Second * time.Duration(c.ServerReadTimeout),
		BodyLimit:   10 * 1024 * 1024 * 1024, // 10MB
	}
}

// DbConnectionString generates a connection string for the database based on config values.
func (c Config) DbConnectionString() string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s", c.DbUser, url.QueryEscape(c.DbPassword), c.DbHost, c.DbPort, c.DbDatabaseName, c.DbSSLMode)
}
