package middleware

import (
	"context"
	"time"

	"api-starter/internal/config"
	red "api-starter/internal/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

const rlLua = `
local tkey = KEYS[1]..":t"
local lastkey = KEYS[1]..":ts"
local now = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local tokens = tonumber(redis.call('GET', tkey) or burst)
local last = tonumber(redis.call('GET', lastkey) or now)
local delta = math.max(0, now - last) * rate / 1000
tokens = math.min(burst, tokens + delta)
if tokens < 1 then
  redis.call('SET', tkey, tokens)
  redis.call('SET', lastkey, now)
  return 0
else
  redis.call('SET', tkey, tokens - 1)
  redis.call('SET', lastkey, now)
  return 1
end`

func RateLimit(cfg config.Config, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := "rl:" + c.IP()
		now := time.Now().UnixMilli()
		res := red.LuaEval(context.Background(), rdb, rlLua, []string{key}, now, cfg.RateLimitRPS, cfg.RateLimitBurst)
		ok, err := res.Int()
		if err != nil {
			return fiber.NewError(500, err.Error())
		}
		if ok == 0 {
			return fiber.NewError(fiber.StatusTooManyRequests, "rate limited")
		}
		return c.Next()
	}
}
