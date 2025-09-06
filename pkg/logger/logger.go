package logger

import (
	"github.com/PocketPalCo/shopping-service/config"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

func NewLogger(cfg *config.Config) *slog.Logger {
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("failed to create logs directory", "err", err)
		os.Exit(1)
	}

	logFilePath := filepath.Join(logDir, "shopping-service.log")

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("failed to open log file", "err", err)
		os.Exit(1)
	}
	//defer func(file *os.File) {
	//	if err := file.Close(); err != nil {
	//		slog.Error("failed to close log file", "err", err)
	//	}
	//}(logFile)

	mw := io.MultiWriter(os.Stdout, logFile)
	var handler slog.Handler
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(mw, &slog.HandlerOptions{
			Level: cfg.GetSlogLevel(),
		})
	} else {
		handler = slog.NewTextHandler(mw, &slog.HandlerOptions{
			Level: cfg.GetSlogLevel(),
		})
	}
	logger := slog.New(handler).With("env", cfg.Environment)

	return logger
}
