## API Starter (Go + Redis) for Dexscreener-style token profiles

### What this service does
- Mirrors Dexscreener’s latest token profiles endpoint and serves it from your own API.
  - Upstream: `https://api.dexscreener.com/token-profiles/latest/v1`
  - Your API: `GET /token-profiles/latest/v1`
  - ETag-aware requests to Dexscreener with Redis-backed caching to minimize upstream calls and stay within the 60 rpm limit.
  - Returns JSON in Dexscreener-compatible shape. By default, results are filtered to a single chain (Solana), configurable via env or query.
- Exposes basic token CRUD-style endpoints backed by Redis for simple storage and a “latest feed”.
- Provides middleware for IP rate limiting and request idempotency via Redis.
- Production-ready service wiring with systemd and Caddy reverse proxy (TLS via Let’s Encrypt) for domain `tzen.ai`.

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
   │  ├─ router.go                # Fiber server, routes, caching proxy
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
- `POLL_INTERVAL_SEC` (default `10`)
- `DEX_URL` (default Dexscreener latest profiles URL)
- `POLLER_CHAIN` (default `solana`) – also controls default chain filter for the proxy endpoint
- `TOKEN_TTL_HOURS` (default `72`)


### Endpoints
- Liveness & readiness
  - `GET /healthz` → `ok`
  - `GET /readyz` → checks Redis, returns `ready`

- Dexscreener mirror (cached + filtered)
  - `GET /token-profiles/latest/v1`
  - Behavior:
    - On each request, use Redis-cached body (fresh within 10s) if available.
    - Otherwise fetch from `DEX_URL` with `If-None-Match` using the stored ETag.
    - On 304 Not Modified, serve cached body.
    - On 200 OK, store new ETag and body in Redis.
    - Response is filtered to a single chain ID (default `POLLER_CHAIN` or `solana`).
    - Override via `?chain=<chainId>` query, e.g. `?chain=solana`.
  - Redis keys used:
    - `dex:latest:etag` – last seen ETag
    - `dex:latest:body` – last response body (raw JSON)
    - `dex:latest:ts` – last refresh unix seconds

- Tokens (simple storage)
  - `POST /tokens` – create minimal token record (idempotent via `Idempotency-Key` header; TTL controlled by `TOKEN_TTL_HOURS` if using the poller)
  - `GET /tokens?limit=50` – list latest tokens
  - `GET /tokens/:mint` – get one token
  - Helper feed (Dex-like array of ProfileOut):
    - `GET /feed/latest?limit=50`

- Repo-backed alias (from Redis only)
  - `GET /token-profiles/latest/by-chain?limit=50`
  - Returns latest profiles for the default chain (Solana) from Redis ZSETs (no upstream call)



### Middleware
- Rate limit: token-bucket per IP stored in Redis, script-driven (Lua) with RPS and burst from env.
- Idempotency: `Idempotency-Key` header stored with TTL in Redis using `SetNX`; duplicates within TTL → 409 Conflict.

### Poller mode
- Command: `go run ./cmd/app -mode=poller` (or `make poller`)
- Periodically fetches `DEX_URL` with ETag handling, writes token hashes and ZSETs into Redis, and schedules a daily clear of app keys at midnight UTC.
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

Example queries:
```
curl -s http://127.0.0.1:3000/healthz
curl -s http://127.0.0.1:3000/readyz
curl -s 'http://127.0.0.1:3000/token-profiles/latest/v1?chain=solana'
curl -s 'http://127.0.0.1:3000/feed/latest?limit=50'
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
- Dex mirror path returns the upstream body with minimal processing, but filtered to the requested chain for consistency with your needs.
- ETag + 10s freshness window avoids hammering Dexscreener and respects their documented rate limit.
- All rate limiting and idempotency state is stored in Redis to keep the app stateless.

### Environment defaults recap
- Default chain: `solana` (via `POLLER_CHAIN` or implicit fallback).
- Rate limit: `RATE_LIMIT_RPS=5`, `RATE_LIMIT_BURST=10`.
- Idempotency TTL: `IDEMP_TTL_SEC=60`.

### License
MIT. See `LICENSE` for details.


