package pool

import "sync-server/sspb"

type PoolNodeChanMessage struct {
	Type PoolNodeChanMessageType
	Data interface{}
}

type PoolNodeChanMessageType = int32

const (
	PNCMT_RECEIVED_SS_MESSAGE         PoolNodeChanMessageType = 0
	PNCMT_SEND_SS_MESSAGE             PoolNodeChanMessageType = 1
	PNCMT_UPDATE_NODE_POSITION        PoolNodeChanMessageType = 2
	PNCMT_UPDATE_SINGLE_NODE_POSITION PoolNodeChanMessageType = 3
	PNCMT_NOTIFY_REPORT_NODE          PoolNodeChanMessageType = 4
)

type PNCMReceivedSSMessageData = *sspb.SSMessage

type PNCMSendSSMessageData = *sspb.SSMessage

type PNCMUpdateNodePositionData = PoolNodePosition

type PNCMUpdateSingleNodePositionData struct {
	NodeID   string
	Position int32
}

type PNCMNotifyReportNodeData struct {
	FromNodeID string
	ReportCode PoolNodeChanReportCode
}
