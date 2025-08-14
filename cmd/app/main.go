package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"api-starter/internal/config"
	httpSrv "api-starter/internal/http"
	"api-starter/internal/poller"
	"api-starter/internal/redis"
)

func main() {
	_ = godotenv.Load()
	mode := flag.String("mode", "api", "run mode: api|poller")
	flag.Parse()

	cfg := config.Load()
	rc := redis.NewClient(cfg)
	defer rc.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch *mode {
	case "api":
		app := httpSrv.NewServer(cfg, rc, nil)
		log.Printf("API listening on :%s", cfg.Port)
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Fatalf("server error: %v", err)
		}
	case "poller":
		log.Printf("Starting poller: %s every %ds (chain=%s)", cfg.DexURL, cfg.PollIntervalSec, cfg.PollerChain)
		if err := poller.Run(ctx, rc, cfg); err != nil {
			log.Fatalf("poller: %v", err)
		}
	default:
		log.Printf("unknown -mode=%s", *mode)
		os.Exit(2)
	}
}
