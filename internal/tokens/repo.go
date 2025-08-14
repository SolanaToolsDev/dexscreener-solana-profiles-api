package tokens

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

type Repo struct {
	rdb      *redis.Client
	ttlHours int
}

func NewRepo(rdb *redis.Client, ttlHours int) *Repo { return &Repo{rdb: rdb, ttlHours: ttlHours} }

func (r *Repo) key(addr string) string { return "token:" + addr }

func (r *Repo) GetOut(ctx context.Context, addr string) (ProfileOut, error) {
	m, err := r.rdb.HGetAll(ctx, r.key(addr)).Result()
	if err != nil {
		return ProfileOut{}, err
	}
	return mapToOut(m), nil
}

func (r *Repo) ListLatestOut(ctx context.Context, offset int, limit int) ([]ProfileOut, error) {
	addrs, err := r.rdb.ZRevRange(ctx, "z:sol:latest", int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		return nil, err
	}
	pipe := r.rdb.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, 0, len(addrs))
	for _, a := range addrs {
		cmds = append(cmds, pipe.HGetAll(ctx, r.key(a)))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	out := make([]ProfileOut, 0, len(cmds))
	for _, c := range cmds {
		m := c.Val()
		// skip if hash expired or empty
		if len(m) == 0 || m["tokenAddress"] == "" {
			continue
		}
		out = append(out, mapToOut(m))
	}
	return out, nil
}

func (r *Repo) ListLatestByChainOut(ctx context.Context, chain string, offset int, limit int) ([]ProfileOut, error) {
	key := "z:" + chain + ":latest"
	addrs, err := r.rdb.ZRevRange(ctx, key, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		return nil, err
	}
	pipe := r.rdb.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, 0, len(addrs))
	for _, a := range addrs {
		cmds = append(cmds, pipe.HGetAll(ctx, r.key(a)))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	out := make([]ProfileOut, 0, len(cmds))
	for _, c := range cmds {
		m := c.Val()
		if len(m) == 0 || m["tokenAddress"] == "" {
			continue
		}
		out = append(out, mapToOut(m))
	}
	return out, nil
}

func mapToOut(m map[string]string) ProfileOut {
	po := ProfileOut{
		URL:          m["url"],
		ChainID:      m["chainId"],
		TokenAddress: m["tokenAddress"],
		Icon:         m["icon"],
		Header:       m["header"],
		OpenGraph:    m["openGraph"],
		Description:  m["description"],
	}
	// links -> []Link (nil when empty for omitempty)
	if ls := m["links"]; ls != "" && ls != "[]" {
		var raw []Link
		if err := json.Unmarshal([]byte(ls), &raw); err == nil && len(raw) > 0 {
			po.Links = raw
		}
	}
	return po
}
