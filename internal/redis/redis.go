package redis

import (
	"context"
	"time"

	"api-starter/internal/config"
	"github.com/redis/go-redis/v9"
)

func NewClient(cfg config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, DB: cfg.RedisDB})
}

func LuaEval(ctx context.Context, rdb *redis.Client, script string, keys []string, args ...interface{}) *redis.Cmd {
	return rdb.Eval(ctx, script, keys, args...)
}

func SetNX(ctx context.Context, rdb *redis.Client, key string, ttlSec int) (bool, error) {
	return rdb.SetNX(ctx, key, "1", time.Duration(ttlSec)*time.Second).Result()
}
