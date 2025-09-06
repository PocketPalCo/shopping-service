package server

import (
	"context"
	"sync"

	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/core/telegram"
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/PocketPalCo/shopping-service/pkg/telemetry"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"google.golang.org/grpc"
	"log/slog"
	"time"
)

type Server struct {
	cfg             *config.Config
	app             *fiber.App
	db              postgres.DB
	traceProvider   *sdktrace.TracerProvider
	metricProvider  *metric.MeterProvider
	telegramService telegram.TelegramService
	loggerProvider  interface{ Shutdown(context.Context) error } // log.LoggerProvider interface
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

var tracer = otel.Tracer("server")

func New(ctx context.Context, cfg *config.Config, dbConn *pgxpool.Pool) *Server {
	traceExporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(cfg.JaegerEndpoint)))
	if err != nil {
		slog.Error("failed to initialize jaeger exporter", slog.String("error", err.Error()))
		return nil
	}

	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OtlpEndpoint),
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithDialOption(grpc.WithUserAgent("shopping-service")),
	)
	if err != nil {
		slog.Error("failed to initialize otlp exporter", slog.String("error", err.Error()))
		return nil
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String("shopping-service"),
			)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	provider := metric.NewMeterProvider(metric.WithResource(resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("shopping-service"),
	)), metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(15*time.Second))))

	otel.SetMeterProvider(provider)

	err = telemetry.InitTelemetry(provider, dbConn)
	if err != nil {
		slog.Error("failed to initialize telemetry", slog.String("error", err.Error()))
		return nil
	}

	instrumentedConn, err := telemetry.NewInstrumentedPool(provider, dbConn)
	if err != nil {
		slog.Error("failed to create instrumented pool", slog.String("error", err.Error()))
		return nil
	}

	app := fiber.New(cfg.Fiber())

	serverCtx, cancel := context.WithCancel(ctx)

	// Initialize Telegram service
	telegramService, err := telegram.NewTelegramService(cfg, dbConn, slog.Default())
	if err != nil {
		slog.Error("failed to initialize telegram service", slog.String("error", err.Error()))
		cancel()
		return nil
	}

	return &Server{
		cfg:             cfg,
		app:             app,
		db:              instrumentedConn,
		traceProvider:   tp,
		metricProvider:  provider,
		telegramService: telegramService,
		ctx:             serverCtx,
		cancel:          cancel,
	}
}

func (s *Server) Start() {
	initGlobalMiddlewares(s.app, s.cfg)
	registerHttpRoutes(s.app, s.cfg, s.db)

	// Start Telegram service
	if s.telegramService.IsEnabled() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := s.telegramService.Start(s.ctx); err != nil {
				slog.Error("Telegram service error", slog.String("error", err.Error()))
			}
		}()
	}

	slog.Info("Starting HTTP server", slog.String("address", s.cfg.ServerAddress))

	// Start HTTP server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.app.Listen(s.cfg.ServerAddress); err != nil {
			slog.Error("HTTP server error", slog.String("error", err.Error()))
		}
	}()
}

func (s *Server) Shutdown() {
	slog.Info("Shutting down server")

	// Cancel context to stop all goroutines
	s.cancel()

	// Stop Telegram service
	s.telegramService.Stop()

	// Shutdown HTTP server
	if err := s.app.Shutdown(); err != nil {
		slog.Error("Error shutting down HTTP server", slog.String("error", err.Error()))
	}

	// Wait for all goroutines to finish
	s.wg.Wait()

	// Shutdown telemetry providers
	if err := s.traceProvider.Shutdown(context.Background()); err != nil {
		slog.Error("Error shutting down trace provider", slog.String("error", err.Error()))
	}

	if err := s.metricProvider.Shutdown(context.Background()); err != nil {
		slog.Error("Error shutting down metric provider", slog.String("error", err.Error()))
	}

	if s.loggerProvider != nil {
		if err := s.loggerProvider.Shutdown(context.Background()); err != nil {
			slog.Error("Error shutting down log provider", slog.String("error", err.Error()))
		}
	}

	s.db.Close()

	slog.Info("Server shut down successfully")
}
