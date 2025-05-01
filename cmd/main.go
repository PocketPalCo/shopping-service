package main

import (
	"context"
	"errors"
	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/PocketPalCo/shopping-service/pkg/logger"
	"github.com/PocketPalCo/shopping-service/pkg/telemetry"
	"github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.9.0"
	"google.golang.org/grpc"
	"log/slog"
	"os"
	"time"
)

var (
	httpRequestsCounter  api.Int64Counter
	httpRequestHistogram api.Float64Histogram
)

func main() {
	mainContext := context.Background()
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	defaultLogger := logger.NewLogger(&cfg)
	slog.SetDefault(defaultLogger)

	conn, err := postgres.Init(cfg)
	if err != nil {
		slog.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	metricExporter, err := otlpmetricgrpc.New(mainContext,
		otlpmetricgrpc.WithEndpoint(cfg.OtlpEndpoint),
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithDialOption(grpc.WithUserAgent("shopping-service")),
	)
	if err != nil {
		slog.Error("failed to initialize otlp exporter", slog.String("error", err.Error()))
		os.Exit(1)
	}

	provider := metric.NewMeterProvider(metric.WithResource(resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("shopping-service"),
	)), metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(15*time.Second))))

	defer func() {
		if err := provider.Shutdown(mainContext); err != nil {
			slog.Error("failed to shutdown metric provider", slog.String("error", err.Error()))
		}
	}()

	err = telemetry.InitTelemetry(provider, conn)
	if err != nil {
		slog.Error("failed to initialize telemetry", slog.String("error", err.Error()))
		os.Exit(1)
	}

	instrumentedConn, err := telemetry.NewInstrumentedPool(provider, conn)
	if err != nil {
		slog.Error("failed to create instrumented pool", slog.String("error", err.Error()))
	}

	slog.Info("Starting server", slog.String("address", cfg.ServerAddress))

	app := fiber.New()
	app.Use(otelfiber.Middleware())

	app.Get("/error", func(ctx *fiber.Ctx) error {
		return errors.New("abc")
	})

	meter := provider.Meter("http")
	httpRequestsCounter, _ = meter.Int64Counter("http_requests_total", api.WithDescription("Total number of HTTP requests."))

	httpRequestHistogram, _ = meter.Float64Histogram("http_request_duration_ms", api.WithDescription("Duration of HTTP requests in milliseconds."))
	app.Get("/list", func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		durationMs := float64(time.Since(start).Milliseconds())

		httpRequestsCounter.Add(c.UserContext(), 1,
			api.WithAttributes(
				attribute.String("method", c.Method()),
				attribute.String("path", c.Route().Path),
				attribute.Int("status_code", c.Response().StatusCode()),
			),
		)

		var id uuid.UUID
		var name string
		err = instrumentedConn.QueryRow(mainContext, "select id, name from shopping_list_items").Scan(&id, &name)
		if err != nil {
			slog.Error("failed to query database", slog.String("error", err.Error()))
		}

		httpRequestHistogram.Record(c.UserContext(), durationMs,
			api.WithAttributes(
				attribute.String("method", c.Method()),
				attribute.String("path", c.Route().Path),
				attribute.Int("status_code", c.Response().StatusCode()),
			),
		)

		return c.JSON(fiber.Map{"id": id, "name": name})
	})

	err = app.Listen(":8080")
	if err != nil {
		return
	}

}

type ListItems struct {
	Id   uuid.UUID
	Name string
	Done bool
}
