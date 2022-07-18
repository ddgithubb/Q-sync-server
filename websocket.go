package main

import (
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

func writeMessageNoKey(ws *websocket.Conn, op int, data interface{}) error {
	return writeWSMessage(ws, constructWSMessage(op, data, ""))
}

func writeMessageWithKey(ws *websocket.Conn, op int, data interface{}, key string) error {
	return writeWSMessage(ws, constructWSMessage(op, data, key))
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

		switch jsoniter.Get(b, "Op").ToInt() {
		case 1000:
			writeMessageNoKey(ws, 1000, nil)
		default:
			handleWebsocketError(ws, 40010, strconv.Itoa(jsoniter.Get(b, "Op").ToInt()))
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

	sendOp := func(op int, data interface{}, responseOp int, timeout time.Duration, expectedResponses int) {
		if expectedResponses > 0 {
			key := strconv.FormatInt(time.Now().UnixMilli(), 10)
			ackPending[key] = &AckPendingInfo{
				responses:  expectedResponses,
				responseOp: responseOp,
			}
			writeErr = writeMessageWithKey(ws, op, data, key)
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
		} else {
			writeErr = writeMessageNoKey(ws, op, data)
		}
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
			if ackPending[chanMessage.Key].responses != 0 {
				writeErr = writeMessageNoKey(ws, 3000, nil)
				safelyCloseWS(ws)
			} else {
				delete(ackPending, chanMessage.Key)
			}
			continue
		} else if chanMessage.Action == clustertree.CLIENT_ACTION {
			if chanMessage.Key != "" {
				ackPendingInfo := ackPending[chanMessage.Key]
				if ackPendingInfo != nil && ackPendingInfo.responseOp == chanMessage.Op {
					ackPendingInfo.responseOp--
				}
			}
		}

		switch chanMessage.Op {
		case 2000:
			sdpData := chanMessage.Data.(SendSDPData)
			if nodesStatus[sdpData.NodeID] == ACTIVE_STATUS {
				if chanMessage.Action == clustertree.CLIENT_ACTION {
					targetNodeID := sdpData.NodeID
					sdpData.NodeID = nodeID
					clustertree.SendToNodeInPool(poolID, targetNodeID, 2000, sdpData)
				} else if chanMessage.Action == clustertree.SERVER_ACTION {
					sendOp(2000, sdpData, 2001, SDP_OFFER_CLIENT_TIMEOUT, 1)
				}
			}
		case 2001:
			sdpData := chanMessage.Data.(SendSDPData)
			if nodesStatus[sdpData.NodeID] == ACTIVE_STATUS {
				if chanMessage.Action == clustertree.CLIENT_ACTION {
					targetNodeID := sdpData.NodeID
					sdpData.NodeID = nodeID
					clustertree.SendToNodeInPool(poolID, targetNodeID, 2001, sdpData)
				} else if chanMessage.Action == clustertree.SERVER_ACTION {
					sendOp(2001, sdpData, 2002, SDP_OFFER_CLIENT_TIMEOUT, 1)
				}
			}
		case 2002:
			if chanMessage.Action == clustertree.SERVER_ACTION {
				newNodePositionData := chanMessage.Data.(clustertree.NodePosition)

				newNodePosition := &NewNodePositionData{
					NodeID:                     nodeID,
					Path:                       newNodePositionData.Path,
					PartnerInt:                 newNodePositionData.PartnerInt,
					CenterCluster:              newNodePositionData.CenterCluster,
					ParentClusterNodeIDs:       newNodePositionData.ParentClusterNodeIDs,
					ChildClusterPartnerNodeIDs: newNodePositionData.ChildClusterPartnerNodeIDs,
					Updates:                    make(map[string]int),
				}

				expectedResponses := 0

				calcUpdates := func(newNodeID, curNodeID string) {
					if newNodeID != "" {
						if nodesStatus[newNodeID] == NOT_ACTIVE_STATUS {
							newNodePosition.Updates[newNodeID] = CONNECT_NODE
							nodesStatus[newNodeID] = ACTIVE_STATUS
							expectedResponses++
						} else if newNodeID != curNodeID {
							if newNodePosition.Updates[newNodeID] == DISCONNECT_NODE {
								newNodePosition.Updates[newNodeID] = NO_CHANGE_NODE
								nodesStatus[newNodeID] = ACTIVE_STATUS
							}
							if _, ok := newNodePosition.Updates[curNodeID]; !ok {
								newNodePosition.Updates[curNodeID] = DISCONNECT_NODE
								nodesStatus[curNodeID] = NOT_ACTIVE_STATUS
							}
						} else {
							newNodePosition.Updates[newNodeID] = NO_CHANGE_NODE
						}
					}
				}

				for i := 0; i < 3; i++ {
					for j := 0; j < 3; j++ {
						calcUpdates(newNodePosition.ParentClusterNodeIDs[i][j], curNodePosition.ParentClusterNodeIDs[i][j])
					}
				}

				for i := 0; i < 2; i++ {
					calcUpdates(newNodePosition.ChildClusterPartnerNodeIDs[i], curNodePosition.ChildClusterPartnerNodeIDs[i])
				}

				curNodePosition = newNodePositionData

				sendOp(2002, newNodePosition, 2000, SDP_OFFER_CLIENT_TIMEOUT, expectedResponses)
			} else if chanMessage.Action == clustertree.CLIENT_ACTION {
				nodeStatusData := chanMessage.Data.(NodeStatusData)

				if nodeStatusData.Status == ACTIVE_STATUS {
					clustertree.SendToNodeInPool(poolID, nodeStatusData.NodeID, 2003, VerifyNodeConnectedData{
						NodeID: nodeID,
					})
				} else if nodeStatusData.Status == NOT_ACTIVE_STATUS {
					// report
				}
			}
		case 2003:
		case 2004:
			if chanMessage.Action == clustertree.SERVER_ACTION {
				curNodeID := ""
				updateData := chanMessage.Data.(clustertree.UpdateNodePositionData)
				if updateData.Position >= 9 {
					curNodeID = curNodePosition.ChildClusterPartnerNodeIDs[updateData.Position-9]
					curNodePosition.ChildClusterPartnerNodeIDs[updateData.Position-9] = updateData.NodeID
				} else {
					curNodeID = curNodePosition.ParentClusterNodeIDs[int(updateData.Position/3)][(updateData.Position % 3)]
					curNodePosition.ParentClusterNodeIDs[int(updateData.Position/3)][(updateData.Position % 3)] = updateData.NodeID
				}
				if updateData.NodeID == "" {
					delete(nodesStatus, curNodeID)
				} else if nodesStatus[updateData.NodeID] == NOT_ACTIVE_STATUS {
					nodesStatus[curNodeID] = ACTIVE_STATUS
				}
				sendOp(2005, updateData, 2005, DEFUALT_CLIENT_TIMEOUT, 1)
			}
		}
	}

}
