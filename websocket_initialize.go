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
			return handleUpgradeError(c, ERROR_INVALID_POOL_ID)
		}

		c.Locals("poolid", pool_id)

		return c.Next()
	}

	return handleUpgradeError(c, ERROR_WEBSOCKET_UPGRADE)
}
