package pool

import (
	"math"
	"sync"
	"sync-server/sstypes"
	"time"
)

type PoolDeviceInfo = sstypes.PoolDeviceInfo

type PoolNode struct {
	NodeID             string
	UserID             string
	DeviceInfo         *PoolDeviceInfo
	UserInfo           *sstypes.PoolUserInfo // TEMP
	NodeChan           chan PoolNodeChanMessage
	ContainerNodePanel *PoolNodePanel
	PartnerInt         int
	Created            time.Time
}

type PoolNodePosition struct {
	Path                 []int
	PartnerInt           int
	CenterCluster        bool
	ParentClusterNodeIDs [3][3]string // connector panels is index 2
	ChildClusterNodeIDs  [2][3]string
}

type PoolNodePanel struct {
	Path          []int
	Nodes         [3]*PoolNode
	ParentCluster *PoolNodeCluster
	ChildCluster  *PoolNodeCluster
}

type PoolNodeCluster struct {
	Center bool
	Panels [3]*PoolNodePanel // for non-center, connector panels is index 2
}

type Pool struct {
	ClusterTree [][]*PoolNodeCluster
	NodeMap     map[string]*PoolNode
	sync.RWMutex
}

func (np *PoolNodePanel) getPanelNumber() int {
	return np.Path[len(np.Path)-1]
}

func (np *PoolNodePanel) getPanelLevel() int {
	return len(np.Path) - 1
}

func (np *PoolNodePanel) getOpposingPanel() *PoolNodePanel {
	return np.ParentCluster.Panels[int(math.Abs(float64(np.getPanelNumber()-1)))]
}

func (np *PoolNodePanel) getNodeAmount() int {
	amount := 0
	for i := 0; i < len(np.Nodes); i++ {
		if np.Nodes[i] != nil {
			amount++
		}
	}
	return amount
}

func (nc *PoolNodeCluster) getNodeAmount() int {
	amount := 0
	panels_num := 2
	if nc.Center {
		panels_num = 3
	}
	for i := 0; i < panels_num; i++ {
		for j := 0; j < 3; j++ {
			if nc.Panels[i].Nodes[j] != nil {
				amount++
			}
		}
	}
	return amount
}

func (n *PoolNode) getPath() []int {
	return n.ContainerNodePanel.Path
}

func (n *PoolNode) getPathInt32() []uint32 {
	path := n.getPath()
	newPath := make([]uint32, len(path))
	for i := 0; i < len(path); i++ {
		newPath[i] = uint32(path[i])
	}
	return newPath
}

func (n *PoolNode) updateSingleNodePosition(position int, nodeID string) {
	n.NodeChan <- PoolNodeChanMessage{
		Type: PNCMT_UPDATE_SINGLE_NODE_POSITION,
		Data: PNCMUpdateSingleNodePositionData{
			Position: int32(position),
			NodeID:   nodeID,
		},
	}
}

func (n *PoolNode) updateNodePosition() {

	var parent_cluster_node_ids [3][3]string
	var child_cluster_node_ids [2][3]string

	for i := 0; i < 3; i++ {
		// if !n.ContainerNodePanel.ParentCluster.Center && i != 2 && i != n.ContainerNodePanel.getPanelNumber() {
		// 	continue
		// }
		position := (n.ContainerNodePanel.getPanelNumber() * 3) + n.PartnerInt
		if i == 2 && !n.ContainerNodePanel.ParentCluster.Center {
			position = position + 9
		}
		for j := 0; j < 3; j++ {
			node := n.ContainerNodePanel.ParentCluster.Panels[i].Nodes[j]
			if node != nil {
				parent_cluster_node_ids[i][j] = node.NodeID
				if node != n {
					node.updateSingleNodePosition(position, n.NodeID)
				}
			}
		}
	}

	if n.ContainerNodePanel.ChildCluster != nil {
		position := 6 + n.PartnerInt
		for i := 0; i < 2; i++ {
			for j := 0; j < 3; j++ {
				node := n.ContainerNodePanel.ChildCluster.Panels[i].Nodes[j]
				if node != nil {
					child_cluster_node_ids[i][j] = node.NodeID
					node.updateSingleNodePosition(position, n.NodeID)
				}
			}
		}
	}

	if n.NodeID != "" {
		n.NodeChan <- PoolNodeChanMessage{
			Type: PNCMT_UPDATE_NODE_POSITION,
			Data: PNCMUpdateNodePositionData{
				Path:                 n.ContainerNodePanel.Path,
				PartnerInt:           n.PartnerInt,
				CenterCluster:        n.ContainerNodePanel.ParentCluster.Center,
				ParentClusterNodeIDs: parent_cluster_node_ids,
				ChildClusterNodeIDs:  child_cluster_node_ids,
			},
		}
	}

}

