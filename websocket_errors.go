package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

type ERROR_CODE_TYPE = int32

const (
	ERROR_WEBSOCKET_UPGRADE      = 0
	ERROR_WEBSOCKET_WRITE        = 1
	ERROR_MARSHALLING_PROTOBUF   = 2
	ERROR_UNMARSHALLING_PROTOBUF = 3
	ERROR_SET_READ_DEADLINE      = 4
	ERROR_INVALID_POOL_ID        = 5
)

var errorCodesDescription = map[int]string{
	0: "Websocket upgrade error",
	1: "Websocket write error",
	2: "Error marshalling to protobuf",
	3: "Error unmarshalling protobuf",
	4: "Websocket set read deadline error",
	5: "Invalid pool id",
}

type ErrorResponse struct {
	Error       bool
	Description string
}

func handleUpgradeError(c *fiber.Ctx, errorCode int) error {
	return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
		Error:       true,
		Description: errorCodesDescription[errorCode],
	})
}

func handleWebsocketError(ws *websocket.Conn, errorCode int, errorInfo ...string) {
	ip := "not captured"
	if ws != nil && ws.Conn != nil {
		ip = ws.RemoteAddr().String()
	}
	if !DISABLE_LOGGING {
		logger.Println("IP:", ip, "| Code:", errorCode, "| Error info:", errorInfo)
	}
}
