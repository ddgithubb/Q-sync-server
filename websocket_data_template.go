package main

const (
	NOT_ACTIVE_STATUS = 0
	ACTIVE_STATUS     = 1

	DISCONNECT_NODE = 0
	CONNECT_NODE    = 1
	NO_CHANGE_NODE  = 2
)

type AckPendingInfo struct {
	responses  int
	responseOp int
	nodeID     string
}

type WSMessage struct {
	Op   int
	Key  string
	Data interface{}
}

func constructWSMessage(op int, data interface{}, key string) WSMessage {
	return WSMessage{
		Op:   op,
		Key:  key,
		Data: data,
	}
}

type SendSDPData struct {
	NodeID string
	SDP    string
}

type NewNodePositionData struct {
	NodeID                     string
	Path                       []int
	PartnerInt                 int
	CenterCluster              bool
	ParentClusterNodeIDs       [3][3]string
	ChildClusterPartnerNodeIDs [2]string
	Updates                    map[string]int
}

type VerifyNodeConnectedData struct {
	NodeID string
}

type NodeStatusData struct {
	NodeID string
	Status int
}
