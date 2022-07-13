package main

import (
	"log"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

const (
	port = 8001
)

var logger *log.Logger

func init() {

	logFile, err := os.OpenFile("logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		panic("Error opening/creating log.txt")
	}

	logger = log.New(logFile, "", log.LstdFlags)

}

func main() {

	app := fiber.New()
	defer app.Shutdown()

	app.Use("/connect", InitializeSocket, websocket.New(WebsocketServer))

	log.Fatal(app.Listen(strconv.Itoa(port)))

}
