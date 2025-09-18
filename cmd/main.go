package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/PocketPalCo/shopping-service/internal/infra/server"
	"github.com/PocketPalCo/shopping-service/pkg/logger"
	"go.opentelemetry.io/otel/sdk/log"
	"log/slog"
)

func main() {
	ctx := context.Background()
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Initialize logger based on configuration
	var loggerProvider *log.LoggerProvider
	if cfg.OtlpLogsEnabled {
		// OTLP logging enabled - use OTLP for logs
		otlpLogger, otlpLoggerProvider, err := logger.NewOTLPOnlyLogger(&cfg)
		if err != nil {
			slog.Error("Failed to initialize OTLP logger",
				"error", err.Error(),
				"service", "shopping-service",
				"component", "logger")
			os.Exit(1)
		} else {
			loggerProvider = otlpLoggerProvider
			slog.SetDefault(otlpLogger)
			slog.Info("OTLP logging enabled successfully",
				"endpoint", cfg.OtlpEndpoint,
				"service", "shopping-service",
				"component", "logger")
		}
	} else {
		// OTLP logging disabled - use standard logging (Promtail will collect from container logs)
		jsonLogger := logger.NewJSONLogger(&cfg)
		slog.SetDefault(jsonLogger)
		slog.Info("Standard JSON logging enabled (logs will be collected by Promtail)",
			"service", "shopping-service",
			"component", "logger",
			"log_level", cfg.LogLevel)
	}

	slog.Info("Starting shopping service",
		"component", "main",
		"environment", cfg.Environment,
		"server_address", cfg.ServerAddress,
		"log_level", cfg.LogLevel,
		"db_host", cfg.DbHost,
		"db_port", cfg.DbPort)

	conn, err := postgres.Init(&cfg)
	if err != nil {
		slog.Error("Failed to connect to database",
			"component", "database",
			"error", err.Error(),
			"db_host", cfg.DbHost,
			"db_port", cfg.DbPort,
			"db_name", cfg.DbDatabaseName)
		os.Exit(1)
	}

	slog.Info("Database connection established",
		"component", "database",
		"db_host", cfg.DbHost,
		"db_port", cfg.DbPort,
		"db_name", cfg.DbDatabaseName,
		"max_connections", cfg.DbMaxConnections)

	mainServer := server.New(ctx, &cfg, conn)
	if mainServer == nil {
		slog.Error("Failed to create server", "component", "server")
		os.Exit(1)
	}

	slog.Info("Starting server", "component", "server")
	go mainServer.Start()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("Server ready, waiting for shutdown signal", "component", "main")
	<-interrupt

	slog.Info("Shutdown signal received, stopping server", "component", "main")
	mainServer.Shutdown()
	conn.Close()

	// Cleanup OTLP logger provider if it was initialized
	if loggerProvider != nil {
		ctx := context.Background()
		if err := loggerProvider.Shutdown(ctx); err != nil {
			slog.Error("Failed to shutdown logger provider", "error", err.Error())
		}
	}

	slog.Info("Service shutdown complete", "component", "main")
}
