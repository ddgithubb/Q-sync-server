package main

import (
	"log"
	"os"
	"sync-server/auth"
	"sync-server/store"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/websocket/v2"
	// "net/http"
	// _ "net/http/pprof"
)

const (
	VERSION = "v1"
	PORT    = ":80"

	HEARTBEAT_INTERVAL       = 30 * time.Second
	HEARTBEAT_CLIENT_TIMEOUT = 10 * time.Second
	DEFUALT_CLIENT_TIMEOUT   = 5 * time.Second
	SDP_OFFER_CLIENT_TIMEOUT = 15 * time.Second
	TIMEOUT_INTERVAL         = 5 * time.Second

	MAX_UNIQUE_REPORTS = 3
	MAX_REPORTS        = 5

	DISABLE_LOGGING = true
)

type VersionInfo struct {
	Version string
}

var logger *log.Logger

func init() {

	if DISABLE_LOGGING {
		return
	}

	logFile, err := os.OpenFile("server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		panic("Error opening/creating server.log")
	}

	logger = log.New(logFile, "", log.LstdFlags)

}

func main() {

	// go func() {
	// 	log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()

	defer cleanUp()

	app := fiber.New()
	defer app.Shutdown()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
	}))

	auth.AttatchAuthRoutes(app)

	app.Get("/ss/version", func(c *fiber.Ctx) error {
		return c.JSON(VersionInfo{
			Version: VERSION,
		})
	})

	ssGroup := app.Group("/ss/" + VERSION)
	ssGroup.Use("/connect", InitializeSocket, websocket.New(WebsocketServer))

	log.Fatal(app.Listen(PORT))
}

func cleanUp() {
	store.CloseDB()
}
