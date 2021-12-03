package controllers

import (
	"net/http"
	"strings"

	"github.com/penguin-statistics/backend-next/internal/repos"
	"github.com/penguin-statistics/backend-next/internal/server"
	"github.com/penguin-statistics/backend-next/internal/utils"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
)

type ItemController struct {
	repo  *repos.ItemRepo
	redis *redis.Client
}

func RegisterItemController(v3 *server.V3, repo *repos.ItemRepo, redis *redis.Client) {
	c := &ItemController{
		repo:  repo,
		redis: redis,
	}

	v3.Get("/items", c.GetItems)
	v3.Get("/items/:itemId", buildSanitizer(utils.NonNullString, utils.IsInt), c.GetItemById)
}

func buildSanitizer(sanitizer ...func(string) bool) func(ctx *fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		itemId := strings.TrimSpace(ctx.Params("itemId"))

		for _, sanitizer := range sanitizer {
			if !sanitizer(itemId) {
				return fiber.NewError(http.StatusBadRequest, "invalid or missing itemId")
			}
		}

		return ctx.Next()
	}
}

// GetItems godoc
// @Summary      Get all Items
// @Description  Get all Items
// @Tags         Item
// @Produce      json
// @Success      200     {array}  models.PItem{name=models.I18nString,existence=models.Existence,keywords=models.Keywords}
// @Failure      500     {object}  errors.PenguinError "An unexpected error occurred"
// @Router       /v3/items [GET]
func (c *ItemController) GetItems(ctx *fiber.Ctx) error {
	items, err := c.repo.GetItems(ctx.Context())
	if err != nil {
		return err
	}

	return ctx.JSON(items)
}

// GetItemById godoc
// @Summary      Get an Item with numerical ID
// @Description  Get an Item using the item's numerical ID
// @Tags         Item
// @Produce      json
// @Param        itemId  path      int  true  "Numerical Item ID"
// @Success      200     {object}  models.PItem{name=models.I18nString,existence=models.Existence,keywords=models.Keywords}
// @Failure      400     {object}  errors.PenguinError "Invalid or missing itemId. Notice that this shall be the **numerical ID** of the item, instead of the previously used string form **arkItemId** of the item."
// @Failure      500     {object}  errors.PenguinError "An unexpected error occurred"
// @Router       /v3/items/{itemId} [GET]
func (c *ItemController) GetItemById(ctx *fiber.Ctx) error {
	itemId := ctx.Params("itemId")

	item, err := c.repo.GetItemById(ctx.Context(), itemId)
	if err != nil {
		return err
	}

	return ctx.JSON(item)
}