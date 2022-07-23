package main

import "time"

const (
	SERVER_ACTION = 0
	CLIENT_ACTION = 1

	INACTIVE_STATE = 0
	ACTIVE_STATE   = 1

	UNSUCCESSFUL_STATUS = 0
	SUCCESSFUL_STATUS   = 1

	DISCONNECT_NODE = 0
	CONNECT_NODE    = 1
	NO_CHANGE_NODE  = 2

	DISCONNECT_REPORT      = 0
	RECONNECT_REPORT       = 1
	UNRESPONSIVE_REPORT    = 2
	MUNGED_MESSAGES_REPORT = 3
)

type AckPendingInfo struct {
	ExpireTime   time.Time
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

type NodeStates int

type SDPData struct {
	SDP    string
	Status int
}

type NodeStatusData struct {
	Status int
}

type ReportNodeData struct {
	ReportCode int
}
