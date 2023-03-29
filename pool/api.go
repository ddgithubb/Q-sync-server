package pool

import (
	"sync-server/auth"
	"sync-server/store"

	"github.com/gofiber/fiber/v2"
)

func AttatchPoolRoutes(app *fiber.App) {
	group := app.Group("/pool", auth.AuthTokenMiddleware)
	group.Post("/create", CreatePoolApi)
	group.Post("/join", JoinPoolApi)

	poolGroup := group.Group("/:poolid", PoolGroupMiddleware)
	poolGroup.Post("/leave", LeavePoolApi)
	poolGroup.Post("/create-invite", CreateInviteToPoolApi)
}

func OKResponse(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusOK)
}

func BadRequestResponse(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusBadRequest)
}

func PoolGroupMiddleware(c *fiber.Ctx) error {
	poolID := c.Params("poolid")
	if poolID == "" {
		return BadRequestResponse(c)
	}
	return c.Next()
}

func CreatePoolApi(c *fiber.Ctx) error {
	req := new(CreatePoolRequest)

	if err := c.BodyParser(req); err != nil {
		return BadRequestResponse(c)
	}

	if req.PoolName == "" || len(req.PoolName) > store.MAX_POOL_NAME_LENGTH {
		return BadRequestResponse(c)
	}

	deviceID := c.Locals("deviceid").(string)
	userDevice, err := store.GetUserDevice(deviceID)
	if err != nil {
		return BadRequestResponse(c)
	}

	pool, ok := store.NewPool(req.PoolName, userDevice.PoolUserInfo())
	if !ok {
		return BadRequestResponse(c)
	}

	return c.JSON(CreatePoolResponse{
		PoolInfo: pool.GetPoolInfo(),
	})
}

func JoinPoolApi(c *fiber.Ctx) error {
	req := new(JoinPoolRequest)

	if err := c.BodyParser(req); err != nil {
		return BadRequestResponse(c)
	}

	if req.InviteLink == "" {
		return BadRequestResponse(c)
	}

	poolID, ok := UseInviteLink(req.InviteLink)
	if !ok {
		return BadRequestResponse(c)
	}

	deviceID := c.Locals("deviceid").(string)
	userDevice, err := store.GetUserDevice(deviceID)
	if err != nil {
		return BadRequestResponse(c)
	}

	poolInfo, ok := JoinPool(poolID, userDevice.PoolUserInfo())
	if !ok {
		return BadRequestResponse(c)
	}

	return c.JSON(JoinPoolResponse{
		PoolInfo: poolInfo,
	})
}

func LeavePoolApi(c *fiber.Ctx) error {
	poolID := c.Params("poolid")
	userID := c.Locals("userid").(string)
	deviceID := c.Locals("deviceid").(string)

	success := LeavePool(poolID, userID, deviceID)

	return c.JSON(LeavePoolResponse{
		Success: success,
	})
}

func CreateInviteToPoolApi(c *fiber.Ctx) error {
	poolID := c.Params("poolid")
	userID := c.Locals("userid").(string)

	exists := UserExistsInPool(poolID, userID)
	if !exists {
		return BadRequestResponse(c)
	}

	inviteLink, ok := GenerateAndStoreNewInviteLinkDefault(poolID)
	if !ok {
		return BadRequestResponse(c)
	}

	return c.JSON(CreateInviteToPoolResponse{
		InviteLink: inviteLink,
	})
}
