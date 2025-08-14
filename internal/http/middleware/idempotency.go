package middleware

import (
	"context"
	"time"

	"api-starter/internal/config"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func Idempotency(cfg config.Config, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get("Idempotency-Key")
		if id == "" {
			return c.Next()
		}
		key := "idem:" + id
		ok, err := rdb.SetNX(context.Background(), key, "1", time.Duration(cfg.IdempTTL)*time.Second).Result()
		if err != nil {
			return fiber.NewError(500, err.Error())
		}
		if !ok {
			return fiber.NewError(fiber.StatusConflict, "duplicate request")
		}
		defer rdb.Del(context.Background(), key)
		return c.Next()
	}
}
