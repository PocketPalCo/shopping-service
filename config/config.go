package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

type Config struct {
	Environment       string `mapstructure:"SSV_ENVIRONMENT"`
	ServerName        string `mapstructure:"SSV_SERVER_NAME"`
	ServerAddress     string `mapstructure:"SSV_SERVER_BIND_ADDR"`
	ServerReadTimeout int16  `mapstructure:"SSV_SERVER_READ_TIMEOUT"`
	LogFormat         string `mapstructure:"SSV_LOG_FORMAT"` // text or json
	LogLevel          string `mapstructure:"SSV_LOG_LEVEL"`  // debug, info, warn, error
	RateLimitMax      int    `mapstructure:"SSV_RATE_LIMIT_MAX"`
	RateLimitWindow   int    `mapstructure:"SSV_RATE_LIMIT_WINDOW"`

	DbHost           string `mapstructure:"SSV_DB_HOST"`
	DbPort           int16  `mapstructure:"SSV_DB_PORT"`
	DbSSLMode        string `mapstructure:"SSV_DB_SSL"`
	DbUser           string `mapstructure:"SSV_DB_USER"`
	DbPassword       string `mapstructure:"SSV_DB_PASSWORD"`
	DbDatabaseName   string `mapstructure:"SSV_DB_DATABASE"`
	DbMaxConnections int    `mapstructure:"SSV_DB_MAX_CONNECTIONS"`

	//swagger
	SwaggerHost string `mapstructure:"SSV_SWAGGER_HOST"`

	// Redis
	RedisHost string `mapstructure:"SSV_REDIS_HOST"`
	RedisPort int16  `mapstructure:"SSV_REDIS_PORT"`
	RedisDb   int    `mapstructure:"SSV_REDIS_DB"`
	RedisUser string `mapstructure:"SSV_REDIS_USER"`
	RedisPass string `mapstructure:"SSV_REDIS_PASS"`

	OtlpEndpoint   string `mapstructure:"SSV_OTLP_ENDPOINT"`
	JaegerEndpoint string `mapstructure:"SSV_JAEGER_ENDPOINT"`

	// Telegram Bot Configuration
	TelegramBotToken string `mapstructure:"SSV_TELEGRAM_BOT_TOKEN"`
	TelegramDebug    bool   `mapstructure:"SSV_TELEGRAM_DEBUG"`
	TelegramAdmins   string `mapstructure:"SSV_TELEGRAM_ADMINS"` // Comma-separated list of Telegram IDs

	// OpenAI Configuration
	OpenAIAPIKey          string  `mapstructure:"SSV_OPENAI_API_KEY"`
	OpenAIModel           string  `mapstructure:"SSV_OPENAI_MODEL"`
	OpenAIBaseURL         string  `mapstructure:"SSV_OPENAI_BASE_URL"`
	OpenAIMaxTokens       int     `mapstructure:"SSV_OPENAI_MAX_TOKENS"`
	OpenAITemperature     float64 `mapstructure:"SSV_OPENAI_TEMPERATURE"`
	OpenAIUseResponsesAPI bool    `mapstructure:"SSV_OPENAI_USE_RESPONSES_API"`
	OpenAIStore           bool    `mapstructure:"SSV_OPENAI_STORE"`
	OpenAIReasoningEffort string  `mapstructure:"SSV_OPENAI_REASONING_EFFORT"`

	// STT Service Configuration
	STTServiceURL string `mapstructure:"SSV_STT_SERVICE_URL"`

	// Cloud Storage Configuration
	CloudProvider                string `mapstructure:"SSV_CLOUD_PROVIDER"`
	AzureStorageConnectionString string `mapstructure:"SSV_AZURE_STORAGE_CONNECTION_STRING"`
	AzureStorageAccountName      string `mapstructure:"SSV_AZURE_STORAGE_ACCOUNT_NAME"`
	AzureStorageAccountKey       string `mapstructure:"SSV_AZURE_STORAGE_ACCOUNT_KEY"`
	AzureStorageContainerName    string `mapstructure:"SSV_AZURE_STORAGE_CONTAINER_NAME"`
	AzureStorageBaseURL          string `mapstructure:"SSV_AZURE_STORAGE_BASE_URL"`
	AzureStorageUseHTTPS         bool   `mapstructure:"SSV_AZURE_STORAGE_USE_HTTPS"`

	// Azure Document Intelligence Configuration
	AzureDocumentIntelligenceEndpoint   string `mapstructure:"SSV_AZURE_DOCUMENT_INTELLIGENCE_ENDPOINT"`
	AzureDocumentIntelligenceAPIKey     string `mapstructure:"SSV_AZURE_DOCUMENT_INTELLIGENCE_API_KEY"`
	AzureDocumentIntelligenceAPIVersion string `mapstructure:"SSV_AZURE_DOCUMENT_INTELLIGENCE_API_VERSION"`
	AzureDocumentIntelligenceModel      string `mapstructure:"SSV_AZURE_DOCUMENT_INTELLIGENCE_RECEIPT_MODEL"`
}

