package main

import (
	"fmt"

	"github.com/gofiber/websocket/v2"
	jsoniter "github.com/json-iterator/go"
)

func writeMessage(ws *websocket.Conn, code int, data interface{}) error {

	b, err := jsoniter.Marshal(constructWSMessage(code, data))
	if err != nil {
		handleWebsocketError(ws, 40006, err.Error())
	}

	fmt.Println("SEND WS:", string(b))

	err = ws.WriteMessage(websocket.TextMessage, b)
	if err != nil {
		handleWebsocketError(ws, 40005, err.Error())
	}

	return err
}

func WebsocketServer(ws *websocket.Conn) {

	poolID := ws.Locals("poolid")

}
