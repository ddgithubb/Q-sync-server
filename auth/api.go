package auth

import "github.com/gofiber/fiber/v2"

func AttatchAuthRoutes(app *fiber.App) {
	group := app.Group("/auth")
	group.Post("/begin-register", BeginRegistrationApi)
	group.Post("/finish-register", FinishRegisterApi)
}

func BadRequestResponse(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusBadRequest)
}

func BeginRegistrationApi(c *fiber.Ctx) error {
	req := new(BeginRegisterRequest)

	if err := c.BodyParser(req); err != nil {
		return BadRequestResponse(c)
	}

	if req.DisplayName == "" {
		return BadRequestResponse(c)
	}

	credentialCreation, deviceID, success := BeginRegistration(req.DisplayName)
	if !success {
		return BadRequestResponse(c)
	}

	return c.JSON(BeginRegisterResponse{
		DeviceID:           deviceID,
		CredentialCreation: credentialCreation,
	})
}

func FinishRegisterApi(c *fiber.Ctx) error {
	req := new(FinishRegisterRequest)

	if err := c.BodyParser(req); err != nil {
		return BadRequestResponse(c)
	}

	if req.DeviceInfo.DeviceName == "" {
		return BadRequestResponse(c)
	}

	success := FinishRegistration(req.DeviceInfo, req.CredentialData)
	if !success {
		return BadRequestResponse(c)
	}

	return c.SendStatus(fiber.StatusOK)
}
