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

	LAST_REPORT = -2
	MY_REPORT              = -1
	DISCONNECT_REPORT      = 0
	RECONNECT_REPORT       = 1
	UNRELIABLE_REPORT      = 2
	MUNGED_MESSAGES_REPORT = 3
	SPAM_REPORT            = 4
)

type VersionInfo struct {
	Version string
}

type AckPendingInfo struct {
	ExpireTime   time.Time
	ResponseOp   int
	TargetNodeID string
}

type LwtWSMessage struct {
	Op   int
	Data interface{}
}

type WSMessage struct {
	Op           int
	Key          string
	TargetNodeID string
	Data         interface{}
}

func constructLwtWSMessage(op int, data interface{}) LwtWSMessage {
	return LwtWSMessage{
		Op:   op,
		Data: data,
	}
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

type DisconnectData struct {
	RemoveFromPool bool
}

type NodeStatusData struct {
	Status int
}

type ReportNodeData struct {
	ReportCode int
}