// DefaultConfig generates a config with sane defaults.
// See: The example .env file in the package docs for default values.
func DefaultConfig() Config {
	return Config{
		Environment:       "local",
		ServerAddress:     "0.0.0.0:3001",
		ServerReadTimeout: 60,
		LogFormat:         "text",
		LogLevel:          "info",
		RateLimitMax:      100,
		RateLimitWindow:   30,

		DbHost:           "localhost",
		DbPort:           5432,
		DbSSLMode:        "disable",
		DbUser:           "postgres",
		DbPassword:       "postgres",
		DbDatabaseName:   "pocket-pal",
		DbMaxConnections: 100,

		// Swagger
		SwaggerHost: "localhost:3001",

		// Redis
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDb:   0,
		RedisUser: "redis",
		RedisPass: "redis",

		OtlpEndpoint:   "localhost:4317",
		JaegerEndpoint: "http://localhost:14268/api/traces",

		TelegramBotToken: "",
		TelegramDebug:    false,
		TelegramAdmins:   "",

		// OpenAI defaults
		OpenAIAPIKey:          "",
		OpenAIModel:           "gpt-5-nano",
		OpenAIBaseURL:         "https://api.openai.com/v1",
		OpenAIMaxTokens:       300,
		OpenAITemperature:     0.1,
		OpenAIUseResponsesAPI: true,
		OpenAIStore:           true,
		OpenAIReasoningEffort: "medium",

		// STT service defaults
		STTServiceURL: "http://localhost:8000",

		// Cloud storage defaults
		CloudProvider:                "azure",
		AzureStorageConnectionString: "",
		AzureStorageAccountName:      "",
		AzureStorageAccountKey:       "",
		AzureStorageContainerName:    "files",
		AzureStorageBaseURL:          "",
		AzureStorageUseHTTPS:         true,

		// Azure Document Intelligence defaults
		AzureDocumentIntelligenceEndpoint:   "",
		AzureDocumentIntelligenceAPIKey:     "",
		AzureDocumentIntelligenceAPIVersion: "2024-11-30",
		AzureDocumentIntelligenceModel:      "prebuilt-receipt",
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
	viper.SetDefault("SSV_SWAGGER_HOST", config.SwaggerHost)
	viper.SetDefault("SSV_OTLP_ENDPOINT", config.OtlpEndpoint)
	viper.SetDefault("SSV_JAEGER_ENDPOINT", config.JaegerEndpoint)
	viper.SetDefault("SSV_REDIS_HOST", config.RedisHost)
	viper.SetDefault("SSV_REDIS_PORT", config.RedisPort)
	viper.SetDefault("SSV_REDIS_USER", config.RedisUser)
	viper.SetDefault("SSV_REDIS_PASS", config.RedisPass)
	viper.SetDefault("SSV_REDIS_DB", config.RedisDb)
	viper.SetDefault("SSV_TELEGRAM_BOT_TOKEN", config.TelegramBotToken)
	viper.SetDefault("SSV_TELEGRAM_DEBUG", config.TelegramDebug)
	viper.SetDefault("SSV_TELEGRAM_ADMINS", config.TelegramAdmins)
	viper.SetDefault("SSV_OPENAI_API_KEY", config.OpenAIAPIKey)
	viper.SetDefault("SSV_OPENAI_MODEL", config.OpenAIModel)
	viper.SetDefault("SSV_OPENAI_BASE_URL", config.OpenAIBaseURL)
	viper.SetDefault("SSV_OPENAI_MAX_TOKENS", config.OpenAIMaxTokens)
	viper.SetDefault("SSV_OPENAI_TEMPERATURE", config.OpenAITemperature)
	viper.SetDefault("SSV_OPENAI_USE_RESPONSES_API", config.OpenAIUseResponsesAPI)
	viper.SetDefault("SSV_OPENAI_STORE", config.OpenAIStore)
	viper.SetDefault("SSV_OPENAI_REASONING_EFFORT", config.OpenAIReasoningEffort)
	viper.SetDefault("SSV_STT_SERVICE_URL", config.STTServiceURL)
	viper.SetDefault("SSV_CLOUD_PROVIDER", config.CloudProvider)
	viper.SetDefault("SSV_AZURE_STORAGE_CONNECTION_STRING", config.AzureStorageConnectionString)
	viper.SetDefault("SSV_AZURE_STORAGE_ACCOUNT_NAME", config.AzureStorageAccountName)
	viper.SetDefault("SSV_AZURE_STORAGE_ACCOUNT_KEY", config.AzureStorageAccountKey)
	viper.SetDefault("SSV_AZURE_STORAGE_CONTAINER_NAME", config.AzureStorageContainerName)
	viper.SetDefault("SSV_AZURE_STORAGE_BASE_URL", config.AzureStorageBaseURL)
	viper.SetDefault("SSV_AZURE_STORAGE_USE_HTTPS", config.AzureStorageUseHTTPS)
	viper.SetDefault("SSV_AZURE_DOCUMENT_INTELLIGENCE_ENDPOINT", config.AzureDocumentIntelligenceEndpoint)
	viper.SetDefault("SSV_AZURE_DOCUMENT_INTELLIGENCE_API_KEY", config.AzureDocumentIntelligenceAPIKey)
	viper.SetDefault("SSV_AZURE_DOCUMENT_INTELLIGENCE_API_VERSION", config.AzureDocumentIntelligenceAPIVersion)
	viper.SetDefault("SSV_AZURE_DOCUMENT_INTELLIGENCE_RECEIPT_MODEL", config.AzureDocumentIntelligenceModel)

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

// GetSlogLevel converts the string log level to slog.Level.
func (c Config) GetSlogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo // default fallback
	}
}

