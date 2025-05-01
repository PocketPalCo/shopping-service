package telemetry

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"log/slog"
	"time"
)

type InstrumentedPool struct {
	*pgxpool.Pool
	queryDuration api.Float64Histogram
}

func NewInstrumentedPool(provider *metric.MeterProvider, pool *pgxpool.Pool) (*InstrumentedPool, error) {
	meter := provider.Meter("db_queries")

	queryDuration, err := meter.Float64Histogram(
		"db.query_duration",
		api.WithDescription("Duration of database queries in milliseconds."),
	)
	if err != nil {
		slog.Error("Error creating query_duration histogram", slog.String("error", err.Error()))
		return nil, err
	}

	return &InstrumentedPool{
		Pool:          pool,
		queryDuration: queryDuration,
	}, nil
}

func (ip *InstrumentedPool) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	start := time.Now()
	tag, err := ip.Pool.Exec(ctx, sql, args...)
	duration := time.Since(start).Milliseconds()
	ip.queryDuration.Record(ctx, float64(duration))
	return tag, err
}

func (ip *InstrumentedPool) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	start := time.Now()
	rows, err := ip.Pool.Query(ctx, sql, args...)
	duration := time.Since(start).Milliseconds()
	ip.queryDuration.Record(ctx, float64(duration))
	return rows, err
}

func (ip *InstrumentedPool) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	start := time.Now()
	row := ip.Pool.QueryRow(ctx, sql, args...)
	duration := time.Since(start).Milliseconds()
	ip.queryDuration.Record(ctx, float64(duration))
	return row
}
