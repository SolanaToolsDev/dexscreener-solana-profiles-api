## API Starter (Go + Redis) for Dexscreener-style token profiles

### What this service does
- Serves a Dexscreener-compatible "latest token profiles" feed from your own API.
  - Upstream source polled by the background poller: `DEX_URL` (defaults to Dexscreener)
  - Your API: `GET /token-profiles/latest/v1`
  - Returns JSON in Dexscreener-compatible shape, filtered to a single chain (default `solana`).
  - Supports pagination with `limit` (max 200) and `offset` (zero-based).
- Uses Redis to store profile hashes and a per-chain sorted set of latest tokens; the API reads from Redis only (no upstream call per request).
- Provides middleware for IP rate limiting and request idempotency via Redis.
- Production-ready wiring notes for systemd and Caddy (TLS via Let’s Encrypt) for `tzen.ai`.

### Tech stack
- Go 1.22, Fiber v2
- Redis (go-redis v9)
- Caddy (HTTPS, reverse proxy)

### Directory layout
```
.
├─ cmd/app/main.go                # Entrypoint; -mode=api or -mode=poller
└─ internal/
   ├─ config/config.go            # Env config loader
   ├─ redis/redis.go              # Redis client helpers & Lua eval
   ├─                            
   ├─ http/
   │  ├─ router.go                # Fiber server, routes, paginated endpoint
   │  └─ middleware/
   │     ├─ ratelimit.go          # IP token-bucket using Redis + Lua
   │     └─ idempotency.go        # Idempotency via Redis SetNX
   ├─ poller/poller.go            # Periodic Dex fetcher + Redis persistence
   └─ tokens/
      ├─ model.go
      ├─ repo.go                  # Redis-backed token repository
      ├─ service.go
      └─ handler.go               # HTTP handlers for tokens
```

### Configuration (.env.example)
- `REDIS_ADDR` (default `localhost:6379`)
- `REDIS_DB` (default `0`)
- `PORT` (default `3000`)
  (No Telegram or Asynq config; those features were removed.)
- `IDEMP_TTL_SEC` (default `60`)
- `RATE_LIMIT_RPS` (default `5`)
- `RATE_LIMIT_BURST` (default `10`)
- `POLL_INTERVAL_SEC` (default `10`) – how often the poller checks upstream
- `DEX_URL` (default Dexscreener latest profiles URL)
- `POLLER_CHAIN` (default `solana`) – enforced chain for both poller and API output
- `TOKEN_TTL_HOURS` (default `72`) – rolling retention window for latest sets and token hashes


### Endpoint
- `GET /token-profiles/latest/v1`
  - Query params:
    - `limit` (int, 1..200; default 50)
    - `offset` (int, >=0; default 0)
  - Always filtered to the chain set by `POLLER_CHAIN` (default `solana`).
  - Returns an array of profiles in Dexscreener-compatible shape.



### Middleware
- Rate limit: token-bucket per IP stored in Redis, script-driven (Lua) with RPS and burst from env.
- Idempotency: `Idempotency-Key` header stored with TTL in Redis using `SetNX`; duplicates within TTL → 409 Conflict.

### Poller mode
- Command: `go run ./cmd/app -mode=poller` (or `make poller`)
- Periodically fetches `DEX_URL` with ETag handling (If-None-Match), writes token hashes and ZSETs into Redis.
- Rolling retention: after each poll, trims `z:tokens:latest` and `z:<chain>:latest` by score to keep only entries within `TOKEN_TTL_HOURS`.
- Keys used by poller:
  - `token:<address>` – Redis hash with fields matching Dex output
  - `z:tokens:latest` – global recency ZSET
  - `z:<chain>:latest` – per-chain recency ZSET
  - `dex:latest:etag` – ETag for upstream conditional requests

### Running locally
```
make tidy
make api     # runs the Fiber API on $PORT (default 3000)
make poller  # runs the poller (optional)
```

Example query:
```
curl -s 'http://127.0.0.1:3000/token-profiles/latest/v1?limit=50&offset=0'
```

### Production notes
- A systemd unit named `api-starter.service` is included in deployment (installed on this server), running the API on port 3000.
  - Manage: `systemctl status|restart api-starter`
  - Logs: `journalctl -u api-starter -f`
- Caddy reverse proxies `tzen.ai → 127.0.0.1:3000` with automatic HTTPS.
- UFW is configured to allow ports 80/443.

### Production hardening (recommended for high traffic)
- Scale & performance
  - Run multiple API instances behind a load balancer/CDN (e.g., Cloudflare) for TLS offload, caching, and WAF
  - Tune Fiber: timeouts, keep-alives; consider Prefork on large hosts
  - Add single-flight on Dex fetch to avoid cache stampedes; keep 10s freshness window
- Redis reliability
  - Use managed HA Redis (Sentinel/Cluster) with proper sizing; enable AOF and set `maxmemory` with a suitable eviction policy
  - Expose Redis metrics; set connection pool sizes and operation timeouts
- Poller robustness
  - Add retry/backoff with jitter and a simple circuit breaker for upstream errors
  - Track last-success timestamp and alert if stale beyond SLO
- Observability
  - Add Prometheus `/metrics` (HTTP durations, Redis ops, poller runs) and alerting (5xx rate, latency, poller staleness)
  - Use structured JSON logs with request IDs and upstream/cache hit/miss fields
- Security
  - Run API under a non-root user; harden systemd (NoNewPrivileges, ProtectSystem=strict, PrivateTmp, resource limits)
  - In Caddy, enable HSTS and tune timeouts; configure CORS policy as needed
- API hygiene
  - Enforce strict pagination and parameter validation; return 429 with `Retry-After` when rate-limited
  - Set Cache-Control/ETag on your responses to leverage client/CDN caching
- Deployment & DR
  - CI/CD with tests (go test/vet/staticcheck), containerization, and blue/green or canary deploys
  - Backup Redis (AOF/RDB) and test restores; keep infra-as-code for Caddy and systemd

### Design details
- The background poller handles upstream ETag/HTTP caching and persistence into Redis.
- The API endpoint reads from Redis ZSETs and serves a paginated, chain-filtered view (no upstream calls per request).
- All rate limiting and idempotency state is stored in Redis to keep the app stateless.

### Environment defaults recap
- Default chain: `solana` (via `POLLER_CHAIN` or implicit fallback).
- Rate limit: `RATE_LIMIT_RPS=5`, `RATE_LIMIT_BURST=10`.
- Idempotency TTL: `IDEMP_TTL_SEC=60`.

### License
MIT. See `LICENSE` for details.


