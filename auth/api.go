package auth

import (
	"sync-server/sspb"

	"github.com/gofiber/fiber/v2"
)

func AttatchAuthRoutes(app *fiber.App) {
	group := app.Group("/auth")
	group.Post("/begin-register", BeginRegistrationApi)
	group.Post("/finish-register", FinishRegistrationApi)
	group.Post("/begin-auth", BeginAuthenticateApi)
	group.Post("/finish-auth", FinishAuthenticateApi)
}

func BadRequestResponse(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusBadRequest)
}

func UnauthorizedResponse(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusUnauthorized)
}

func AuthTokenMiddleware(c *fiber.Ctx) error {
	deviceID := string(c.Request().Header.Peek("x-device-id"))
	authToken := string(c.Request().Header.Peek("x-auth-token"))

	if deviceID == "" || len(authToken) != AUTH_TOKEN_SIZE {
		return UnauthorizedResponse(c)
	}

	tokenData, verified := VerifyAuthToken(deviceID, authToken)
	if !verified {
		println("Failed to verify auth token", deviceID, authToken)
		return UnauthorizedResponse(c)
	}

	println("Verified auth token", deviceID, tokenData.UserID, authToken)

	c.Locals("deviceid", deviceID)
	c.Locals("userid", tokenData.UserID)
	return c.Next()
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

func FinishRegistrationApi(c *fiber.Ctx) error {
	req := new(FinishRegisterRequest)

	if err := c.BodyParser(req); err != nil {
		return BadRequestResponse(c)
	}

	deviceInfo := &sspb.PoolDeviceInfo{
		DeviceId:   req.DeviceID,
		DeviceType: sspb.DeviceType(req.DeviceType),
		DeviceName: req.DeviceName,
	}

	if req.DeviceName == "" {
		return BadRequestResponse(c)
	}

	pcc, err := req.CredentialData.Parse()
	if err != nil {
		return BadRequestResponse(c)
	}

	token, success := FinishRegistration(deviceInfo, pcc)
	if !success {
		return BadRequestResponse(c)
	}

	return c.JSON(FinishRegisterResponse{
		Token: token,
	})
}

func BeginAuthenticateApi(c *fiber.Ctx) error {
	req := new(BeginAuthenticateRequest)

	if err := c.BodyParser(req); err != nil {
		return BadRequestResponse(c)
	}

	if req.UserID == "" || req.DeviceID == "" {
		return BadRequestResponse(c)
	}

	credentialAssertion, success := BeginAuthenticate(req.UserID, req.DeviceID)
	if !success {
		return BadRequestResponse(c)
	}

	return c.JSON(BeginAuthenticateResponse{
		CredentialAssertion: credentialAssertion,
	})
}

func FinishAuthenticateApi(c *fiber.Ctx) error {
	req := new(FinishAuthenticateRequest)

	if err := c.BodyParser(req); err != nil {
		return BadRequestResponse(c)
	}

	if req.UserID == "" || req.DeviceID == "" {
		return BadRequestResponse(c)
	}

	par, err := req.CredentialData.Parse()
	if err != nil {
		return BadRequestResponse(c)
	}

	token, success := FinishAuthenticate(req.UserID, req.DeviceID, par)
	if !success {
		return BadRequestResponse(c)
	}

	return c.JSON(FinishAuthenticateResponse{
		Token: token,
	})
}
