package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

type errorResponse struct {
	Error       bool
	Code        int
	Description string
	ErrorInfo   []string
}

var errorCodesDescription = map[int]string{

	// Websocket errors
	40000: "Websocket upgrade error",
	40001: "Invalid pool id",
	40002: "",
	40003: "",
	40004: "",
	40005: "Websocket write error",
	40006: "Error marshalling to JSON",
	40007: "Error unmarshalling JSON",
	40008: "Websocket set read deadline error",
	40009: "Websocket unexpected binary message",
	40010: "Unrecognzied Op code",
}

func handleUpgradeError(c *fiber.Ctx, code int, errorInfo ...string) error {
	return c.Status(fiber.StatusBadRequest).JSON(errorResponse{
		Error:       true,
		Code:        code,
		Description: errorCodesDescription[code],
		ErrorInfo:   errorInfo,
	})
}

func handleWebsocketError(ws *websocket.Conn, code int, errorInfo ...string) {
	logger.Println("IP:", ws.RemoteAddr().String(), "| Code:", code, "| Error info:", errorInfo)
}