func (np *PoolNodePanel) getChildNodeAmount() int {
	amount := 0

	for i := 0; i < 3; i++ {
		if np.Nodes[i] != nil {
			amount++
		}
	}

	if np.ChildCluster != nil && len(np.ChildCluster.Panels) != 0 {
		for i := 0; i < 2; i++ {
			amount += np.ChildCluster.Panels[i].getChildNodeAmount()
		}
	}

	return amount
}

func (pool *Pool) addNewClusterTreeLevel() {
	ct := pool.ClusterTree
	var new_cluster_level []*PoolNodeCluster
	if len(ct) > 1 {
		new_cluster_level = make([]*PoolNodeCluster, len(ct[len(ct)-1])*2)
		for i := 0; i < len(new_cluster_level)/2; i++ {
			for j := 0; j < 2; j++ {
				var cluster_panels [3]*PoolNodePanel
				new_cluster := &PoolNodeCluster{
					Center: false,
				}
				cluster_panels[2] = ct[len(ct)-1][i].Panels[j]
				cluster_panels[2].ChildCluster = new_cluster

				for k := 0; k < 2; k++ {
					var nodes [3]*PoolNode
					cluster_panels[k] = &PoolNodePanel{
						Path:          append(cluster_panels[2].Path, k),
						Nodes:         nodes,
						ParentCluster: new_cluster,
						ChildCluster:  nil,
					}
				}

				new_cluster_level[i*2+j] = new_cluster
				new_cluster.Panels = cluster_panels
			}
		}
	} else if len(ct) == 1 {
		new_cluster_level = make([]*PoolNodeCluster, 3)
		for i := 0; i < 3; i++ {
			var cluster_panels [3]*PoolNodePanel
			new_cluster := &PoolNodeCluster{
				Center: false,
			}
			cluster_panels[2] = ct[0][0].Panels[i]
			cluster_panels[2].ChildCluster = new_cluster

			for j := 0; j < 2; j++ {
				var nodes [3]*PoolNode
				cluster_panels[j] = &PoolNodePanel{
					Path:          append(cluster_panels[2].Path, j),
					Nodes:         nodes,
					ParentCluster: new_cluster,
					ChildCluster:  nil,
				}
			}

			new_cluster.Panels = cluster_panels
			new_cluster_level[i] = new_cluster
		}
	}
	pool.ClusterTree = append(ct, new_cluster_level)
}

func (pool *Pool) AddNode(node_id, user_id string, device_info *PoolDeviceInfo, node_chan chan PoolNodeChanMessage) *PoolNode {
	ct := pool.ClusterTree
	var new_node *PoolNode

	added := false
	for i := 0; i < len(ct); i++ {
		lowest_node_amount := ct[i][0].getNodeAmount()
		lowest_node_amount_index := 0
		for j := 1; j < len(ct[i]); j++ {
			node_amount := ct[i][j].getNodeAmount()
			if node_amount <= lowest_node_amount {
				lowest_node_amount = node_amount
				lowest_node_amount_index = j
			}
		}
		panels_num := 2
		if ct[i][0].Center {
			if lowest_node_amount != 9 {
				added = true
				panels_num = 3
			}
		} else {
			if lowest_node_amount != 6 {
				added = true
			}
		}
		if added {
			lowest_node_panel_amount := ct[i][lowest_node_amount_index].Panels[0].getNodeAmount()
			lowest_node_panel_amount_index := 0
			for j := 1; j < panels_num; j++ {
				node_amount := ct[i][lowest_node_amount_index].Panels[j].getNodeAmount()
				if node_amount <= lowest_node_panel_amount {
					lowest_node_panel_amount = node_amount
					lowest_node_panel_amount_index = j
				}
			}
			for j := 0; j < 3; j++ {
				if ct[i][lowest_node_amount_index].Panels[lowest_node_panel_amount_index].Nodes[j] == nil {
					new_node = &PoolNode{
						NodeID:             node_id,
						UserID:             user_id,
						DeviceInfo:         device_info,
						NodeChan:           node_chan,
						ContainerNodePanel: ct[i][lowest_node_amount_index].Panels[lowest_node_panel_amount_index],
						PartnerInt:         j,
						Created:            time.Now(),
					}
					ct[i][lowest_node_amount_index].Panels[lowest_node_panel_amount_index].Nodes[j] = new_node
					break
				}
			}
			break
		}
	}

	if !added {
		pool.addNewClusterTreeLevel()
		return pool.AddNode(node_id, user_id, device_info, node_chan)
	}

	new_node.updateNodePosition()
	pool.NodeMap[new_node.NodeID] = new_node
	return new_node
}

