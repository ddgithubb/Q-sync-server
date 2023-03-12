package main

import (
	"errors"
	"fmt"
	"sync-server/pool"
	"sync-server/sspb"
	"sync/atomic"
	"time"

	"github.com/aidarkhanov/nanoid/v2"
	"github.com/gofiber/websocket/v2"
)

type NodeState int32

const (
	inactiveNodeState NodeState = 0
	activeNodeState   NodeState = 1
)

type AckPendingInfo struct {
	expireTime time.Time
	responseOp sspb.SSMessage_Op

	targetNodeID string
}

func opNoKeyRequiredSSMsg(op sspb.SSMessage_Op) bool {
	return op == sspb.SSMessage_HEARTBEAT ||
		op == sspb.SSMessage_REPORT_NODE
	//return op < 2000 || op == 2006
}

func getThirdPanelNumber(panelA, panelB int) int {
	if panelA == 1 && panelB == 2 || panelA == 2 && panelB == 1 {
		return 0
	} else if panelA == 0 && panelB == 2 || panelA == 2 && panelB == 0 {
		return 1
	}
	return 2
}

type NodeManager struct {
	poolID string
	nodeID string

	ws        *websocket.Conn
	nodeChan  chan pool.PoolNodeChanMessage
	closeChan chan struct{}

	clientErr error

	curReports      map[string]pool.PoolNodeChanReportCode
	curReportsCount int

	curNodePosition pool.PoolNodePosition

	nodeStates map[string]NodeState // nodeID -> nodeState

	ackPending map[string]*AckPendingInfo // Keys -> AckPendingInfo
}