// GetTelegramAdmins parses the comma-separated list of Telegram admin IDs.
func (c Config) GetTelegramAdmins() ([]int64, error) {
	if c.TelegramAdmins == "" {
		return []int64{}, nil
	}

	adminStrings := strings.Split(c.TelegramAdmins, ",")
	admins := make([]int64, 0, len(adminStrings))

	for _, adminStr := range adminStrings {
		adminStr = strings.TrimSpace(adminStr)
		if adminStr == "" {
			continue
		}

		adminID, err := strconv.ParseInt(adminStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid admin Telegram ID '%s': %w", adminStr, err)
		}

		admins = append(admins, adminID)
	}

	return admins, nil
}

// GetOpenAIConfig converts config values to OpenAI configuration struct.
func (c Config) GetOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		APIKey:          c.OpenAIAPIKey,
		Model:           c.OpenAIModel,
		BaseURL:         c.OpenAIBaseURL,
		MaxTokens:       c.OpenAIMaxTokens,
		Temperature:     c.OpenAITemperature,
		UseResponsesAPI: c.OpenAIUseResponsesAPI,
		Store:           c.OpenAIStore,
		ReasoningEffort: c.OpenAIReasoningEffort,
	}
}

// OpenAIConfig holds OpenAI client configuration
type OpenAIConfig struct {
	APIKey          string
	Model           string // e.g., "gpt-5", "gpt-5-nano"
	BaseURL         string // for switching to local models later
	MaxTokens       int
	Temperature     float64
	UseResponsesAPI bool   // Use new Responses API instead of Chat Completions
	Store           bool   // Enable stateful context for better reasoning
	ReasoningEffort string // "low", "medium", "high" for GPT-5 reasoning
}

// GetCloudConfig converts config values to cloud storage configuration struct.
func (c Config) GetCloudConfig() CloudConfig {
	return CloudConfig{
		Provider: c.CloudProvider,
		Azure: AzureCloudConfig{
			StorageAccountName: c.AzureStorageAccountName,
			StorageAccountKey:  c.AzureStorageAccountKey,
			ConnectionString:   c.AzureStorageConnectionString,
			ContainerName:      c.AzureStorageContainerName,
			BaseURL:            c.AzureStorageBaseURL,
			UseHTTPS:           c.AzureStorageUseHTTPS,
		},
	}
}

// CloudConfig holds cloud storage configuration
type CloudConfig struct {
	Provider string
	Azure    AzureCloudConfig
	// AWS and GCP configs can be added later
}

// AzureCloudConfig holds Azure Blob Storage specific configuration
type AzureCloudConfig struct {
	StorageAccountName string
	StorageAccountKey  string
	ConnectionString   string
	ContainerName      string
	BaseURL            string
	UseHTTPS           bool
}

// GetDocumentIntelligenceConfig converts config values to Document Intelligence configuration struct.
func (c Config) GetDocumentIntelligenceConfig() DocumentIntelligenceConfig {
	return DocumentIntelligenceConfig{
		Endpoint:   c.AzureDocumentIntelligenceEndpoint,
		APIKey:     c.AzureDocumentIntelligenceAPIKey,
		APIVersion: c.AzureDocumentIntelligenceAPIVersion,
		Model:      c.AzureDocumentIntelligenceModel,
	}
}

// DocumentIntelligenceConfig holds Azure Document Intelligence configuration
type DocumentIntelligenceConfig struct {
	Endpoint   string
	APIKey     string
	APIVersion string
	Model      string
}
