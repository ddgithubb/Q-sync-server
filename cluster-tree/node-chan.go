package clustertree

const (
	SERVER_ACTION = 0
	CLIENT_ACTION = 1
)

type NodeChanMessage struct {
	Op           int
	Action       int
	Key          string
	TargetNodeID string
	Data         interface{}
}

type UpdateSingleNodePositionData struct {
	Position int
	NodeID   string
}
