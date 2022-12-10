package main

import (
	"errors"
	"fmt"
	"strconv"
	clustertree "sync-server/cluster-tree"
	"time"

	"github.com/aidarkhanov/nanoid/v2"
	"github.com/gofiber/websocket/v2"
	jsoniter "github.com/json-iterator/go"
)

func safelyCloseWS(ws *websocket.Conn) {
	if ws != nil && ws.Conn != nil {
		ws.Close()
	}
}

func clientNotCompliant(ws *websocket.Conn) error {
	writeMessage(ws, 3000, nil)
	safelyCloseWS(ws)
	return errors.New("client not compliant")
}

func writeWSMessage(ws *websocket.Conn, msg interface{}) error {

	if ws == nil || ws.Conn == nil {
		return errors.New("conn closed")
	}

	b, err := jsoniter.Marshal(msg)
	if err != nil {
		handleWebsocketError(ws, 40006, err.Error())
		safelyCloseWS(ws)
		return err
	}

	// fmt.Println("SEND WS:", string(b))

	err = ws.WriteMessage(websocket.TextMessage, b)
	if err != nil {
		handleWebsocketError(ws, 40005, err.Error())
		safelyCloseWS(ws)
		return err
	}

	return nil
}

func writeMessage(ws *websocket.Conn, op int, data interface{}) error {
	return writeWSMessage(ws, constructLwtWSMessage(op, data))
}

func writeMessageWithTarget(ws *websocket.Conn, op int, data interface{}, key string, targetNodeID string) error {
	return writeWSMessage(ws, constructWSMessage(op, data, key, targetNodeID))
}

func WebsocketServer(ws *websocket.Conn) {

	poolID := ws.Locals("poolid").(string)

	// 1. Authenticate
	// TODO

	// 2. Validate that poolID is valid via the pool manager
	// TODO
	nodeID, _ := nanoid.GenerateString(nanoid.DefaultAlphabet, 10)

	closeChan := make(chan struct{})
	nodeChan := make(chan clustertree.NodeChanMessage)

	go nodeChanRecv(ws, poolID, nodeID, nodeChan, closeChan)

	clustertree.JoinPool(poolID, nodeID, nodeChan)

	fmt.Println(nodeID, "JOINED MAIN POOL")

	defer func() {
		clustertree.RemoveFromPool(poolID, nodeID)
		closeChan <- struct{}{}
	}()

	var (
		mt  int
		b   []byte
		err error

		op       int
		validOp  bool
		jsonData jsoniter.Any
		data     interface{}
	)
	for {

		if err = ws.SetReadDeadline(time.Now().Add(HEARTBEAT_INTERVAL + HEARTBEAT_CLIENT_TIMEOUT)); err != nil {
			handleWebsocketError(ws, 40008, err.Error())
			break
		}

		if mt, b, err = ws.ReadMessage(); err != nil {
			// i/o timeout is late heartbeat
			fmt.Println("WS read err", err, "| NodeID", nodeID)
			break
		}
		if mt == websocket.BinaryMessage {
			handleWebsocketError(ws, 40009)
			break
		}

		// fmt.Println("RECV WS:", string(b))

		validOp = true
		data = nil
		op = jsoniter.Get(b, "Op").ToInt()
		jsonData = jsoniter.Get(b, "Data")

		switch op {
		case 1000:
			writeMessage(ws, 1000, nil)
		case 2000:
		case 2001:
			d := new(NodeStatusData)
			jsonData.ToVal(d)
			data = *d
		case 2002:
		case 2003:
			d := new(SDPData)
			jsonData.ToVal(d)
			data = *d
		case 2004:
			d := new(SDPData)
			jsonData.ToVal(d)
			data = *d
		case 2005:
			d := new(NodeStatusData)
			jsonData.ToVal(d)
			data = *d
		case 2006:
			d := new(ReportNodeData)
			jsonData.ToVal(d)
			data = *d
		default:
			validOp = false
		}

		if !validOp {
			handleWebsocketError(ws, 40010, strconv.Itoa(jsoniter.Get(b, "Op").ToInt()))
			break
		}

		if op >= 2000 && op < 3000 {
			nodeChan <- clustertree.NodeChanMessage{
				Op:           op,
				Action:       clustertree.CLIENT_ACTION,
				Key:          jsoniter.Get(b, "Key").ToString(),
				TargetNodeID: jsoniter.Get(b, "TargetNodeID").ToString(),
				Data:         data,
			}
		}
	}
}
