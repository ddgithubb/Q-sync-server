package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// InitializeSocket initializes websocket connection
func InitializeSocket(c *fiber.Ctx) error {

	if websocket.IsWebSocketUpgrade(c) {

		pool_id := c.Query("poolid")

		if pool_id == "" {
			return handleUpgradeError(c, 40100, pool_id)
		}

		c.Locals("poolid", pool_id)

		return c.Next()
	}

	return handleUpgradeError(c, 40000, fiber.ErrUpgradeRequired.Error())
}
