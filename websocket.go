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
	return errors.New("Client not compliant")
}

func writeWSMessage(ws *websocket.Conn, msg WSMessage) error {

	b, err := jsoniter.Marshal(msg)
	if err != nil {
		handleWebsocketError(ws, 40006, err.Error())
		safelyCloseWS(ws)
		return err
	}

	fmt.Println("SEND WS:", string(b))

	err = ws.WriteMessage(websocket.TextMessage, b)
	if err != nil {
		handleWebsocketError(ws, 40005, err.Error())
		safelyCloseWS(ws)
		return err
	}

	return nil
}

func writeMessage(ws *websocket.Conn, op int, data interface{}) error {
	return writeWSMessage(ws, constructWSMessage(op, data, "", ""))
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

	node := clustertree.JoinPool(poolID, nodeID)

	defer func() {
		clustertree.RemoveFromPool(poolID, node)
	}()

	go nodeChanRecv(ws, poolID, nodeID, node.NodeChan)

	var (
		mt  int
		b   []byte
		err error

		op      int
		validOp bool
		data    interface{}
	)
	for {

		if err = ws.SetReadDeadline(time.Now().Add(HEARTBEAT_INTERVAL + HEARTBEAT_CLIENT_TIMEOUT)); err != nil {
			handleWebsocketError(ws, 40008, err.Error())
			break
		}

		if mt, b, err = ws.ReadMessage(); err != nil {
			break
		}
		if mt == websocket.BinaryMessage {
			handleWebsocketError(ws, 40009)
			break
		}

		fmt.Println("RECV WS:", string(b))

		validOp = true
		data = nil
		op = jsoniter.Get(b, "Op").ToInt()

		switch op {
		case 1000:
			writeMessage(ws, 1000, nil)
		case 2000:
		case 2001:
			d := new(NodeStatusData)
			jsoniter.Get(b, "Data").ToVal(d)
			data = *d
		case 2002:
		case 2003:
			d := new(SendSDPData)
			jsoniter.Get(b, "Data").ToVal(d)
			data = *d
		case 2004:
			d := new(SendSDPData)
			jsoniter.Get(b, "Data").ToVal(d)
			data = *d
		case 2005:
			d := new(NodeStatusData)
			jsoniter.Get(b, "Data").ToVal(d)
			data = *d
		default:
			validOp = false
			handleWebsocketError(ws, 40010, strconv.Itoa(jsoniter.Get(b, "Op").ToInt()))
		}

		if validOp {
			if op >= 2000 && op < 3000 {
				node.NodeChan <- clustertree.ChanMessage{
					Op:           op,
					Action:       clustertree.CLIENT_ACTION,
					Key:          jsoniter.Get(b, "Key").ToString(),
					TargetNodeID: jsoniter.Get(b, "TargetNodeID").ToString(),
					Data:         data,
				}
			}
		}
	}
}

