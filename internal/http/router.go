package http

import (
	"strconv"

	"api-starter/internal/config"
	mid "api-starter/internal/http/middleware"
	"api-starter/internal/tokens"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

type Server struct{ *fiber.App }

func NewServer(cfg config.Config, rdb *redis.Client, _ interface{}) *Server {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(mid.RateLimit(cfg, rdb))
	app.Use(mid.Idempotency(cfg, rdb))

	// Dexscreener-compatible endpoint: /token-profiles/latest/v1
	// Served from Redis repo with pagination
	app.Get("/token-profiles/latest/v1", func(c *fiber.Ctx) error {
		// Enforce Solana-only responses (ignore chain query param)
		chain := cfg.PollerChain
		if chain == "" {
			chain = "solana"
		}
		// pagination params
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		offset, _ := strconv.Atoi(c.Query("offset", "0"))
		if offset < 0 {
			offset = 0
		}
		repo := tokens.NewRepo(rdb, 72)
		items, err := repo.ListLatestByChainOut(c.Context(), chain, offset, limit)
		if err != nil {
			return fiber.NewError(500, err.Error())
		}
		return c.JSON(items)
	})

	// Only `/token-profiles/latest/v1` is exposed

	return &Server{app}
}

// filterByChain reduces Dexscreener response array to only items with matching chainId.
// Returns filtered JSON bytes and true if filtering succeeded, otherwise false.
// (no additional helpers)
