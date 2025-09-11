package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/PocketPalCo/shopping-service/config"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"google.golang.org/grpc"
)

// NewOTLPOnlyLogger creates a logger that exports logs only to OTLP (no local files)
func NewOTLPOnlyLogger(cfg *config.Config) (*slog.Logger, *log.LoggerProvider, error) {
	ctx := context.Background()

	// Create OTLP log exporter
	logExporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(cfg.OtlpEndpoint),
		otlploggrpc.WithInsecure(),
		otlploggrpc.WithDialOption(grpc.WithUserAgent("shopping-service")),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OTLP log exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("shopping-service"),
			semconv.ServiceVersionKey.String("1.0.0"),
			semconv.ServiceInstanceIDKey.String("shopping-service-instance"),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create log processor and provider
	loggerProvider := log.NewLoggerProvider(
		log.WithResource(res),
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	)

	// Create OTLP slog handler
	otlpHandler := otelslog.NewHandler("shopping-service",
		otelslog.WithLoggerProvider(loggerProvider),
	)

	// Create combined handler with stdout for local debugging and OTLP for observability
	var handler slog.Handler
	if cfg.Environment == "production" {
		// Production: OTLP only
		handler = otlpHandler
	} else {
		// Development: stdout + OTLP
		stdoutHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: cfg.GetSlogLevel(),
		})
		handler = &MultiHandler{
			handlers: []slog.Handler{stdoutHandler, otlpHandler},
		}
	}

	// Create the logger
	logger := slog.New(handler).With(
		"service", "shopping-service",
		"version", "1.0.0",
		"environment", cfg.Environment,
	)

	return logger, loggerProvider, nil
}

// MultiHandler sends logs to multiple handlers
type MultiHandler struct {
	handlers []slog.Handler
}

func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, record.Level) {
			// Clone the record for each handler
			if err := h.Handle(ctx, record.Clone()); err != nil {
				// Log handler errors to stderr but don't fail
				fmt.Printf("Handler error: %v\n", err)
			}
		}
	}
	return nil
}

func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: newHandlers}
}

func (m *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: newHandlers}
}