func nodeChanRecv(ws *websocket.Conn, poolID string, nodeID string, nodeChan chan clustertree.ChanMessage) {

	var (
		chanMessage clustertree.ChanMessage
		ok          bool
		writeErr    error

		curNodePosition clustertree.NodePosition

		nodesStatus map[string]clustertree.NodeStatus = make(map[string]clustertree.NodeStatus) // keys are node ids

		ackPending map[string]*AckPendingInfo = make(map[string]*AckPendingInfo) // keys are Keys, value are how many acks accepted
	)

	sendOp := func(op int, data interface{}, responseOp int, targetNodeID string, timeout time.Duration) {
		key := strconv.FormatInt(time.Now().UnixMilli(), 10)
		ackPending[key] = &AckPendingInfo{
			ResponseOp:   responseOp,
			TargetNodeID: targetNodeID,
		}
		writeErr = writeMessageWithTarget(ws, op, data, key, targetNodeID)
		if writeErr != nil {
			return
		}
		go func() {
			time.Sleep(timeout)
			nodeChan <- clustertree.ChanMessage{
				Op:     op,
				Key:    key,
				Action: clustertree.TIMEOUT_ACTION,
			}
		}()
	}

	for {
		chanMessage, ok = <-nodeChan

		if !ok {
			return
		}

		if writeErr != nil {
			continue
		}

		if chanMessage.Action == clustertree.TIMEOUT_ACTION {
			if _, ok := ackPending[chanMessage.Key]; ok {
				writeErr = clientNotCompliant(ws)
			}
			continue
		} else if chanMessage.Action == clustertree.CLIENT_ACTION {
			if chanMessage.Key != "" {
				ackPendingInfo := ackPending[chanMessage.Key]
				if ackPendingInfo != nil && ackPendingInfo.ResponseOp == chanMessage.Op && ackPendingInfo.TargetNodeID == chanMessage.TargetNodeID {
					delete(ackPending, chanMessage.Key)
				} else {
					continue
				}
			}
		}

		switch chanMessage.Op {
		case 2000:
			if chanMessage.Action == clustertree.SERVER_ACTION {
				newNodePositionData := chanMessage.Data.(clustertree.NodePosition)
				updates := make(map[string]int)

				calcUpdates := func(newNodeID, curNodeID string) {
					if newNodeID != "" {
						if nodesStatus[newNodeID] == INACTIVE_STATUS {
							updates[newNodeID] = CONNECT_NODE
							nodesStatus[newNodeID] = ACTIVE_STATUS
						} else if newNodeID != curNodeID {
							if updates[newNodeID] == DISCONNECT_NODE {
								updates[newNodeID] = NO_CHANGE_NODE
								nodesStatus[newNodeID] = ACTIVE_STATUS
							}
							if _, ok := updates[curNodeID]; !ok {
								updates[curNodeID] = DISCONNECT_NODE
								nodesStatus[curNodeID] = INACTIVE_STATUS
							}
						} else {
							updates[newNodeID] = NO_CHANGE_NODE
						}
					}
				}

				for i := 0; i < 3; i++ {
					for j := 0; j < 3; j++ {
						calcUpdates(newNodePositionData.ParentClusterNodeIDs[i][j], curNodePosition.ParentClusterNodeIDs[i][j])
					}
				}

				for i := 0; i < 2; i++ {
					calcUpdates(newNodePositionData.ChildClusterPartnerNodeIDs[i], curNodePosition.ChildClusterPartnerNodeIDs[i])
				}

				curNodePosition = newNodePositionData

				sendOp(2000, curNodePosition, 2000, nodeID, DEFUALT_CLIENT_TIMEOUT)

				for id, instruction := range updates {
					switch instruction {
					case CONNECT_NODE:
						sendOp(2001, nil, 2003, id, SDP_OFFER_CLIENT_TIMEOUT)
					case DISCONNECT_NODE:
						sendOp(2002, nil, 2002, id, DEFUALT_CLIENT_TIMEOUT)
					}
				}
			}
		case 2001:
			if chanMessage.Action == clustertree.CLIENT_ACTION {
				nodeStatusData, ok := chanMessage.Data.(NodeStatusData)
				if !ok {
					writeErr = clientNotCompliant(ws)
				}
				if nodeStatusData.Status == ACTIVE_STATUS {
					clustertree.SendToNodeInPool(poolID, nodeID, chanMessage.TargetNodeID, 2005, nil)
				} else if nodeStatusData.Status == INACTIVE_STATUS {
					// report
				}
			}
		case 2003:
			sdpData, ok := chanMessage.Data.(SendSDPData)
			if !ok {
				writeErr = clientNotCompliant(ws)
			}
			if nodesStatus[chanMessage.TargetNodeID] == ACTIVE_STATUS {
				if chanMessage.Action == clustertree.CLIENT_ACTION {
					clustertree.SendToNodeInPool(poolID, nodeID, chanMessage.TargetNodeID, 2003, sdpData)
				} else if chanMessage.Action == clustertree.SERVER_ACTION {
					sendOp(2003, sdpData, 2004, chanMessage.TargetNodeID, SDP_OFFER_CLIENT_TIMEOUT)
				}
			}
		case 2004:
			sdpData, ok := chanMessage.Data.(SendSDPData)
			if !ok {
				writeErr = clientNotCompliant(ws)
			}
			if nodesStatus[chanMessage.TargetNodeID] == ACTIVE_STATUS {
				if chanMessage.Action == clustertree.CLIENT_ACTION {
					clustertree.SendToNodeInPool(poolID, nodeID, chanMessage.TargetNodeID, 2004, sdpData)
				} else if chanMessage.Action == clustertree.SERVER_ACTION {
					sendOp(2004, sdpData, 2001, chanMessage.TargetNodeID, SDP_OFFER_CLIENT_TIMEOUT)
				}
			}
		case 2005:
			if chanMessage.Action == clustertree.SERVER_ACTION {
				sendOp(2005, nil, 2005, chanMessage.TargetNodeID, DEFUALT_CLIENT_TIMEOUT)
			} else if chanMessage.Action == clustertree.CLIENT_ACTION {
				nodeStatusData, ok := chanMessage.Data.(NodeStatusData)
				if !ok {
					writeErr = clientNotCompliant(ws)
				}
				if nodeStatusData.Status == INACTIVE_STATUS {
					// report
				}
			}
		case 2006:
			if chanMessage.Action == clustertree.SERVER_ACTION {
				curNodeID := ""
				updateData := chanMessage.Data.(clustertree.UpdateSingleNodePositionData)

				found := false
				for i := 0; i < 3; i++ {
					for j := 0; j < 3; j++ {
						if curNodePosition.ParentClusterNodeIDs[i][j] == updateData.NodeID {
							curNodePosition.ParentClusterNodeIDs[i][j] = ""
							found = true
							break
						}
					}
				}

				if !found {
					for i := 0; i < 2; i++ {
						if curNodePosition.ChildClusterPartnerNodeIDs[i] == updateData.NodeID {
							curNodePosition.ChildClusterPartnerNodeIDs[i] = ""
							break
						}
					}
				}

				if updateData.Position >= 9 {
					curNodeID = curNodePosition.ChildClusterPartnerNodeIDs[updateData.Position-9]
					curNodePosition.ChildClusterPartnerNodeIDs[updateData.Position-9] = updateData.NodeID
				} else {
					curNodeID = curNodePosition.ParentClusterNodeIDs[int(updateData.Position/3)][(updateData.Position % 3)]
					curNodePosition.ParentClusterNodeIDs[int(updateData.Position/3)][(updateData.Position % 3)] = updateData.NodeID
				}

				sendOp(2000, curNodePosition, 2000, nodeID, DEFUALT_CLIENT_TIMEOUT)
				if updateData.NodeID == "" {
					delete(nodesStatus, curNodeID)
				} else if nodesStatus[updateData.NodeID] == INACTIVE_STATUS {
					nodesStatus[updateData.NodeID] = ACTIVE_STATUS
				}
				if curNodeID != "" {
					sendOp(2002, nil, 2002, curNodeID, DEFUALT_CLIENT_TIMEOUT)
				}
			}
		}
	}

}
