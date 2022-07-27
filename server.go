package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"

	"net/http"
	_ "net/http/pprof"
)

const (
	PORT = 8001

	HEARTBEAT_INTERVAL       = 30 * time.Second
	HEARTBEAT_CLIENT_TIMEOUT = 10 * time.Second
	DEFUALT_CLIENT_TIMEOUT   = 5 * time.Second
	SDP_OFFER_CLIENT_TIMEOUT = 15 * time.Second
	TIMEOUT_INTERVAL         = 5 * time.Second
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

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	app := fiber.New()
	defer app.Shutdown()

	app.Use("/connect", InitializeSocket, websocket.New(WebsocketServer))

	log.Fatal(app.Listen(":" + strconv.Itoa(PORT)))

}
