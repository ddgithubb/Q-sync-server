package main

import (
	"fmt"
	clustertree "sync-server/cluster-tree"
	"sync/atomic"
	"time"

	"github.com/aidarkhanov/nanoid/v2"
	"github.com/gofiber/websocket/v2"
)

func opNoKeyRequired(op int) bool {
	return op == 2006
}

func getThirdPanelNumber(panelA, panelB int) int {
	if panelA == 1 && panelB == 2 || panelA == 2 && panelB == 1 {
		return 0
	} else if panelA == 0 && panelB == 2 || panelA == 2 && panelB == 0 {
		return 1
	}
	return 2
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

	sendToNodeInPool := func(targetNodeID string, op int, data interface{}) {
		go clustertree.SendToNodeInPool(poolID, nodeID, targetNodeID, op, data)
	}

	connectNode := func(targetNodeID string) {
		sendOp(2001, nil, 2003, targetNodeID, SDP_OFFER_CLIENT_TIMEOUT)
	}

	disconnectNode := func(targetNodeID string, removeFromPool bool) {
		delete(nodeStates, targetNodeID)
		delete(curReports, targetNodeID)
		sendOp(2002, nil, 2002, targetNodeID, DEFUALT_CLIENT_TIMEOUT)
		// sendOp(2002, DisconnectData{RemoveFromPool: removeFromPool}, 2002, targetNodeID, DEFUALT_CLIENT_TIMEOUT)
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
			sendToNodeInPool(targetNodeID, 2006, ReportNodeData{ReportCode: reportCode})
		}
	}

	updateNodePosition := func(promoted bool, removeNodeID string) {
		connectNodes := make(map[string]bool)

		partnerInt := curNodePosition.PartnerInt
		panelNumber := curNodePosition.Path[len(curNodePosition.Path)-1]
		neighbouringNodesCount := 0
		for i := 0; i < 3; i++ {
			if i != panelNumber {
				nodeID := curNodePosition.ParentClusterNodeIDs[i][partnerInt]
				if nodeID == "" {
					greaterPanel := panelNumber > getThirdPanelNumber(i, panelNumber)
					for j := partnerInt + 1; j < partnerInt+3; j++ {
						id := ""
						if greaterPanel { // this panel is greater than opposing panel
							id = curNodePosition.ParentClusterNodeIDs[i][j%3]
						} else {
							id = curNodePosition.ParentClusterNodeIDs[i][(2*partnerInt+3-j)%3]
						}
						if id != "" {
							nodeID = id
							break
						}
					}
				}
				if nodeID != "" {
					if panelNumber > i {
						connectNodes[nodeID] = true
					} else {
						connectNodes[nodeID] = false
					}
				}
			} else {
				for j := 0; j < 3; j++ {
					if j != partnerInt {
						nodeID := curNodePosition.ParentClusterNodeIDs[i][j]
						if nodeID != "" {
							if promoted {
								connectNodes[nodeID] = true
							} else {
								connectNodes[nodeID] = false
							}
							neighbouringNodesCount++
						}
					}
				}
			}
		}

		for i := 0; i < 2; i++ {
			nodeID := curNodePosition.ChildClusterNodeIDs[i][partnerInt]
			if nodeID == "" {
				for j := partnerInt + 1; j < partnerInt+3; j++ {
					id := curNodePosition.ChildClusterNodeIDs[i][j%3]
					if id != "" {
						nodeID = id
						break
					}
				}
			}
			if nodeID != "" {
				connectNodes[nodeID] = true
			}
		}

		if neighbouringNodesCount == 0 {
			// everyone other parentcluster node (not your partner int) connects to this node
			for i := 0; i < 3; i++ {
				if i != panelNumber {
					for j := 0; j < 3; j++ {
						if j != partnerInt {
							nodeID := curNodePosition.ParentClusterNodeIDs[i][j]
							if nodeID != "" {
								if panelNumber > i {
									connectNodes[nodeID] = true
								} else {
									connectNodes[nodeID] = false
								}
							}
						}
					}
				}
			}
		} else if neighbouringNodesCount == 1 {
			nodeID := ""
			panel := 0
			if curNodePosition.ParentClusterNodeIDs[panelNumber][(partnerInt+1)%3] == "" {
				// from lower panel
				panel = 0
				if panelNumber == 0 {
					panel = 1
				}
				nodeID = curNodePosition.ParentClusterNodeIDs[panel][(partnerInt+1)%3]
			} else if curNodePosition.ParentClusterNodeIDs[panelNumber][(partnerInt+2)%3] == "" {
				// from higher panel
				panel = 2
				if panelNumber == 2 {
					panel = 1
				}
				nodeID = curNodePosition.ParentClusterNodeIDs[panel][(partnerInt+2)%3]
			}
			if nodeID != "" {
				if panelNumber > panel {
					connectNodes[nodeID] = true
				} else {
					connectNodes[nodeID] = false
				}
			}
		}

		sendOp(2000, curNodePosition, 2000, nodeID, DEFUALT_CLIENT_TIMEOUT)

		for id, connect := range connectNodes {
			if nodeStates[id] == INACTIVE_STATE {
				if connect {
					connectNode(id)
				}
				nodeStates[id] = ACTIVE_STATE
			}
		}

		for id := range nodeStates {
			if _, ok := connectNodes[id]; !ok {
				if removeNodeID != "" && removeNodeID == id {
					disconnectNode(id, true)
				} else {
					disconnectNode(id, false)
				}
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
					fmt.Println("KEY TIMEOUT", temp, "| NodeID", nodeID, "| ackInfo", ackInfo)
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
				if !opNoKeyRequired(msg.Op) {
					continue
				}
			}
		}

		switch msg.Op {
		case 2000:
			if msg.Action == SERVER_ACTION {
				newNodePositionData := msg.Data.(clustertree.NodePosition)
				curNodePosition = newNodePositionData

				updateNodePosition(true, "")
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
						sendToNodeInPool(msg.TargetNodeID, 2005, nil)
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
						sendToNodeInPool(msg.TargetNodeID, 2003, sdpData)
					} else if sdpData.Status == UNSUCCESSFUL_STATUS {
						reportNode(msg.TargetNodeID, RECONNECT_REPORT, SERVER_ACTION)
					}
				} else if msg.Action == SERVER_ACTION {
					sendOp(2003, sdpData, 2004, msg.TargetNodeID, SDP_OFFER_CLIENT_TIMEOUT)
				}
			} else {
				fmt.Println("!!! 2003 NOT ACTIVE", msg.TargetNodeID)
			}
		case 2004:
			sdpData, ok := msg.Data.(SDPData)
			if !ok {
				clientErr = clientNotCompliant(ws)
			}
			if nodeStates[msg.TargetNodeID] == ACTIVE_STATE {
				if msg.Action == CLIENT_ACTION {
					if sdpData.Status == SUCCESSFUL_STATUS {
						sendToNodeInPool(msg.TargetNodeID, 2004, sdpData)
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
			reportNode(msg.TargetNodeID, reportNodeData.ReportCode, msg.Action)
		case 2010:
			if msg.Action == SERVER_ACTION {
				sendOp(2010, msg.Data, 2010, nodeID, DEFUALT_CLIENT_TIMEOUT)
			}
		case 2011:
			if msg.Action == SERVER_ACTION {
				sendOp(2011, msg.Data, 2011, nodeID, DEFUALT_CLIENT_TIMEOUT)
			}
		case 2100:
			if msg.Action == SERVER_ACTION {
				updateData := msg.Data.(clustertree.UpdateSingleNodePositionData)

				if updateData.NodeID != "" {
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
							for j := 0; j < 3; j++ {
								if curNodePosition.ChildClusterNodeIDs[i][j] == updateData.NodeID {
									curNodePosition.ChildClusterNodeIDs[i][j] = ""
									break
								}
							}
						}
					}
				}

				curNodeID := ""

				if updateData.Position >= 9 {
					position := updateData.Position - 9
					curNodeID = curNodePosition.ChildClusterNodeIDs[int(position/3)][position%3]
					curNodePosition.ChildClusterNodeIDs[int(position/3)][position%3] = updateData.NodeID
				} else {
					curNodeID = curNodePosition.ParentClusterNodeIDs[int(updateData.Position/3)][updateData.Position%3]
					curNodePosition.ParentClusterNodeIDs[int(updateData.Position/3)][updateData.Position%3] = updateData.NodeID
				}

				if updateData.NodeID == "" {
					updateNodePosition(false, curNodeID)
				} else {
					updateNodePosition(false, "")
				}

			}
		}
	}

}
