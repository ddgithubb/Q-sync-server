package main

const (
	INACTIVE_STATUS = 0
	ACTIVE_STATUS   = 1

	DISCONNECT_NODE = 0
	CONNECT_NODE    = 1
	NO_CHANGE_NODE  = 2
)

type AckPendingInfo struct {
	ResponseOp   int
	TargetNodeID string
}

type WSMessage struct {
	Op           int
	Key          string
	TargetNodeID string
	Data         interface{}
}

func constructWSMessage(op int, data interface{}, key string, targetNodeID string) WSMessage {
	return WSMessage{
		Op:           op,
		Key:          key,
		TargetNodeID: targetNodeID,
		Data:         data,
	}
}

type SendSDPData struct {
	SDP string
}

type NodeStatusData struct {
	Status int
}
