package common

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"log/slog"
	"os"
	"time"
)

var RDB *redis.Client
var RedisEnabled = true

// InitRedisClient This function is called after init()
func InitRedisClient() (err error) {
	connString := os.Getenv("REDIS_CONN_STRING")
	if connString == "" {
		DisableRedisClient()
		if RedisRequired {
			return fmt.Errorf("REDIS_CONN_STRING is required when REDIS_REQUIRED is enabled")
		}
		slog.Info("redis disabled because REDIS_CONN_STRING is not set")
		return nil
	}
	opt, err := redis.ParseURL(connString)
	if err != nil {
		return handleRedisInitError(fmt.Errorf("parse redis connection string: %w", err))
	}
	RDB = redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = RDB.Ping(ctx).Result()
	if err != nil {
		return handleRedisInitError(fmt.Errorf("ping redis: %w", err))
	}
	RedisEnabled = true
	return nil
}

func ParseRedisOption() (*redis.Options, error) {
	connString := os.Getenv("REDIS_CONN_STRING")
	if connString == "" {
		return nil, fmt.Errorf("REDIS_CONN_STRING is not set")
	}
	opt, err := redis.ParseURL(connString)
	if err != nil {
		return nil, err
	}
	return opt, nil
}

func handleRedisInitError(err error) error {
	DisableRedisClient()
	if RedisRequired {
		return err
	}
	slog.Warn("redis disabled because initialization failed", "error", err)
	return nil
}

func DisableRedisClient() {
	RedisEnabled = false
	if RDB != nil {
		_ = RDB.Close()
		RDB = nil
	}
}