func StartNodeManager(ws *websocket.Conn, poolID string, nodeID string, nodeChan chan pool.PoolNodeChanMessage, closeChan chan struct{}) {
	nodeManager := &NodeManager{
		poolID:          poolID,
		nodeID:          nodeID,
		ws:              ws,
		nodeChan:        nodeChan,
		closeChan:       closeChan,
		clientErr:       nil,
		curReports:      make(map[string]pool.PoolNodeChanReportCode),
		curReportsCount: 0,
		curNodePosition: pool.PoolNodePosition{},
		nodeStates:      make(map[string]NodeState),
		ackPending:      make(map[string]*AckPendingInfo),
	}

	var (
		nodeChanMsg  pool.PoolNodeChanMessage
		ok           bool = true
		chanClosing  atomic.Bool
		timeoutTimer = make(chan struct{})
	)

	chanClosing.Store(false)

	go func() {
		for {
			if chanClosing.Load() {
				close(timeoutTimer)
				return
			}

			timeoutTimer <- struct{}{}
			time.Sleep(TIMEOUT_INTERVAL)
		}
	}()

	for {
		select {
		case nodeChanMsg, ok = <-nodeChan:
		case <-timeoutTimer:
			now := time.Now()
			for temp, ackInfo := range nodeManager.ackPending {
				if now.After(ackInfo.expireTime) {
					fmt.Println("KEY TIMEOUT", temp, "| NodeID", nodeID, "| ackInfo", ackInfo)
					nodeManager.clientNotCompliant()
				}
			}
			continue
		case <-closeChan:
			chanClosing.Store(true)
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

		if nodeManager.clientErr != nil {
			continue
		}

		switch nodeChanMsg.Type {
		case pool.PNCMT_RECEIVED_SS_MESSAGE:

			ssMsg, ok := nodeChanMsg.Data.(pool.PNCMReceivedSSMessageData)
			if !ok {
				nodeManager.clientNotCompliant()
				continue
			}

			var targetNodeID string

			if ssMsg.Key != "" {
				ackPendingInfo := nodeManager.ackPending[ssMsg.Key]
				if ackPendingInfo != nil && ackPendingInfo.responseOp == ssMsg.Op {
					delete(nodeManager.ackPending, ssMsg.Key)
					targetNodeID = ackPendingInfo.targetNodeID
				} else {
					nodeManager.clientNotCompliant()
					continue
				}
			} else {
				if !opNoKeyRequiredSSMsg(ssMsg.Op) {
					nodeManager.clientNotCompliant()
					continue
				}
			}

			switch ssMsg.Op {
			case sspb.SSMessage_HEARTBEAT:
				writeSSMessage(ws, sspb.BuildSSMessage(sspb.SSMessage_HEARTBEAT, nil))
			case sspb.SSMessage_CONNECT_NODE:
				successResponseData, ok := ssMsg.Data.(*sspb.SSMessage_SuccessResponseData_)
				if !ok {
					nodeManager.clientNotCompliant()
					continue
				}
				if nodeManager.nodeStates[targetNodeID] == activeNodeState {
					if successResponseData.SuccessResponseData.Success {
						nodeManager.clearReport(targetNodeID)
						nodeManager.sendToNodeInPool(
							targetNodeID,
							pool.PNCMT_SEND_SS_MESSAGE,
							sspb.BuildSSMessage(
								sspb.SSMessage_VERIFY_NODE_CONNECTED,
								&sspb.SSMessage_VerifyNodeConnectedData_{
									VerifyNodeConnectedData: &sspb.SSMessage_VerifyNodeConnectedData{
										NodeId: nodeID,
									},
								},
							),
						)
					} else {
						nodeManager.clientReportNode(targetNodeID, pool.REPORT_CODE_NOT_CONNECTING)
					}
				}
			case sspb.SSMessage_SEND_OFFER:
				sdpResponseData, ok := ssMsg.Data.(*sspb.SSMessage_SdpResponseData)
				if !ok {
					nodeManager.clientNotCompliant()
					continue
				}
				if nodeManager.nodeStates[targetNodeID] == activeNodeState {
					if sdpResponseData.SdpResponseData.Success {
						nodeManager.sendToNodeInPool(
							targetNodeID,
							pool.PNCMT_SEND_SS_MESSAGE,
							sspb.BuildSSMessage(
								sspb.SSMessage_SEND_OFFER,
								&sspb.SSMessage_SdpOfferData{
									SdpOfferData: &sspb.SSMessage_SDPOfferData{
										FromNodeId: nodeID,
										Sdp:        sdpResponseData.SdpResponseData.Sdp,
									},
								},
							),
						)
					} else {
						nodeManager.clientNotCompliant()
					}
				}
			case sspb.SSMessage_ANSWER_OFFER:
				sdpResponseData, ok := ssMsg.Data.(*sspb.SSMessage_SdpResponseData)
				if !ok {
					nodeManager.clientNotCompliant()
					continue
				}
				if nodeManager.nodeStates[targetNodeID] == activeNodeState {
					if sdpResponseData.SdpResponseData.Success {
						nodeManager.sendToNodeInPool(
							targetNodeID,
							pool.PNCMT_SEND_SS_MESSAGE,
							sspb.BuildSSMessage(
								sspb.SSMessage_ANSWER_OFFER,
								&sspb.SSMessage_SdpOfferData{
									SdpOfferData: &sspb.SSMessage_SDPOfferData{
										FromNodeId: nodeID,
										Sdp:        sdpResponseData.SdpResponseData.Sdp,
									},
								},
							),
						)
					} else {
						nodeManager.clientNotCompliant()
					}
				}
			case sspb.SSMessage_VERIFY_NODE_CONNECTED:
				successResponseData, ok := ssMsg.Data.(*sspb.SSMessage_SuccessResponseData_)
				if !ok {
					nodeManager.clientNotCompliant()
					continue
				}
				if nodeManager.nodeStates[targetNodeID] == activeNodeState {
					if successResponseData.SuccessResponseData.Success {
						nodeManager.clearReport(targetNodeID)
					} else {
						nodeManager.clientReportNode(targetNodeID, pool.REPORT_CODE_NOT_CONNECTING)
					}
				}
			case sspb.SSMessage_REPORT_NODE:
				reportNodeData, ok := ssMsg.Data.(*sspb.SSMessage_ReportNodeData_)
				if !ok {
					nodeManager.clientNotCompliant()
					continue
				}
				reportCode := pool.PoolNodeChanReportCode(reportNodeData.ReportNodeData.ReportCode)
				if !pool.ReportCodeIsFromClient(reportCode) {
					nodeManager.clientNotCompliant()
					continue
				}
				nodeManager.clientReportNode(reportNodeData.ReportNodeData.NodeId, reportCode)
			}

		case pool.PNCMT_SEND_SS_MESSAGE:

			ssMsg := nodeChanMsg.Data.(pool.PNCMSendSSMessageData)

			switch ssMsg.Op {
			case sspb.SSMessage_SEND_OFFER:
				fromNodeID := ssMsg.Data.(*sspb.SSMessage_SdpOfferData).SdpOfferData.FromNodeId
				if nodeManager.nodeStates[fromNodeID] == activeNodeState {
					nodeManager.sendServerSSMsgRequest(ssMsg, sspb.SSMessage_ANSWER_OFFER, SDP_OFFER_CLIENT_TIMEOUT, fromNodeID)
				}
			case sspb.SSMessage_ANSWER_OFFER:
				fromNodeID := ssMsg.Data.(*sspb.SSMessage_SdpOfferData).SdpOfferData.FromNodeId
				if nodeManager.nodeStates[fromNodeID] == activeNodeState {
					nodeManager.sendServerSSMsgRequest(ssMsg, sspb.SSMessage_CONNECT_NODE, SDP_OFFER_CLIENT_TIMEOUT, fromNodeID)
				}
			case sspb.SSMessage_VERIFY_NODE_CONNECTED:
				nodeID := ssMsg.Data.(*sspb.SSMessage_VerifyNodeConnectedData_).VerifyNodeConnectedData.NodeId
				if nodeManager.nodeStates[nodeID] == activeNodeState {
					nodeManager.sendServerSSMsgRequest(ssMsg, sspb.SSMessage_VERIFY_NODE_CONNECTED, DEFUALT_CLIENT_TIMEOUT, nodeID)
				}
			default:
				nodeManager.sendServerSSMsgRequest(ssMsg, ssMsg.Op, DEFUALT_CLIENT_TIMEOUT, nodeID)
			}

		case pool.PNCMT_UPDATE_NODE_POSITION:
			newNodePositionData := nodeChanMsg.Data.(pool.PNCMUpdateNodePositionData)
			nodeManager.curNodePosition = newNodePositionData

			nodeManager.updateNodePosition(true)
		case pool.PNCMT_UPDATE_SINGLE_NODE_POSITION:
			updateData := nodeChanMsg.Data.(pool.PNCMUpdateSingleNodePositionData)

			if updateData.NodeID != "" {
				found := false
				for i := 0; i < 3; i++ {
					for j := 0; j < 3; j++ {
						if nodeManager.curNodePosition.ParentClusterNodeIDs[i][j] == updateData.NodeID {
							nodeManager.curNodePosition.ParentClusterNodeIDs[i][j] = ""
							found = true
							break
						}
					}
				}

				if !found {
					for i := 0; i < 2; i++ {
						for j := 0; j < 3; j++ {
							if nodeManager.curNodePosition.ChildClusterNodeIDs[i][j] == updateData.NodeID {
								nodeManager.curNodePosition.ChildClusterNodeIDs[i][j] = ""
								break
							}
						}
					}
				}
			}

			if updateData.Position >= 9 {
				position := updateData.Position - 9
				nodeManager.curNodePosition.ChildClusterNodeIDs[int(position/3)][position%3] = updateData.NodeID
			} else {
				nodeManager.curNodePosition.ParentClusterNodeIDs[int(updateData.Position/3)][updateData.Position%3] = updateData.NodeID
			}

			nodeManager.updateNodePosition(false)

		case pool.PNCMT_NOTIFY_REPORT_NODE:

			notifyReportNodeData := nodeChanMsg.Data.(pool.PNCMNotifyReportNodeData)

			if nodeManager.reportNode(notifyReportNodeData.FromNodeID, notifyReportNodeData.ReportCode) {
				if pool.ReportCodeRequiresReconnect(notifyReportNodeData.ReportCode) {
					nodeManager.connectNode(notifyReportNodeData.FromNodeID)
				}
			}
		}
	}
}

func (nodeManager *NodeManager) clientNotCompliant() {
	writeSSMessage(nodeManager.ws, sspb.BuildSSMessage(sspb.SSMessage_CLOSE, nil))
	safelyCloseWS(nodeManager.ws)
	nodeManager.clientErr = errors.New("client not compliant")
}

func (nodeManager *NodeManager) createAckKey(responseOp sspb.SSMessage_Op, timeout time.Duration, targetNodeID string) string {
	key, _ := nanoid.GenerateString(nanoid.DefaultAlphabet, 5)
	nodeManager.ackPending[key] = &AckPendingInfo{
		expireTime:   time.Now().Add(timeout),
		responseOp:   responseOp,
		targetNodeID: targetNodeID,
	}
	return key
}

func (nodeManager *NodeManager) sendOpRequest(op, responseOp sspb.SSMessage_Op, data sspb.SSMessageData, timeout time.Duration, targetNodeID string) {
	nodeManager.clientErr = writeSSMessage(nodeManager.ws, sspb.BuildSSMessageWithKey(op, nodeManager.createAckKey(responseOp, timeout, targetNodeID), data))
	if nodeManager.clientErr != nil {
		return
	}
}

func (nodeManager *NodeManager) sendServerSSMsgRequest(ssMsg *sspb.SSMessage, responseOp sspb.SSMessage_Op, timeout time.Duration, targetNodeID string) {
	ssMsg.Key = nodeManager.createAckKey(responseOp, timeout, targetNodeID)
	nodeManager.clientErr = writeSSMessage(nodeManager.ws, ssMsg)
	if nodeManager.clientErr != nil {
		return
	}
}

func (nodeManager *NodeManager) sendToNodeInPool(targetNodeID string, msgType pool.PoolNodeChanMessageType, data interface{}) {
	go pool.SendToNodeInPool(nodeManager.poolID, nodeManager.nodeID, targetNodeID, msgType, data)
}

func (nodeManager *NodeManager) clearReport(targetNodeID string) {
	delete(nodeManager.curReports, targetNodeID)
	nodeManager.curReportsCount = 0
}

func (nodeManager *NodeManager) connectNode(targetNodeID string) {
	nodeManager.sendOpRequest(sspb.SSMessage_CONNECT_NODE, sspb.SSMessage_SEND_OFFER, &sspb.SSMessage_ConnectNodeData_{
		ConnectNodeData: &sspb.SSMessage_ConnectNodeData{
			NodeId: targetNodeID,
		},
	}, SDP_OFFER_CLIENT_TIMEOUT, targetNodeID)
}

func (nodeManager *NodeManager) disconnectNode(targetNodeID string) {
	delete(nodeManager.nodeStates, targetNodeID)
	nodeManager.clearReport(targetNodeID)
	nodeManager.sendOpRequest(sspb.SSMessage_DISCONNECT_NODE, sspb.SSMessage_DISCONNECT_NODE, &sspb.SSMessage_DisconnectNodeData_{
		DisconnectNodeData: &sspb.SSMessage_DisconnectNodeData{
			NodeId: targetNodeID,
		},
	}, DEFUALT_CLIENT_TIMEOUT, targetNodeID)
}

func (nodeManager *NodeManager) reportNode(targetNodeID string, reportCode pool.PoolNodeChanReportCode) (reported bool) {

	reported = false

	if nodeManager.nodeStates[targetNodeID] == inactiveNodeState {
		return
	}

	nodeManager.curReportsCount++

	if nodeManager.curReportsCount >= MAX_REPORTS {
		nodeManager.clientNotCompliant()
		if reportCode == pool.REPORT_CODE_FINAL {
			return
		}
		nodeManager.sendToNodeInPool(targetNodeID, pool.PNCMT_NOTIFY_REPORT_NODE, pool.PNCMNotifyReportNodeData{
			FromNodeID: nodeManager.nodeID,
			ReportCode: pool.REPORT_CODE_FINAL,
		})
		return
	}

	if reportCode == pool.REPORT_CODE_FINAL {
		return
	}

	_, ok := nodeManager.curReports[targetNodeID]

	if reportCode == pool.REPORT_CODE_DISCONNECT {
		if ok {
			return
		}
	}

	if len(nodeManager.curReports) >= MAX_UNIQUE_REPORTS {
		l := len(nodeManager.curReports)
		for _, code := range nodeManager.curReports {
			if code == -1 {
				l--
			}
		}
		if l >= MAX_UNIQUE_REPORTS {
			nodeManager.clientNotCompliant()
			return
		}
	}

	nodeManager.curReports[targetNodeID] = reportCode
	reported = true
	return

}

func (nodeManager *NodeManager) clientReportNode(targetNodeID string, reportCode pool.PoolNodeChanReportCode) {
	if nodeManager.reportNode(targetNodeID, reportCode) {
		nodeManager.sendToNodeInPool(targetNodeID, pool.PNCMT_NOTIFY_REPORT_NODE, pool.PNCMNotifyReportNodeData{
			FromNodeID: nodeManager.nodeID,
			ReportCode: reportCode,
		})
		//sendToNodeInPool(targetNodeID, 2006, ReportNodeData{ReportCode: reportCode})
	}
}

func (nodeManager *NodeManager) updateNodePosition(promoted bool) {
	connectNodes := make(map[string]bool)

	partnerInt := nodeManager.curNodePosition.PartnerInt
	panelNumber := nodeManager.curNodePosition.Path[len(nodeManager.curNodePosition.Path)-1]
	neighbouringNodesCount := 0
	for i := 0; i < 3; i++ {
		if i != panelNumber {
			nodeID := nodeManager.curNodePosition.ParentClusterNodeIDs[i][partnerInt]
			if nodeID == "" {
				greaterPanel := panelNumber > getThirdPanelNumber(i, panelNumber)
				for j := partnerInt + 1; j < partnerInt+3; j++ {
					id := ""
					if greaterPanel { // this panel is greater than opposing panel
						id = nodeManager.curNodePosition.ParentClusterNodeIDs[i][j%3]
					} else {
						id = nodeManager.curNodePosition.ParentClusterNodeIDs[i][(2*partnerInt+3-j)%3]
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
					nodeID := nodeManager.curNodePosition.ParentClusterNodeIDs[i][j]
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
		nodeID := nodeManager.curNodePosition.ChildClusterNodeIDs[i][partnerInt]
		if nodeID == "" {
			for j := partnerInt + 1; j < partnerInt+3; j++ {
				id := nodeManager.curNodePosition.ChildClusterNodeIDs[i][j%3]
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
						nodeID := nodeManager.curNodePosition.ParentClusterNodeIDs[i][j]
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
		if nodeManager.curNodePosition.ParentClusterNodeIDs[panelNumber][(partnerInt+1)%3] == "" {
			// from lower panel
			panel = 0
			if panelNumber == 0 {
				panel = 1
			}
			nodeID = nodeManager.curNodePosition.ParentClusterNodeIDs[panel][(partnerInt+1)%3]
		} else if nodeManager.curNodePosition.ParentClusterNodeIDs[panelNumber][(partnerInt+2)%3] == "" {
			// from higher panel
			panel = 2
			if panelNumber == 2 {
				panel = 1
			}
			nodeID = nodeManager.curNodePosition.ParentClusterNodeIDs[panel][(partnerInt+2)%3]
		}
		if nodeID != "" {
			if panelNumber > panel {
				connectNodes[nodeID] = true
			} else {
				connectNodes[nodeID] = false
			}
		}
	}

	path := make([]uint32, len(nodeManager.curNodePosition.Path))
	for i := 0; i < len(nodeManager.curNodePosition.Path); i++ {
		path[i] = uint32(nodeManager.curNodePosition.Path[i])
	}

	correctedParentClusterNodeIDs := make([]string, 0, 9)
	for i := 0; i < len(nodeManager.curNodePosition.ParentClusterNodeIDs); i++ {
		for j := 0; j < len(nodeManager.curNodePosition.ParentClusterNodeIDs[0]); j++ {
			correctedParentClusterNodeIDs = append(correctedParentClusterNodeIDs, nodeManager.curNodePosition.ParentClusterNodeIDs[i][j])
		}
	}

	correctedChildClusterNodeIDs := make([]string, 0, 6)
	for i := 0; i < len(nodeManager.curNodePosition.ChildClusterNodeIDs); i++ {
		for j := 0; j < len(nodeManager.curNodePosition.ChildClusterNodeIDs[0]); j++ {
			correctedChildClusterNodeIDs = append(correctedChildClusterNodeIDs, nodeManager.curNodePosition.ChildClusterNodeIDs[i][j])
		}
	}

	nodeManager.sendOpRequest(sspb.SSMessage_UPDATE_NODE_POSITION, sspb.SSMessage_UPDATE_NODE_POSITION, &sspb.SSMessage_UpdateNodePositionData_{
		UpdateNodePositionData: &sspb.SSMessage_UpdateNodePositionData{
			Path:                 path,
			PartnerInt:           uint32(nodeManager.curNodePosition.PartnerInt),
			CenterCluster:        nodeManager.curNodePosition.CenterCluster,
			ParentClusterNodeIds: correctedParentClusterNodeIDs,
			ChildClusterNodeIds:  correctedChildClusterNodeIDs,
		},
	}, DEFUALT_CLIENT_TIMEOUT, nodeManager.nodeID)

	for id, connect := range connectNodes {
		if nodeManager.nodeStates[id] == inactiveNodeState {
			if connect {
				nodeManager.connectNode(id)
			}
			nodeManager.nodeStates[id] = activeNodeState
		}
	}

	for id := range nodeManager.nodeStates {
		if _, ok := connectNodes[id]; !ok {
			nodeManager.disconnectNode(id)
		}
	}

}

// func nodeManager(ws *websocket.Conn, poolID string, nodeID string, nodeChan chan pool.PoolNodeChanMessage, closeChan chan struct{}) {

// 	var (
// 		nodeChanMsg pool.PoolNodeChanMessage
// 		ok          bool = true
// 		clientErr   error

// 		chanClosing  int32
// 		timeoutTimer chan struct{} = make(chan struct{})

// 		curReports      map[string]pool.PoolNodeChanReportCode = make(map[string]pool.PoolNodeChanReportCode)
// 		curReportsCount int

// 		curNodePosition pool.PoolNodePosition

// 		nodeStates map[string]NodeState = make(map[string]NodeState) // keys are node ids

// 		ackPending map[string]*AckPendingInfo = make(map[string]*AckPendingInfo) // keys are Keys, value are how many acks accepted

// 	)

// 	atomic.StoreInt32(&chanClosing, 0)

// 	go func() {
// 		for {
// 			if atomic.LoadInt32(&chanClosing) == 1 {
// 				close(timeoutTimer)
// 				return
// 			}

// 			timeoutTimer <- struct{}{}
// 			time.Sleep(TIMEOUT_INTERVAL)
// 		}
// 	}()

// 	clientNotCompliant := func() {
// 		writeSSMessage(ws, sspb.BuildSSMessage(sspb.SSMessage_CLOSE, nil))
// 		safelyCloseWS(ws)
// 		clientErr = errors.New("client not compliant")
// 	}

// 	createAckKey := func(responseOp sspb.SSMessage_Op, timeout time.Duration, targetNodeID string) string {
// 		key, _ := nanoid.GenerateString(nanoid.DefaultAlphabet, 5)
// 		ackPending[key] = &AckPendingInfo{
// 			expireTime:   time.Now().Add(timeout),
// 			responseOp:   responseOp,
// 			targetNodeID: targetNodeID,
// 		}
// 		return key
// 	}

// 	sendOpRequest := func(op, responseOp sspb.SSMessage_Op, data sspb.SSMessageData, timeout time.Duration, targetNodeID string) {
// 		clientErr = writeSSMessage(ws, sspb.BuildSSMessageWithKey(op, createAckKey(responseOp, timeout, targetNodeID), data))
// 		if clientErr != nil {
// 			return
// 		}
// 	}

// 	sendServerSSMsgRequest := func(ssMsg *sspb.SSMessage, responseOp sspb.SSMessage_Op, timeout time.Duration, targetNodeID string) {
// 		ssMsg.Key = createAckKey(responseOp, timeout, targetNodeID)
// 		clientErr = writeSSMessage(ws, ssMsg)
// 		if clientErr != nil {
// 			return
// 		}
// 	}

// 	sendToNodeInPool := func(targetNodeID string, msgType pool.PoolNodeChanMessageType, data interface{}) {
// 		go pool.SendToNodeInPool(poolID, nodeID, targetNodeID, msgType, data)
// 	}

// 	clearReport := func(targetNodeID string) {
// 		delete(curReports, targetNodeID)
// 		curReportsCount = 0
// 	}

// 	connectNode := func(targetNodeID string) {
// 		sendOpRequest(sspb.SSMessage_CONNECT_NODE, sspb.SSMessage_SEND_OFFER, &sspb.SSMessage_ConnectNodeData_{
// 			ConnectNodeData: &sspb.SSMessage_ConnectNodeData{
// 				NodeId: targetNodeID,
// 			},
// 		}, SDP_OFFER_CLIENT_TIMEOUT, targetNodeID)
// 	}

// 	disconnectNode := func(targetNodeID string) {
// 		delete(nodeStates, targetNodeID)
// 		clearReport(targetNodeID)
// 		sendOpRequest(sspb.SSMessage_DISCONNECT_NODE, sspb.SSMessage_DISCONNECT_NODE, &sspb.SSMessage_DisconnectNodeData_{
// 			DisconnectNodeData: &sspb.SSMessage_DisconnectNodeData{
// 				NodeId: targetNodeID,
// 			},
// 		}, DEFUALT_CLIENT_TIMEOUT, targetNodeID)
// 	}

// 	reportNode := func(targetNodeID string, reportCode pool.PoolNodeChanReportCode) (reported bool) {

// 		reported = false

// 		if nodeStates[targetNodeID] == inactiveNodeState {
// 			return
// 		}

// 		curReportsCount++

// 		if curReportsCount >= MAX_REPORTS {
// 			clientNotCompliant()
// 			if reportCode == pool.REPORT_CODE_FINAL {
// 				return
// 			}
// 			sendToNodeInPool(targetNodeID, pool.PNCMT_NOTIFY_REPORT_NODE, pool.PNCMNotifyReportNodeData{
// 				FromNodeID: nodeID,
// 				ReportCode: pool.REPORT_CODE_FINAL,
// 			})
// 			return
// 		}

// 		if reportCode == pool.REPORT_CODE_FINAL {
// 			return
// 		}

// 		_, ok := curReports[targetNodeID]

// 		if reportCode == pool.REPORT_CODE_DISCONNECT {
// 			if ok {
// 				return
// 			}
// 		}

// 		if len(curReports) >= MAX_UNIQUE_REPORTS {
// 			l := len(curReports)
// 			for _, code := range curReports {
// 				if code == -1 {
// 					l--
// 				}
// 			}
// 			if l >= MAX_UNIQUE_REPORTS {
// 				clientNotCompliant()
// 				return
// 			}
// 		}

// 		curReports[targetNodeID] = reportCode
// 		reported = true
// 		return

// 	}

// 	clientReportNode := func(targetNodeID string, reportCode pool.PoolNodeChanReportCode) {
// 		if reportNode(targetNodeID, reportCode) {
// 			sendToNodeInPool(targetNodeID, pool.PNCMT_NOTIFY_REPORT_NODE, pool.PNCMNotifyReportNodeData{
// 				FromNodeID: nodeID,
// 				ReportCode: reportCode,
// 			})
// 			//sendToNodeInPool(targetNodeID, 2006, ReportNodeData{ReportCode: reportCode})
// 		}
// 	}

// 	updateNodePosition := func(promoted bool) {
// 		connectNodes := make(map[string]bool)

// 		partnerInt := curNodePosition.PartnerInt
// 		panelNumber := curNodePosition.Path[len(curNodePosition.Path)-1]
// 		neighbouringNodesCount := 0
// 		for i := 0; i < 3; i++ {
// 			if i != panelNumber {
// 				nodeID := curNodePosition.ParentClusterNodeIDs[i][partnerInt]
// 				if nodeID == "" {
// 					greaterPanel := panelNumber > getThirdPanelNumber(i, panelNumber)
// 					for j := partnerInt + 1; j < partnerInt+3; j++ {
// 						id := ""
// 						if greaterPanel { // this panel is greater than opposing panel
// 							id = curNodePosition.ParentClusterNodeIDs[i][j%3]
// 						} else {
// 							id = curNodePosition.ParentClusterNodeIDs[i][(2*partnerInt+3-j)%3]
// 						}
// 						if id != "" {
// 							nodeID = id
// 							break
// 						}
// 					}
// 				}
// 				if nodeID != "" {
// 					if panelNumber > i {
// 						connectNodes[nodeID] = true
// 					} else {
// 						connectNodes[nodeID] = false
// 					}
// 				}
// 			} else {
// 				for j := 0; j < 3; j++ {
// 					if j != partnerInt {
// 						nodeID := curNodePosition.ParentClusterNodeIDs[i][j]
// 						if nodeID != "" {
// 							if promoted {
// 								connectNodes[nodeID] = true
// 							} else {
// 								connectNodes[nodeID] = false
// 							}
// 							neighbouringNodesCount++
// 						}
// 					}
// 				}
// 			}
// 		}

// 		for i := 0; i < 2; i++ {
// 			nodeID := curNodePosition.ChildClusterNodeIDs[i][partnerInt]
// 			if nodeID == "" {
// 				for j := partnerInt + 1; j < partnerInt+3; j++ {
// 					id := curNodePosition.ChildClusterNodeIDs[i][j%3]
// 					if id != "" {
// 						nodeID = id
// 						break
// 					}
// 				}
// 			}
// 			if nodeID != "" {
// 				connectNodes[nodeID] = true
// 			}
// 		}

// 		if neighbouringNodesCount == 0 {
// 			// everyone other parentcluster node (not your partner int) connects to this node
// 			for i := 0; i < 3; i++ {
// 				if i != panelNumber {
// 					for j := 0; j < 3; j++ {
// 						if j != partnerInt {
// 							nodeID := curNodePosition.ParentClusterNodeIDs[i][j]
// 							if nodeID != "" {
// 								if panelNumber > i {
// 									connectNodes[nodeID] = true
// 								} else {
// 									connectNodes[nodeID] = false
// 								}
// 							}
// 						}
// 					}
// 				}
// 			}
// 		} else if neighbouringNodesCount == 1 {
// 			nodeID := ""
// 			panel := 0
// 			if curNodePosition.ParentClusterNodeIDs[panelNumber][(partnerInt+1)%3] == "" {
// 				// from lower panel
// 				panel = 0
// 				if panelNumber == 0 {
// 					panel = 1
// 				}
// 				nodeID = curNodePosition.ParentClusterNodeIDs[panel][(partnerInt+1)%3]
// 			} else if curNodePosition.ParentClusterNodeIDs[panelNumber][(partnerInt+2)%3] == "" {
// 				// from higher panel
// 				panel = 2
// 				if panelNumber == 2 {
// 					panel = 1
// 				}
// 				nodeID = curNodePosition.ParentClusterNodeIDs[panel][(partnerInt+2)%3]
// 			}
// 			if nodeID != "" {
// 				if panelNumber > panel {
// 					connectNodes[nodeID] = true
// 				} else {
// 					connectNodes[nodeID] = false
// 				}
// 			}
// 		}

// 		path := make([]uint32, len(curNodePosition.Path))
// 		for i := 0; i < len(curNodePosition.Path); i++ {
// 			path[i] = uint32(curNodePosition.Path[i])
// 		}
// 		correctedParentClusterNodeIDs := make([]string, 0, 9)
// 		for i := 0; i < len(curNodePosition.ParentClusterNodeIDs); i++ {
// 			for j := 0; j < len(curNodePosition.ParentClusterNodeIDs[0]); j++ {
// 				correctedParentClusterNodeIDs = append(correctedParentClusterNodeIDs, curNodePosition.ParentClusterNodeIDs[i][j])
// 			}
// 		}
// 		correctedChildClusterNodeIDs := make([]string, 0, 6)
// 		for i := 0; i < len(curNodePosition.ChildClusterNodeIDs); i++ {
// 			for j := 0; j < len(curNodePosition.ChildClusterNodeIDs[0]); j++ {
// 				correctedChildClusterNodeIDs = append(correctedChildClusterNodeIDs, curNodePosition.ChildClusterNodeIDs[i][j])
// 			}
// 		}
// 		sendOpRequest(sspb.SSMessage_UPDATE_NODE_POSITION, sspb.SSMessage_UPDATE_NODE_POSITION, &sspb.SSMessage_UpdateNodePositionData_{
// 			UpdateNodePositionData: &sspb.SSMessage_UpdateNodePositionData{
// 				Path:                 path,
// 				PartnerInt:           uint32(curNodePosition.PartnerInt),
// 				CenterCluster:        curNodePosition.CenterCluster,
// 				ParentClusterNodeIds: correctedParentClusterNodeIDs,
// 				ChildClusterNodeIds:  correctedChildClusterNodeIDs,
// 			},
// 		}, DEFUALT_CLIENT_TIMEOUT, nodeID)
// 		//sendOp(2000, curNodePosition, 2000, nodeID, DEFUALT_CLIENT_TIMEOUT)

// 		for id, connect := range connectNodes {
// 			if nodeStates[id] == inactiveNodeState {
// 				if connect {
// 					connectNode(id)
// 				}
// 				nodeStates[id] = activeNodeState
// 			}
// 		}

// 		for id := range nodeStates {
// 			if _, ok := connectNodes[id]; !ok {
// 				disconnectNode(id)
// 				// if removeNodeID != "" && removeNodeID == id {
// 				// 	disconnectNode(id, true)
// 				// } else {
// 				// 	disconnectNode(id, false)
// 				// }
// 			}
// 		}

// 	}

// 	for {
// 		select {
// 		case nodeChanMsg, ok = <-nodeChan:
// 		case <-timeoutTimer:
// 			now := time.Now()
// 			for temp, ackInfo := range ackPending {
// 				if now.After(ackInfo.expireTime) {
// 					fmt.Println("KEY TIMEOUT", temp, "| NodeID", nodeID, "| ackInfo", ackInfo)
// 					clientNotCompliant()
// 				}
// 			}
// 			continue
// 		case <-closeChan:
// 			atomic.StoreInt32(&chanClosing, 1)
// 			for {
// 				if _, ok := <-timeoutTimer; !ok {
// 					break
// 				}
// 			}
// 			return
// 		}

// 		if !ok {
// 			continue
// 		}

// 		if clientErr != nil {
// 			continue
// 		}

// 		switch nodeChanMsg.Type {
// 		case pool.PNCMT_RECEIVED_SS_MESSAGE:

// 			ssMsg, ok := nodeChanMsg.Data.(pool.PNCMReceivedSSMessageData)
// 			if !ok {
// 				clientNotCompliant()
// 				continue
// 			}

// 			var targetNodeID string

// 			if ssMsg.Key != "" {
// 				ackPendingInfo := ackPending[ssMsg.Key]
// 				if ackPendingInfo != nil && ackPendingInfo.responseOp == ssMsg.Op {
// 					delete(ackPending, ssMsg.Key)
// 					targetNodeID = ackPendingInfo.targetNodeID
// 				} else {
// 					clientNotCompliant()
// 					continue
// 				}
// 			} else {
// 				if !opNoKeyRequiredSSMsg(ssMsg.Op) {
// 					clientNotCompliant()
// 					continue
// 				}
// 			}

// 			switch ssMsg.Op {
// 			case sspb.SSMessage_HEARTBEAT:
// 				writeSSMessage(ws, sspb.BuildSSMessage(sspb.SSMessage_HEARTBEAT, nil))
// 			case sspb.SSMessage_CONNECT_NODE:
// 				successResponseData, ok := ssMsg.Data.(*sspb.SSMessage_SuccessResponseData_)
// 				if !ok {
// 					clientNotCompliant()
// 					continue
// 				}
// 				if nodeStates[targetNodeID] == activeNodeState {
// 					if successResponseData.SuccessResponseData.Success {
// 						clearReport(targetNodeID)
// 						sendToNodeInPool(
// 							targetNodeID,
// 							pool.PNCMT_SEND_SS_MESSAGE,
// 							sspb.BuildSSMessage(
// 								sspb.SSMessage_VERIFY_NODE_CONNECTED,
// 								&sspb.SSMessage_VerifyNodeConnectedData_{
// 									VerifyNodeConnectedData: &sspb.SSMessage_VerifyNodeConnectedData{
// 										NodeId: nodeID,
// 									},
// 								},
// 							),
// 						)
// 					} else {
// 						clientReportNode(targetNodeID, pool.REPORT_CODE_NOT_CONNECTING)
// 					}
// 				}
// 			case sspb.SSMessage_SEND_OFFER:
// 				sdpResponseData, ok := ssMsg.Data.(*sspb.SSMessage_SdpResponseData)
// 				if !ok {
// 					clientNotCompliant()
// 					continue
// 				}
// 				if nodeStates[targetNodeID] == activeNodeState {
// 					if sdpResponseData.SdpResponseData.Success {
// 						sendToNodeInPool(
// 							targetNodeID,
// 							pool.PNCMT_SEND_SS_MESSAGE,
// 							sspb.BuildSSMessage(
// 								sspb.SSMessage_SEND_OFFER,
// 								&sspb.SSMessage_SdpOfferData{
// 									SdpOfferData: &sspb.SSMessage_SDPOfferData{
// 										FromNodeId: nodeID,
// 										Sdp:        sdpResponseData.SdpResponseData.Sdp,
// 									},
// 								},
// 							),
// 						)
// 					} else {
// 						clientNotCompliant()
// 					}
// 				}
// 			case sspb.SSMessage_ANSWER_OFFER:
// 				sdpResponseData, ok := ssMsg.Data.(*sspb.SSMessage_SdpResponseData)
// 				if !ok {
// 					clientNotCompliant()
// 					continue
// 				}
// 				if nodeStates[targetNodeID] == activeNodeState {
// 					if sdpResponseData.SdpResponseData.Success {
// 						sendToNodeInPool(
// 							targetNodeID,
// 							pool.PNCMT_SEND_SS_MESSAGE,
// 							sspb.BuildSSMessage(
// 								sspb.SSMessage_ANSWER_OFFER,
// 								&sspb.SSMessage_SdpOfferData{
// 									SdpOfferData: &sspb.SSMessage_SDPOfferData{
// 										FromNodeId: nodeID,
// 										Sdp:        sdpResponseData.SdpResponseData.Sdp,
// 									},
// 								},
// 							),
// 						)
// 					} else {
// 						clientNotCompliant()
// 					}
// 				}
// 			case sspb.SSMessage_VERIFY_NODE_CONNECTED:
// 				successResponseData, ok := ssMsg.Data.(*sspb.SSMessage_SuccessResponseData_)
// 				if !ok {
// 					clientNotCompliant()
// 					continue
// 				}
// 				if nodeStates[targetNodeID] == activeNodeState {
// 					if successResponseData.SuccessResponseData.Success {
// 						clearReport(targetNodeID)
// 					} else {
// 						clientReportNode(targetNodeID, pool.REPORT_CODE_NOT_CONNECTING)
// 					}
// 				}
// 			case sspb.SSMessage_REPORT_NODE:
// 				reportNodeData, ok := ssMsg.Data.(*sspb.SSMessage_ReportNodeData_)
// 				if !ok {
// 					clientNotCompliant()
// 					continue
// 				}
// 				reportCode := pool.PoolNodeChanReportCode(reportNodeData.ReportNodeData.ReportCode)
// 				if !pool.ReportCodeIsFromClient(reportCode) {
// 					clientNotCompliant()
// 					continue
// 				}
// 				clientReportNode(reportNodeData.ReportNodeData.NodeId, reportCode)
// 			}

// 		case pool.PNCMT_SEND_SS_MESSAGE:

// 			ssMsg := nodeChanMsg.Data.(pool.PNCMSendSSMessageData)

// 			switch ssMsg.Op {
// 			case sspb.SSMessage_SEND_OFFER:
// 				fromNodeID := ssMsg.Data.(*sspb.SSMessage_SdpOfferData).SdpOfferData.FromNodeId
// 				if nodeStates[fromNodeID] == activeNodeState {
// 					sendServerSSMsgRequest(ssMsg, sspb.SSMessage_ANSWER_OFFER, SDP_OFFER_CLIENT_TIMEOUT, fromNodeID)
// 				}
// 			case sspb.SSMessage_ANSWER_OFFER:
// 				fromNodeID := ssMsg.Data.(*sspb.SSMessage_SdpOfferData).SdpOfferData.FromNodeId
// 				if nodeStates[fromNodeID] == activeNodeState {
// 					sendServerSSMsgRequest(ssMsg, sspb.SSMessage_CONNECT_NODE, SDP_OFFER_CLIENT_TIMEOUT, fromNodeID)
// 				}
// 			case sspb.SSMessage_VERIFY_NODE_CONNECTED:
// 				nodeID := ssMsg.Data.(*sspb.SSMessage_VerifyNodeConnectedData_).VerifyNodeConnectedData.NodeId
// 				if nodeStates[nodeID] == activeNodeState {
// 					sendServerSSMsgRequest(ssMsg, sspb.SSMessage_VERIFY_NODE_CONNECTED, DEFUALT_CLIENT_TIMEOUT, nodeID)
// 				}
// 			default:
// 				sendServerSSMsgRequest(ssMsg, ssMsg.Op, DEFUALT_CLIENT_TIMEOUT, nodeID)
// 			}

// 		case pool.PNCMT_UPDATE_NODE_POSITION:
// 			newNodePositionData := nodeChanMsg.Data.(pool.PNCMUpdateNodePositionData)
// 			curNodePosition = newNodePositionData

// 			updateNodePosition(true)
// 		case pool.PNCMT_UPDATE_SINGLE_NODE_POSITION:
// 			updateData := nodeChanMsg.Data.(pool.PNCMUpdateSingleNodePositionData)

// 			if updateData.NodeID != "" {
// 				found := false
// 				for i := 0; i < 3; i++ {
// 					for j := 0; j < 3; j++ {
// 						if curNodePosition.ParentClusterNodeIDs[i][j] == updateData.NodeID {
// 							curNodePosition.ParentClusterNodeIDs[i][j] = ""
// 							found = true
// 							break
// 						}
// 					}
// 				}

// 				if !found {
// 					for i := 0; i < 2; i++ {
// 						for j := 0; j < 3; j++ {
// 							if curNodePosition.ChildClusterNodeIDs[i][j] == updateData.NodeID {
// 								curNodePosition.ChildClusterNodeIDs[i][j] = ""
// 								break
// 							}
// 						}
// 					}
// 				}
// 			}

// 			// curNodeID := ""

// 			if updateData.Position >= 9 {
// 				position := updateData.Position - 9
// 				// curNodeID = curNodePosition.ChildClusterNodeIDs[int(position/3)][position%3]
// 				curNodePosition.ChildClusterNodeIDs[int(position/3)][position%3] = updateData.NodeID
// 			} else {
// 				// curNodeID = curNodePosition.ParentClusterNodeIDs[int(updateData.Position/3)][updateData.Position%3]
// 				curNodePosition.ParentClusterNodeIDs[int(updateData.Position/3)][updateData.Position%3] = updateData.NodeID
// 			}

// 			// if updateData.NodeID == "" {
// 			// 	updateNodePosition(false, curNodeID)
// 			// } else {
// 			// 	updateNodePosition(false, "")
// 			// }
// 			updateNodePosition(false)

// 		case pool.PNCMT_NOTIFY_REPORT_NODE:

// 			notifyReportNodeData := nodeChanMsg.Data.(pool.PNCMNotifyReportNodeData)

// 			if reportNode(notifyReportNodeData.FromNodeID, notifyReportNodeData.ReportCode) {
// 				if pool.ReportCodeRequiresReconnect(notifyReportNodeData.ReportCode) {
// 					connectNode(notifyReportNodeData.FromNodeID)
// 				}
// 			}
// 		}
// 	}

// }
