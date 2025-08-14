package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

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
	app.Get("/token-profiles/latest/v1", func(c *fiber.Ctx) error {
		const bodyKey = "dex:latest:body"
		const etagKey = "dex:latest:etag"
		const tsKey = "dex:latest:ts"
		// Enforce Solana-only responses (ignore chain query param)
		wantChain := cfg.PollerChain
		if wantChain == "" {
			wantChain = "solana"
		}

		// Serve recent cache (<=10s) immediately
		cachedBody, _ := rdb.Get(c.Context(), bodyKey).Result()
		lastTs, _ := rdb.Get(c.Context(), tsKey).Int64()
		if cachedBody != "" && time.Since(time.Unix(lastTs, 0)) <= 10*time.Second {
			// filter by chainId
			if filtered, ok := filterByChain(cachedBody, wantChain); ok {
				return c.Type("json").Send(filtered)
			}
			return c.Type("json").SendString(cachedBody)
		}

		etag, _ := rdb.Get(c.Context(), etagKey).Result()
		client := &http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequestWithContext(c.Context(), http.MethodGet, cfg.DexURL, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		resp, err := client.Do(req)
		if err != nil {
			if cachedBody != "" {
				if filtered, ok := filterByChain(cachedBody, wantChain); ok {
					return c.Type("json").Send(filtered)
				}
				return c.Type("json").SendString(cachedBody)
			}
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotModified && cachedBody != "" {
			// Update freshness timestamp
			_ = rdb.Set(c.Context(), tsKey, time.Now().Unix(), 0).Err()
			if filtered, ok := filterByChain(cachedBody, wantChain); ok {
				return c.Type("json").Send(filtered)
			}
			return c.Type("json").SendString(cachedBody)
		}
		if resp.StatusCode != http.StatusOK {
			if cachedBody != "" {
				if filtered, ok := filterByChain(cachedBody, wantChain); ok {
					return c.Type("json").Send(filtered)
				}
				return c.Type("json").SendString(cachedBody)
			}
			b, _ := io.ReadAll(resp.Body)
			return fiber.NewError(fiber.StatusBadGateway, string(b))
		}

		b, _ := io.ReadAll(resp.Body)
		if newETag := resp.Header.Get("ETag"); newETag != "" {
			_ = rdb.Set(c.Context(), etagKey, newETag, 0).Err()
		}
		_ = rdb.Set(c.Context(), bodyKey, string(b), 0).Err()
		_ = rdb.Set(c.Context(), tsKey, time.Now().Unix(), 0).Err()
		if filtered, ok := filterByChain(string(b), wantChain); ok {
			return c.Type("json").Send(filtered)
		}
		return c.Type("json").Send(b)
	})

	// liveness & readiness
	app.Get("/healthz", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/readyz", func(c *fiber.Ctx) error {
		if err := rdb.Ping(c.Context()).Err(); err != nil {
			return fiber.NewError(500, "redis not ready")
		}
		return c.SendString("ready")
	})

	tokensH := tokens.NewHandler(rdb)
	app.Post("/tokens", tokensH.Create)
	app.Get("/tokens", tokensH.List)
	app.Get("/tokens/:mint", tokensH.GetOne)

	// Array of ProfileOut, matching Dexscreener-style responses
	app.Get("/feed/latest", func(c *fiber.Ctx) error {
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		items, err := tokensH.Repo().ListLatestOut(c.Context(), limit)
		if err != nil {
			return fiber.NewError(500, err.Error())
		}
		return c.JSON(items)
	})

	// Repo-backed alias: by-chain latest (does not call upstream)
	app.Get("/token-profiles/latest/by-chain", func(c *fiber.Ctx) error {
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		// Enforce Solana-only responses (ignore chain query param)
		chain := cfg.PollerChain
		if chain == "" {
			chain = "solana"
		}
		items, err := tokensH.Repo().ListLatestByChainOut(c.Context(), chain, limit)
		if err != nil {
			return fiber.NewError(500, err.Error())
		}
		return c.JSON(items)
	})

	// broadcast endpoint removed (Telegram/Asynq feature dropped)

	return &Server{app}
}

// filterByChain reduces Dexscreener response array to only items with matching chainId.
// Returns filtered JSON bytes and true if filtering succeeded, otherwise false.
func filterByChain(raw string, chain string) ([]byte, bool) {
	if chain == "" {
		chain = "solana"
	}
	// Try root array
	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		out := make([]map[string]any, 0, len(arr))
		for _, it := range arr {
			if v, ok := it["chainId"].(string); ok && v == chain {
				out = append(out, it)
			}
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, false
		}
		return b, true
	}

	// Try object with profiles field
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		if p, ok := obj["profiles"].([]any); ok {
			out := make([]map[string]any, 0, len(p))
			for _, item := range p {
				if m, ok := item.(map[string]any); ok {
					if v, ok := m["chainId"].(string); ok && v == chain {
						out = append(out, m)
					}
				}
			}
			b, err := json.Marshal(out)
			if err != nil {
				return nil, false
			}
			return b, true
		}
	}
	return nil, false
}
