package logger

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PocketPalCo/shopping-service/config"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"google.golang.org/grpc"
)

// NewObservableLogger creates a logger that exports logs to OTLP for Grafana
func NewObservableLogger(cfg *config.Config) (*slog.Logger, *log.LoggerProvider, error) {
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

	// Create the standard logger for local output
	localLogger := NewLogger(cfg)

	// Create a multi-handler that sends logs both locally and to OTLP
	multiHandler := &MultiHandler{
		handlers: []slog.Handler{
			localLogger.Handler(),
			otlpHandler,
		},
	}

	// Create the observable logger
	observableLogger := slog.New(multiHandler).With(
		"service", "shopping-service",
		"version", "1.0.0",
		"environment", cfg.Environment,
	)

	return observableLogger, loggerProvider, nil
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
