package config

import (
	"os"
	"strconv"
)

type Config struct {
	RedisAddr       string
	RedisDB         int
	Port            string
	IdempTTL        int
	RateLimitRPS    int
	RateLimitBurst  int
	PollIntervalSec int
	DexURL          string
	PollerChain     string
	TokenTTLHours   int
}

func get(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
func geti(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func Load() Config {
	return Config{
		RedisAddr:       get("REDIS_ADDR", "localhost:6379"),
		RedisDB:         geti("REDIS_DB", 0),
		Port:            get("PORT", "3000"),
		IdempTTL:        geti("IDEMP_TTL_SEC", 60),
		RateLimitRPS:    geti("RATE_LIMIT_RPS", 5),
		RateLimitBurst:  geti("RATE_LIMIT_BURST", 10),
		PollIntervalSec: geti("POLL_INTERVAL_SEC", 10),
		DexURL:          get("DEX_URL", "https://api.dexscreener.com/token-profiles/latest/v1"),
		PollerChain:     get("POLLER_CHAIN", "solana"),
		TokenTTLHours:   geti("TOKEN_TTL_HOURS", 72),
	}
}
