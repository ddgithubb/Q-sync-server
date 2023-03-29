package main

import (
	"errors"
	"fmt"
	"sync-server/auth"
	"sync-server/pool"
	"sync-server/sspb"
	"time"

	"github.com/gofiber/websocket/v2"
	"google.golang.org/protobuf/proto"
)

func safelyCloseWS(ws *websocket.Conn) {
	// fmt.Println("CLOSING WS", ws != nil && ws.Conn != nil)
	if ws != nil && ws.Conn != nil {
		ws.Close()
	}
}

func writeSSMessage(ws *websocket.Conn, ssMsg *sspb.SSMessage) error {

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

func AuthenticateWebsocket(ws *websocket.Conn, deviceID string) (*auth.AuthTokenData, bool) {
	if err := ws.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, false
	}

	_, b, err := ws.ReadMessage()
	if err != nil || len(b) == 0 {
		return nil, false
	}

	authToken := string(b)

	if len(authToken) != auth.AUTH_TOKEN_SIZE {
		return nil, false
	}

	tokenData, ok := auth.VerifyAndRefreshAuthToken(deviceID, authToken)
	if !ok {
		return nil, false
	}

	ws.WriteMessage(websocket.TextMessage, []byte(tokenData.Token))
	return tokenData, true
}

func WebsocketServer(ws *websocket.Conn) {

	poolID := ws.Locals("poolid").(string)
	deviceID := ws.Query("deviceid")
	isTestStr := ws.Query("test", "false")

	isTest := false
	if isTestStr == "true" {
		isTest = true
	}

	_ = isTest

	// Authenticate
	authTokenData, ok := AuthenticateWebsocket(ws, deviceID)
	if !ok {
		ws.WriteMessage(websocket.CloseMessage, []byte{})
		ws.Close()
		return
	}

	userID := authTokenData.UserID
	nodeID := deviceID

	closeChan := make(chan struct{})
	nodeChan := make(chan pool.PoolNodeChanMessage) // Should be blocking for certain synchornization

	defer func() {
		fmt.Println(nodeID, "- leaving pool", poolID)
		pool.DisconnectFromPool(poolID, nodeID)
		closeChan <- struct{}{}
	}()

	go StartNodeManager(ws, poolID, nodeID, nodeChan, closeChan)

	ok = pool.ConnectToPool(poolID, nodeID, userID, authTokenData.Device, nodeChan)
	if !ok {
		return
	}

	fmt.Println(nodeID, "+ joining pool", poolID)

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

		ssm := new(sspb.SSMessage)
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