func (pool *Pool) RemoveNode(node_id string) ([]*PoolNode, *PoolNode) { // TEMP second *PoolNode parameter
	node, ok := pool.NodeMap[node_id]
	if !ok {
		return nil, nil
	}

	promoted_nodes := make([]*PoolNode, 0)
	cur_panel := node.ContainerNodePanel
	target_panel := cur_panel
	partner_int := node.PartnerInt

	node.ContainerNodePanel.Nodes[node.PartnerInt] = nil
	delete(pool.NodeMap, node.NodeID)

	for {
		if cur_panel.ChildCluster == nil {
			break
		}

		panelA := cur_panel.ChildCluster.Panels[0]
		panelB := cur_panel.ChildCluster.Panels[1]
		amountA := panelA.getChildNodeAmount()
		amountB := panelB.getChildNodeAmount()

		if amountA == 0 && amountB == 0 {
			break
		}

		partner_int = node.PartnerInt

		if panelA.Nodes[node.PartnerInt] == nil && panelB.Nodes[node.PartnerInt] == nil {
			var created time.Time
			for i := 0; i < 3; i++ {
				if panelA.Nodes[i] != nil && panelA.Nodes[i].Created.After(created) {
					created = panelA.Nodes[i].Created
					target_panel = panelA
					partner_int = i
				}
			}
			for i := 0; i < 3; i++ {
				if panelB.Nodes[i] != nil && panelB.Nodes[i].Created.After(created) {
					created = panelB.Nodes[i].Created
					target_panel = panelB
					partner_int = i
				}
			}
		} else if panelA.Nodes[node.PartnerInt] == nil {
			target_panel = panelB
		} else if panelB.Nodes[node.PartnerInt] == nil {
			target_panel = panelA
		} else if amountA == amountB {
			if panelA.Nodes[node.PartnerInt].Created.After(panelB.Nodes[node.PartnerInt].Created) {
				target_panel = panelA
			} else {
				target_panel = panelB
			}
		} else if amountA > amountB {
			target_panel = panelA
		} else if amountB > amountA {
			target_panel = panelB
		}

		target_node := target_panel.Nodes[partner_int]
		target_node.ContainerNodePanel = cur_panel
		target_node.PartnerInt = node.PartnerInt
		cur_panel.Nodes[node.PartnerInt] = target_node
		target_panel.Nodes[partner_int] = nil
		cur_panel = target_panel

		promoted_nodes = append(promoted_nodes, target_node)
	}

	if len(promoted_nodes) > 0 {
		for i := 0; i < len(promoted_nodes); i++ {
			promoted_nodes[i].updateNodePosition()
		}
		node.ContainerNodePanel = cur_panel
		node.PartnerInt = partner_int
	}

	node.NodeID = ""
	node.updateNodePosition()
	return promoted_nodes, node
}

func NewNodePool() *Pool {
	cluster_tree := make([][]*PoolNodeCluster, 1)
	cluster_tree[0] = make([]*PoolNodeCluster, 1)

	var center_cluster_panels [3]*PoolNodePanel
	center_cluster := &PoolNodeCluster{
		Center: true,
	}

	for i := 0; i < 3; i++ {
		var nodes [3]*PoolNode
		center_cluster_panels[i] = &PoolNodePanel{
			Path:          []int{i},
			Nodes:         nodes,
			ParentCluster: center_cluster,
			ChildCluster:  nil,
		}
	}

	center_cluster.Panels = center_cluster_panels
	cluster_tree[0][0] = center_cluster

	return &Pool{
		NodeMap:     make(map[string]*PoolNode),
		ClusterTree: cluster_tree,
	}
}
