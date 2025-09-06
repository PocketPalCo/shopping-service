package redis

import (
	"context"
	"fmt"
	"github.com/PocketPalCo/shopping-service/config"
	"github.com/go-redis/redis/v8"
	"time"
)

func NewRedisClient(cfg *config.Config) (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort)
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDb,
		Username: cfg.RedisUser,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	return client, nil
}
