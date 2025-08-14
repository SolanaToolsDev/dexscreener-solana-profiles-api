package tokens

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

type Handler struct{ repo *Repo }

func NewHandler(rdb *redis.Client) *Handler { return &Handler{repo: NewRepo(rdb, 72)} }

// Repo exposes the underlying repository for router reuse
func (h *Handler) Repo() *Repo { return h.repo }

func (h *Handler) Create(c *fiber.Ctx) error {
	var in CreateInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if in.TokenAddress == "" || in.ChainID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "chainId and tokenAddress required")
	}
	err := h.repo.rdb.HSet(
		context.Background(),
		"token:"+in.TokenAddress,
		map[string]interface{}{
			"chainId":      in.ChainID,
			"tokenAddress": in.TokenAddress,
			"url":          deref(in.URL),
			"icon":         deref(in.Icon),
			"header":       deref(in.Header),
			"openGraph":    deref(in.OpenGraph),
			"description":  deref(in.Description),
			"links":        "[]",
		},
	).Err()
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	return c.SendStatus(fiber.StatusCreated)
}

func (h *Handler) GetOne(c *fiber.Ctx) error {
	mint := c.Params("mint")
	if mint == "" {
		return fiber.NewError(fiber.StatusBadRequest, "mint required")
	}
	o, err := h.repo.GetOut(context.Background(), mint)
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	if o.TokenAddress == "" {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return c.JSON(o)
}

func (h *Handler) List(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	items, err := h.repo.ListLatestOut(context.Background(), limit)
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	return c.JSON(items)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
