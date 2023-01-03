package main

import (
	"errors"
	"fmt"
	"sync-server/pool"
	"sync-server/sstypes"
	"time"

	"github.com/aidarkhanov/nanoid/v2"
	"github.com/gofiber/websocket/v2"
	"google.golang.org/protobuf/proto"
)

func safelyCloseWS(ws *websocket.Conn) {
	// fmt.Println("CLOSING WS", ws != nil && ws.Conn != nil)
	if ws != nil && ws.Conn != nil {
		ws.Close()
	}
}

func writeSSMessage(ws *websocket.Conn, ssMsg *sstypes.SSMessage) error {

	if ws == nil || ws.Conn == nil {
		return errors.New("conn closed")
	}

	b, err := proto.Marshal(ssMsg)
	if err != nil {
		handleWebsocketError(ws, ERROR_MARSHALLING_PROTOBUF, err.Error())
		safelyCloseWS(ws)
		return err
	}

	// fmt.Println("SEND WS:", string(b))

	err = ws.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		handleWebsocketError(ws, ERROR_WEBSOCKET_WRITE, err.Error())
		safelyCloseWS(ws)
		return err
	}

	return nil
}

func WebsocketServer(ws *websocket.Conn) {

	poolID := ws.Locals("poolid").(string)

	// 1. Authenticate
	// TODO

	// 2. Validate that poolID is valid via the pool manager
	// TODO

	// 3. Keep track of updated users

	// TEMP
	userID, _ := nanoid.GenerateString(nanoid.DefaultAlphabet, 10)
	deviceID, _ := nanoid.GenerateString(nanoid.DefaultAlphabet, 10)
	displayName := ws.Query("displayname")
	userInfo := &sstypes.PoolUserInfo{
		UserId:      userID,
		DisplayName: displayName,
		Devices: []*sstypes.PoolDeviceInfo{
			{
				DeviceId:    deviceID,
				DeviceName:  displayName + "'s Device",
				DeviceType:  sstypes.DeviceType_BROWSER,
			},
		},
	}
	// TEMP

	nodeID, _ := nanoid.GenerateString(nanoid.DefaultAlphabet, 10)

	closeChan := make(chan struct{})
	nodeChan := make(chan pool.PoolNodeChanMessage) // Should be blocking for certain synchornization

	go nodeManager(ws, poolID, nodeID, nodeChan, closeChan)

	pool.JoinPool(poolID, nodeID, userID, userInfo.Devices[0], nodeChan, userInfo)

	fmt.Println(nodeID, "+ joining pool", poolID)

	defer func() {
		fmt.Println(nodeID, "- leaving pool", poolID)
		pool.RemoveFromPool(poolID, nodeID)
		closeChan <- struct{}{}
	}()

	var (
		b   []byte
		err error
	)
	for {

		if err = ws.SetReadDeadline(time.Now().Add(HEARTBEAT_INTERVAL + HEARTBEAT_CLIENT_TIMEOUT)); err != nil {
			handleWebsocketError(ws, ERROR_SET_READ_DEADLINE, err.Error())
			break
		}

		if _, b, err = ws.ReadMessage(); err != nil || len(b) == 0 {
			// i/o timeout is from read deadline
			// 1005 is client closed
			// len(b) == 0 to detect ws.Close() since it doesn't automatically do it
			// So either i/o timeout or len(b) == 0 will eventually close it
			
			// fmt.Println("WS read err", err, "| NodeID", nodeID)
			break
		}

		ssm := new(sstypes.SSMessage)
		if err := proto.Unmarshal(b, ssm); err != nil {
			handleWebsocketError(ws, ERROR_UNMARSHALLING_PROTOBUF, err.Error())
		}

		// fmt.Println("RECV WS:", nodeID, ssm)

		nodeChan <- pool.PoolNodeChanMessage{
			Type: pool.PNCMT_RECEIVED_SS_MESSAGE,
			Data: ssm,
		}
	}
}
