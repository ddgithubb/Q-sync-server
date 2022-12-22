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

type NodeInfo struct {
	UserID      string
	DisplayName string
	DeviceID    string
	DeviceName  string
	DeviceType  int
}

type BasicNode struct {
	NodeID string
	Path   []int
}

type UpdateSingleNodePositionData struct {
	Position int
	NodeID   string
}

type AddNodesData []*AddNodeData

type AddNodeData struct {
	NodeID    string
	Path      []int
	Timestamp int64
	NodeInfo  *NodeInfo
}

type RemoveNodeData struct {
	NodeID        string
	Timestamp     int64
	PromotedNodes []*BasicNode
}

func (node *Node) getAddNodeData() *AddNodeData {
	return &AddNodeData{
		NodeID:    node.NodeID,
		Path:      node.getPath(),
		Timestamp: node.Created.UnixMilli(),
		NodeInfo:  node.NodeInfo,
	}
}

func (node *Node) getBasicNode() *BasicNode {
	return &BasicNode{
		NodeID: node.NodeID,
		Path:   node.getPath(),
	}
}
