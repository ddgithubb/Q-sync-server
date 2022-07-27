package main

import (
	"fmt"
	clustertree "sync-server/cluster-tree"
	"sync/atomic"
	"time"

	"github.com/aidarkhanov/nanoid/v2"
	"github.com/gofiber/websocket/v2"
)

func opRequiresKey(op int) bool {
	return op != 2006
}

func nodeChanRecv(ws *websocket.Conn, poolID string, nodeID string, nodeChan chan clustertree.NodeChanMessage, closeChan chan struct{}) {

	var (
		msg       clustertree.NodeChanMessage
		ok        bool = true
		clientErr error

		chanClosing  int32
		timeout      bool
		timeoutTimer chan struct{} = make(chan struct{})

		curReports      map[string]int = make(map[string]int)
		allReportsCount int

		curNodePosition clustertree.NodePosition

		nodeStates map[string]NodeStates = make(map[string]NodeStates) // keys are node ids

		ackPending map[string]*AckPendingInfo = make(map[string]*AckPendingInfo) // keys are Keys, value are how many acks accepted

	)

	atomic.StoreInt32(&chanClosing, 0)

	go func() {
		for {
			if atomic.LoadInt32(&chanClosing) == 1 {
				close(timeoutTimer)
				return
			}

			timeoutTimer <- struct{}{}
			time.Sleep(TIMEOUT_INTERVAL)
		}
	}()

	sendOp := func(op int, data interface{}, responseOp int, targetNodeID string, timeout time.Duration) {
		key, _ := nanoid.GenerateString(nanoid.DefaultAlphabet, 5)
		ackPending[key] = &AckPendingInfo{
			ExpireTime:   time.Now().Add(timeout),
			ResponseOp:   responseOp,
			TargetNodeID: targetNodeID,
		}
		clientErr = writeMessageWithTarget(ws, op, data, key, targetNodeID)
		if clientErr != nil {
			return
		}
	}

	connectNode := func(targetNodeID string) {
		sendOp(2001, nil, 2003, targetNodeID, SDP_OFFER_CLIENT_TIMEOUT)
	}

	disconnectNode := func(targetNodeID string) {
		nodeStates[targetNodeID] = INACTIVE_STATE
		delete(curReports, targetNodeID)
		sendOp(2002, nil, 2002, targetNodeID, DEFUALT_CLIENT_TIMEOUT)
	}

	reportNode := func(targetNodeID string, reportCode int, action int) {

		if nodeStates[targetNodeID] == INACTIVE_STATE {
			return
		}

		allReportsCount++

		_, ok := curReports[targetNodeID]

		if reportCode == DISCONNECT_REPORT {
			if ok {
				return
			}
		}

		if action == SERVER_ACTION {
			if len(curReports) >= 2 {
				l := len(curReports)
				for _, code := range curReports {
					if code == -1 {
						l--
					}
				}
				if l >= 2 {
					clientErr = clientNotCompliant(ws)
					return
				}
			}
			curReports[targetNodeID] = reportCode
			connectNode(targetNodeID)
		} else if action == CLIENT_ACTION {
			curReports[targetNodeID] = MY_REPORT
			if !clustertree.SendToNodeInPool(poolID, nodeID, targetNodeID, 2006, ReportNodeData{ReportCode: reportCode}) {
				delete(curReports, targetNodeID)
			}

		}
	}

	for {
		timeout = false
		select {
		case msg, ok = <-nodeChan:
		case <-timeoutTimer:
			timeout = true
			now := time.Now()
			for temp, ackInfo := range ackPending {
				if now.After(ackInfo.ExpireTime) {
					fmt.Println("KEY TIMEOUT", temp)
					clientErr = clientNotCompliant(ws)
				}
			}
		case <-closeChan:
			atomic.StoreInt32(&chanClosing, 1)
			for {
				if _, ok := <-timeoutTimer; !ok {
					break
				}
			}
			return
		}

		if !ok {
			continue
		}

		if timeout {
			continue
		}

		if clientErr != nil {
			continue
		}

		if msg.Action == CLIENT_ACTION {
			if msg.Key != "" {
				ackPendingInfo := ackPending[msg.Key]
				if ackPendingInfo != nil && ackPendingInfo.ResponseOp == msg.Op && ackPendingInfo.TargetNodeID == msg.TargetNodeID {
					delete(ackPending, msg.Key)
				} else {
					continue
				}
			} else {
				if opRequiresKey(msg.Op) {
					continue
				}
			}
		}

		switch msg.Op {
		case 2000:
			if msg.Action == SERVER_ACTION {
				newNodePositionData := msg.Data.(clustertree.NodePosition)
				updates := make(map[string]int)

				calcUpdates := func(newNodeID, curNodeID string) {
					if newNodeID != "" {
						if nodeStates[newNodeID] == INACTIVE_STATE {
							updates[newNodeID] = CONNECT_NODE
						} else if newNodeID != curNodeID {
							if updates[newNodeID] == DISCONNECT_NODE {
								updates[newNodeID] = NO_CHANGE_NODE
							}
							if _, ok := updates[curNodeID]; !ok {
								updates[curNodeID] = DISCONNECT_NODE
							}
						} else {
							updates[newNodeID] = NO_CHANGE_NODE
						}
					}
				}

				for i := 0; i < 3; i++ {
					for j := 0; j < 3; j++ {
						calcUpdates(newNodePositionData.ParentClusterNodes[i][j].NodeID, curNodePosition.ParentClusterNodes[i][j].NodeID)
					}
				}

				for i := 0; i < 2; i++ {
					calcUpdates(newNodePositionData.ChildClusterPartners[i].NodeID, curNodePosition.ChildClusterPartners[i].NodeID)
				}

				curNodePosition = newNodePositionData

				sendOp(2000, curNodePosition, 2000, nodeID, DEFUALT_CLIENT_TIMEOUT)

				for id, instruction := range updates {
					switch instruction {
					case CONNECT_NODE:
						if id != nodeID {
							connectNode(id)
							nodeStates[id] = ACTIVE_STATE
						}
					case DISCONNECT_NODE:
						disconnectNode(id)
						nodeStates[id] = INACTIVE_STATE
					}
				}
			}
		case 2001:
			nodeStatusData, ok := msg.Data.(NodeStatusData)
			if !ok {
				clientErr = clientNotCompliant(ws)
			}
			if nodeStates[msg.TargetNodeID] == ACTIVE_STATE {
				if msg.Action == CLIENT_ACTION {
					if nodeStatusData.Status == SUCCESSFUL_STATUS {
						delete(curReports, msg.TargetNodeID)
						clustertree.SendToNodeInPool(poolID, nodeID, msg.TargetNodeID, 2005, nil)
					} else if nodeStatusData.Status == UNSUCCESSFUL_STATUS {
						reportNode(msg.TargetNodeID, RECONNECT_REPORT, msg.Action)
					}
				}
			}
		case 2003:
			sdpData, ok := msg.Data.(SDPData)
			if !ok {
				clientErr = clientNotCompliant(ws)
			}
			if nodeStates[msg.TargetNodeID] == ACTIVE_STATE {
				if msg.Action == CLIENT_ACTION {
					if sdpData.Status == SUCCESSFUL_STATUS {
						clustertree.SendToNodeInPool(poolID, nodeID, msg.TargetNodeID, 2003, sdpData)
					} else if sdpData.Status == UNSUCCESSFUL_STATUS {
						reportNode(msg.TargetNodeID, RECONNECT_REPORT, SERVER_ACTION)
					}
				} else if msg.Action == SERVER_ACTION {
					sendOp(2003, sdpData, 2004, msg.TargetNodeID, SDP_OFFER_CLIENT_TIMEOUT)
				}
			}
		case 2004:
			sdpData, ok := msg.Data.(SDPData)
			if !ok {
				clientErr = clientNotCompliant(ws)
			}
			if nodeStates[msg.TargetNodeID] == ACTIVE_STATE {
				if msg.Action == CLIENT_ACTION {
					if sdpData.Status == SUCCESSFUL_STATUS {
						clustertree.SendToNodeInPool(poolID, nodeID, msg.TargetNodeID, 2004, sdpData)
					} else if sdpData.Status == UNSUCCESSFUL_STATUS {
						reportNode(msg.TargetNodeID, RECONNECT_REPORT, CLIENT_ACTION)
					}
				} else if msg.Action == SERVER_ACTION {
					sendOp(2004, sdpData, 2001, msg.TargetNodeID, SDP_OFFER_CLIENT_TIMEOUT)
				}
			}
		case 2005:
			if msg.Action == SERVER_ACTION {
				sendOp(2005, nil, 2005, msg.TargetNodeID, DEFUALT_CLIENT_TIMEOUT)
			} else if msg.Action == CLIENT_ACTION {
				nodeStatusData, ok := msg.Data.(NodeStatusData)
				if !ok {
					clientErr = clientNotCompliant(ws)
				}
				if nodeStatusData.Status == SUCCESSFUL_STATUS {
					delete(curReports, msg.TargetNodeID)
				} else if nodeStatusData.Status == UNSUCCESSFUL_STATUS {
					reportNode(msg.TargetNodeID, RECONNECT_REPORT, msg.Action)
				}
			}
		case 2006:
			reportNodeData, ok := msg.Data.(ReportNodeData)
			if !ok {
				clientErr = clientNotCompliant(ws)
			}
			for key, info := range ackPending {
				if msg.TargetNodeID == info.TargetNodeID {
					if (info.ResponseOp == 2001 || info.ResponseOp == 2003 || info.ResponseOp == 2004) && nodeStates[info.TargetNodeID] == INACTIVE_STATE {
						delete(ackPending, key)
						return
					}
				}
			}
			reportNode(msg.TargetNodeID, reportNodeData.ReportCode, msg.Action)
		case 2100:
			if msg.Action == SERVER_ACTION {
				curNodeID := ""
				updateData := msg.Data.(clustertree.UpdateSingleNodePositionData)

				if updateData.Node.NodeID != "" {
					found := false
					for i := 0; i < 3; i++ {
						for j := 0; j < 3; j++ {
							if curNodePosition.ParentClusterNodes[i][j].NodeID == updateData.Node.NodeID {
								curNodePosition.ParentClusterNodes[i][j].NodeID = ""
								found = true
								break
							}
						}
					}

					if !found {
						for i := 0; i < 2; i++ {
							if curNodePosition.ChildClusterPartners[i].NodeID == updateData.Node.NodeID {
								curNodePosition.ChildClusterPartners[i].NodeID = ""
								break
							}
						}
					}
				}

				if updateData.Position >= 9 {
					curNodeID = curNodePosition.ChildClusterPartners[updateData.Position-9].NodeID
					curNodePosition.ChildClusterPartners[updateData.Position-9] = updateData.Node
				} else {
					curNodeID = curNodePosition.ParentClusterNodes[int(updateData.Position/3)][(updateData.Position % 3)].NodeID
					curNodePosition.ParentClusterNodes[int(updateData.Position/3)][(updateData.Position % 3)] = updateData.Node
				}

				sendOp(2000, curNodePosition, 2000, nodeID, DEFUALT_CLIENT_TIMEOUT)
				if updateData.Node.NodeID == "" {
					delete(nodeStates, curNodeID)
				} else if nodeStates[updateData.Node.NodeID] == INACTIVE_STATE {
					nodeStates[updateData.Node.NodeID] = ACTIVE_STATE
				}
				if curNodeID != "" {
					disconnectNode(curNodeID)
				}
			}
		}
	}

}
