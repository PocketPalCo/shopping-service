package server

import (
	"context"
	"errors"
	"time"

	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	slogfiber "github.com/samber/slog-fiber"
	"go.opentelemetry.io/otel/attribute"
	api "go.opentelemetry.io/otel/metric"
	"log/slog"
)

var (
	httpRequestsCounter  api.Int64Counter
	httpRequestHistogram api.Float64Histogram
)

func initGlobalMiddlewares(app *fiber.App, cfg *config.Config) {
	app.Use(
		compress.New(compress.Config{
			Level: compress.LevelDefault,
		}),

		slogfiber.NewWithFilters(slog.Default(), slogfiber.IgnorePath("/health")),

		cors.New(cors.Config{
			AllowOrigins: "*", // TODO - add allowed origins
			AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Request-ID",
			AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
		}),

		favicon.New(),
		limiter.New(limiter.Config{
			Max:               cfg.RateLimitMax,
			Expiration:        time.Duration(cfg.RateLimitWindow) * time.Second,
			LimiterMiddleware: limiter.SlidingWindow{},
		}),
	)

	app.Use(otelfiber.Middleware())
}

func registerHttpRoutes(app *fiber.App, cfg *config.Config, db postgres.DB) {
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "timestamp": time.Now().Unix()})
	})

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	app.Static("/", "./public")

	apiRoutes := app.Group("/v1")

	// Legacy shopping list endpoint (maintain compatibility)
	apiRoutes.Get("/list", withMetrics(db, func(c *fiber.Ctx) error {
		var id uuid.UUID
		var name string
		err := db.QueryRow(c.UserContext(), "SELECT id, name FROM shopping_list_items LIMIT 1").Scan(&id, &name)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				slog.Info("No shopping list items found",
					"component", "http_handler",
					"endpoint", "/v1/list",
					"result", "empty")
				return c.JSON(fiber.Map{"message": "No shopping list items found"})
			}
			slog.Error("Database query failed",
				"component", "http_handler",
				"endpoint", "/v1/list",
				"error", err.Error(),
				"query", "SELECT id, name FROM shopping_list_items LIMIT 1")
			return c.Status(500).JSON(fiber.Map{"error": "database error"})
		}

		return c.JSON(fiber.Map{"id": id, "name": name})
	}))

	// Test endpoint for database connectivity
	apiRoutes.Get("/test", withTransaction(db, func(c *fiber.Ctx, tx pgx.Tx) error {
		// Simple database connectivity test
		var result int
		err := tx.QueryRow(c.UserContext(), "SELECT 1").Scan(&result)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Database connectivity test failed")
		}

		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "Database connectivity test passed",
			"result":  result,
		})
	}))

	// Error test endpoint
	apiRoutes.Get("/error", func(c *fiber.Ctx) error {
		return errors.New("test error endpoint")
	})
}

type withTransactionHandler func(c *fiber.Ctx, tx pgx.Tx) error

func withTransaction(db postgres.DB, handler withTransactionHandler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()

		tx, err := db.Begin(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fiber.ErrRequestTimeout
			}
			return err
		}

		err = handler(c, tx)
		ctx, cancel = context.WithTimeout(c.UserContext(), 1*time.Second)
		defer cancel()
		if err != nil || c.Response().StatusCode() >= 400 {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				slog.Error("failed to rollback transaction", slog.String("error", rollbackErr.Error()))
			}
		} else {
			if commitErr := tx.Commit(ctx); commitErr != nil {
				slog.Error("failed to commit transaction", slog.String("error", commitErr.Error()))
			}
		}

		return err
	}
}

func withMetrics(db postgres.DB, handler fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := handler(c)

		durationMs := float64(time.Since(start).Milliseconds())

		if httpRequestsCounter != nil {
			httpRequestsCounter.Add(c.UserContext(), 1,
				api.WithAttributes(
					attribute.String("method", c.Method()),
					attribute.String("path", c.Route().Path),
					attribute.Int("status_code", c.Response().StatusCode()),
				),
			)
		}

		if httpRequestHistogram != nil {
			httpRequestHistogram.Record(c.UserContext(), durationMs,
				api.WithAttributes(
					attribute.String("method", c.Method()),
					attribute.String("path", c.Route().Path),
					attribute.Int("status_code", c.Response().StatusCode()),
				),
			)
		}

		return err
	}
}
