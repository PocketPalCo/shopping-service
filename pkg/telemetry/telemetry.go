package telemetry

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"log/slog"
	"time"
)

func InitTelemetry(provider *metric.MeterProvider, db *pgxpool.Pool) error {
	err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second))
	if err != nil {
		return err
	}

	meter := provider.Meter("pgxpool")

	type PgxpoolMeter struct {
		AcquireCount            api.Int64ObservableGauge
		AcquireDuration         api.Int64ObservableGauge
		AcquiredConns           api.Int64ObservableGauge
		CanceledAcquireCount    api.Int64ObservableGauge
		ConstructingConns       api.Int64ObservableGauge
		EmptyAcquireCount       api.Int64ObservableGauge
		IdleConns               api.Int64ObservableGauge
		MaxConns                api.Int64ObservableGauge
		MaxIdleDestroyCount     api.Int64ObservableGauge
		MaxLifetimeDestroyCount api.Int64ObservableGauge
		NewConnsCount           api.Int64ObservableGauge
		TotalConns              api.Int64ObservableGauge
	}

	var pgxMeter PgxpoolMeter

	if pgxMeter.AcquireCount, err = meter.Int64ObservableGauge("pgxpool.acquire_count",
		api.WithDescription("The cumulative count of successful acquires from the pool.")); err != nil {
		slog.Error("Error creating AcquireCount gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.AcquireDuration, err = meter.Int64ObservableGauge("pgxpool.acquire_duration",
		api.WithDescription("The total duration of all successful acquires from the pool.")); err != nil {
		slog.Error("Error creating AcquireDuration gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.AcquiredConns, err = meter.Int64ObservableGauge("pgxpool.acquired_conns",
		api.WithDescription("The number of currently acquired connections in the pool.")); err != nil {
		slog.Error("Error creating AcquiredConns gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.CanceledAcquireCount, err = meter.Int64ObservableGauge("pgxpool.canceled_acquire_count",
		api.WithDescription("The cumulative count of acquires from the pool that were canceled by a context.")); err != nil {
		slog.Error("Error creating CanceledAcquireCount gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.ConstructingConns, err = meter.Int64ObservableGauge("pgxpool.constructed_conns",
		api.WithDescription("The number of conns with construction in progress in the pool.")); err != nil {
		slog.Error("Error creating ConstructedConns gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.EmptyAcquireCount, err = meter.Int64ObservableGauge("pgxpool.empty_acquire_count",
		api.WithDescription("The cumulative count of successful acquires from the pool that waited for a resource to be released or constructed because the pool was empty.")); err != nil {
		slog.Error("Error creating EmptyAcquireCount gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.IdleConns, err = meter.Int64ObservableGauge("pgxpool.idle_conns",
		api.WithDescription("The number of currently idle conns in the pool.")); err != nil {
		slog.Error("Error creating IdleConns gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.MaxConns, err = meter.Int64ObservableGauge("pgxpool.max_conns",
		api.WithDescription("The maximum size of the pool.")); err != nil {
		slog.Error("Error creating MaxConns gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.MaxIdleDestroyCount, err = meter.Int64ObservableGauge("pgxpool.max_idle_destroy_count",
		api.WithDescription("The cumulative count of connections destroyed because they exceeded MaxConnIdleTime.")); err != nil {
		slog.Error("Error creating MaxIdleDestroyCount gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.MaxLifetimeDestroyCount, err = meter.Int64ObservableGauge("pgxpool.max_lifetime_destroy_count",
		api.WithDescription("The cumulative count of connections destroyed because they exceeded MaxConnLifetime.")); err != nil {
		slog.Error("Error creating MaxLifetimeDestroyCount gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.NewConnsCount, err = meter.Int64ObservableGauge("pgxpool.new_conns_count",
		api.WithDescription("The cumulative count of new connections opened.")); err != nil {
		slog.Error("Error creating NewConnsCount gauge", slog.String("error", err.Error()))
	}

	if pgxMeter.TotalConns, err = meter.Int64ObservableGauge("pgxpool.total_conns",
		api.WithDescription("The total number of resources currently in the pool. The value is the sum of ConstructingConns, AcquiredConns, and IdleConns.")); err != nil {
		slog.Error("Error creating TotalConns gauge", slog.String("error", err.Error()))
	}

	if _, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
		stat := db.Stat()
		o.ObserveInt64(pgxMeter.AcquireCount, stat.AcquireCount())
		o.ObserveInt64(pgxMeter.AcquireDuration, int64(stat.AcquireDuration()))
		o.ObserveInt64(pgxMeter.AcquiredConns, int64(stat.AcquiredConns()))
		o.ObserveInt64(pgxMeter.CanceledAcquireCount, stat.CanceledAcquireCount())
		o.ObserveInt64(pgxMeter.ConstructingConns, int64(stat.ConstructingConns()))
		o.ObserveInt64(pgxMeter.EmptyAcquireCount, stat.EmptyAcquireCount())
		o.ObserveInt64(pgxMeter.IdleConns, int64(stat.IdleConns()))
		o.ObserveInt64(pgxMeter.MaxConns, int64(stat.MaxConns()))
		o.ObserveInt64(pgxMeter.MaxIdleDestroyCount, stat.MaxIdleDestroyCount())
		o.ObserveInt64(pgxMeter.MaxLifetimeDestroyCount, stat.MaxLifetimeDestroyCount())
		o.ObserveInt64(pgxMeter.NewConnsCount, stat.NewConnsCount())
		o.ObserveInt64(pgxMeter.TotalConns, int64(stat.TotalConns()))
		return nil
	}, pgxMeter.AcquireCount, pgxMeter.AcquireDuration, pgxMeter.AcquiredConns, pgxMeter.CanceledAcquireCount,
		pgxMeter.ConstructingConns, pgxMeter.EmptyAcquireCount, pgxMeter.IdleConns, pgxMeter.MaxConns, pgxMeter.MaxIdleDestroyCount,
		pgxMeter.MaxLifetimeDestroyCount, pgxMeter.NewConnsCount, pgxMeter.TotalConns); err != nil {
		slog.Error("Error updating pgxpool gauges", slog.String("error", err.Error()))
	}

	return nil
}
