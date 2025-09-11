package postgres

import (
	"context"
	"fmt"
	"github.com/PocketPalCo/shopping-service/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
	Close()
}

func Init(config *config.Config) (*pgxpool.Pool, error) {
	conn, err := pgxpool.New(context.Background(), config.DbConnectionString())
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %v", err)
	}

	if config.DbMaxConnections > 0 && config.DbMaxConnections <= 2147483647 {
		conn.Config().MaxConns = int32(config.DbMaxConnections)
	}
	conn.Config().MinConns = 5

	return conn, nil
}
