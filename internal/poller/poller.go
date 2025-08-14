package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"api-starter/internal/config"

	"github.com/redis/go-redis/v9"
)

type Link struct {
	Type  string  `json:"type"`
	Label *string `json:"label,omitempty"`
	URL   string  `json:"url"`
}

type Profile struct {
	URL          *string `json:"url"`
	ChainID      string  `json:"chainId"`
	TokenAddress string  `json:"tokenAddress"`
	Icon         *string `json:"icon"`
	Header       *string `json:"header"`
	OpenGraph    *string `json:"openGraph"`
	Description  *string `json:"description"`
	Links        []Link  `json:"links"`
}

type response struct {
	Profiles []Profile `json:"profiles"`
}

const etagKey = "dex:latest:etag"

func Run(ctx context.Context, rdb *redis.Client, cfg config.Config) error {
	client := &http.Client{Timeout: 10 * time.Second}

	// daily clear at midnight UTC
	go scheduleDailyClear(ctx, rdb, cfg)

	for {
		if err := tick(ctx, rdb, cfg, client); err != nil {
			log.Printf("poll error: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(cfg.PollIntervalSec) * time.Second):
		}
	}
}

func tick(ctx context.Context, rdb *redis.Client, cfg config.Config, httpc *http.Client) error {
	currETag, _ := rdb.Get(ctx, etagKey).Result()
	req, _ := http.NewRequestWithContext(ctx, "GET", cfg.DexURL, nil)
	if currETag != "" {
		req.Header.Set("If-None-Match", currETag)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dex http %d: %s", resp.StatusCode, string(b))
	}

	body, _ := io.ReadAll(resp.Body)
	if et := resp.Header.Get("ETag"); et != "" {
		_ = rdb.Set(ctx, etagKey, et, 0).Err()
	}

	// Dexscreener returns a top-level JSON array for latest profiles
	var arr []Profile
	if err := json.Unmarshal(body, &arr); err != nil {
		// fallback to object with profiles
		var out response
		if err2 := json.Unmarshal(body, &out); err2 != nil {
			return err
		}
		arr = out.Profiles
	}

	now := time.Now().UnixMilli()
	pipe := rdb.Pipeline()
	for _, p := range arr {
		if p.ChainID != cfg.PollerChain {
			continue
		}
		addr := p.TokenAddress

		linksJSON := "[]"
		if len(p.Links) > 0 {
			tmp := make([]map[string]string, 0, len(p.Links))
			for _, l := range p.Links {
				m := map[string]string{
					"type": l.Type,
					"url":  l.URL,
				}
				if l.Label != nil && *l.Label != "" {
					m["label"] = *l.Label
				}
				tmp = append(tmp, m)
			}
			b, _ := json.Marshal(tmp)
			linksJSON = string(b)
		}

		h := map[string]interface{}{
			"chainId":      p.ChainID,
			"tokenAddress": addr,
			"url":          deref(p.URL),
			"icon":         deref(p.Icon),
			"header":       deref(p.Header),
			"openGraph":    deref(p.OpenGraph),
			"description":  deref(p.Description),
			"links":        linksJSON,
			"last_seen":    now,
		}
		pipe.HSet(ctx, "token:"+addr, h)
		if cfg.TokenTTLHours > 0 {
			pipe.Expire(ctx, "token:"+addr, time.Duration(cfg.TokenTTLHours)*time.Hour)
		}
		pipe.ZAdd(ctx, "z:tokens:latest", redis.Z{Score: float64(now), Member: addr})
		pipe.ZAdd(ctx, "z:"+cfg.PollerChain+":latest", redis.Z{Score: float64(now), Member: addr})
	}
	_, err = pipe.Exec(ctx)
	return err
}

// daily clear at midnight UTC (token:* hashes, zsets, etag)
func scheduleDailyClear(ctx context.Context, rdb *redis.Client, cfg config.Config) {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	delay := time.Until(next)
	log.Printf("daily clear scheduled at %s UTC (in %s)", next.Format(time.RFC3339), delay.Truncate(time.Second))

	t := time.NewTimer(delay)
	for {
		select {
		case <-ctx.Done():
			if !t.Stop() {
				<-t.C
			}
			return
		case <-t.C:
			start := time.Now()
			cleared := clearAppKeys(ctx, rdb, cfg)
			log.Printf("daily clear done: %d keys removed in %s", cleared, time.Since(start).Truncate(time.Millisecond))
			t.Reset(24 * time.Hour)
		}
	}
}

func clearAppKeys(ctx context.Context, rdb *redis.Client, cfg config.Config) int64 {
	var total int64
	total += scanAndDel(ctx, rdb, "token:*")
	keys := []string{"z:tokens:latest", "z:" + cfg.PollerChain + ":latest", etagKey}
	if n, err := rdb.Del(ctx, keys...).Result(); err == nil {
		total += n
	} else {
		log.Printf("clear del err: %v", err)
	}
	return total
}

func scanAndDel(ctx context.Context, rdb *redis.Client, pattern string) int64 {
	var cursor uint64
	var total int64
	for {
		keys, c, err := rdb.Scan(ctx, cursor, pattern, 1000).Result()
		if err != nil {
			log.Printf("scan err for %s: %v", pattern, err)
			break
		}
		cursor = c
		if len(keys) > 0 {
			if n, err := rdb.Del(ctx, keys...).Result(); err == nil {
				total += n
			} else {
				log.Printf("del err: %v", err)
			}
		}
		if cursor == 0 {
			break
		}
	}
	return total
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
