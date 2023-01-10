package pool

import "sync-server/sstypes"

/////////////////// REMEMEBR TO REMOVE SINGLE UDPATE NODE IN sstypes AFTER WORKS /////////////////////////

// type PoolNodeChanAction = int32
// const (
// 	SERVER_ACTION = 0
// 	CLIENT_ACTION = 1
// )

// type PoolNodeChanMessage struct {
// 	Action    PoolNodeChanAction
// 	SSMessage *sstypes.SSMessage
// }

type PoolNodeChanMessage struct {
	Type PoolNodeChanMessageType
	Data interface{}
}

type PoolNodeChanMessageType = int32

const (
	PNCMT_RECEIVED_SS_MESSAGE PoolNodeChanMessageType = 0
	PNCMT_SEND_SS_MESSAGE PoolNodeChanMessageType = 1
	PNCMT_UPDATE_NODE_POSITION PoolNodeChanMessageType = 2
	PNCMT_UPDATE_SINGLE_NODE_POSITION PoolNodeChanMessageType = 3
	PNCMT_NOTIFY_REPORT_NODE PoolNodeChanMessageType = 4
)

type PNCMReceivedSSMessageData = *sstypes.SSMessage

type PNCMSendSSMessageData = *sstypes.SSMessage

type PNCMUpdateNodePositionData = PoolNodePosition

type PNCMUpdateSingleNodePositionData struct {
	NodeID   string
	Position int32
}

type PNCMNotifyReportNodeData struct {
	FromNodeID string
	ReportCode PoolNodeChanReportCode
}