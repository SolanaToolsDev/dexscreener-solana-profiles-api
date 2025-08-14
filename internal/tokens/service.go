package tokens

import (
	"github.com/redis/go-redis/v9"
)

type Service struct{ repo *Repo }

func NewService(rdb *redis.Client, ttlHours int) *Service { return &Service{repo: NewRepo(rdb, ttlHours)} }

// (Manual create kept minimal; poller populates full records)
type CreateInput struct {
	ChainID      string  `json:"chainId"`
	TokenAddress string  `json:"tokenAddress"`
	URL          *string `json:"url,omitempty"`
	Icon         *string `json:"icon,omitempty"`
	Header       *string `json:"header,omitempty"`
	OpenGraph    *string `json:"openGraph,omitempty"`
	Description  *string `json:"description,omitempty"`
}
