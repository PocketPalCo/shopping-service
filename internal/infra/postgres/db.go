package postgres

import (
	"context"
	"fmt"
	"github.com/PocketPalCo/shopping-service/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Init(config config.Config) (*pgxpool.Pool, error) {
	conn, err := pgxpool.New(context.Background(), config.DbConnectionString())
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %v", err)
	}

	conn.Config().MaxConns = int32(config.DbMaxConnections)
	conn.Config().MinConns = 5

	return conn, nil
}
