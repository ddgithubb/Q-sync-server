package clustertree

import (
	"math"
	"sync"
	"time"
)

type Node struct {
	NodeID             string
	NodeChan           chan NodeChanMessage
	ContainerNodePanel *NodePanel
	PartnerInt         int
	Created            time.Time
}

type NodePosition struct {
	Path                 []int
	PartnerInt           int
	CenterCluster        bool
	ParentClusterNodeIDs [3][3]string //connector panels is index 2
	ChildClusterNodeIDs  [2][3]string
}

type NodePanel struct {
	Path          []int
	Nodes         [3]*Node
	ParentCluster *NodeCluster
	ChildCluster  *NodeCluster
}

type NodeCluster struct {
	Center bool
	Panels [3]*NodePanel //for non-center, connector panels is index 2
}

type NodePool struct {
	ClusterTree [][]*NodeCluster
	NodeMap     map[string]*Node
	sync.RWMutex
}

func (np *NodePanel) getPanelNumber() int {
	return np.Path[len(np.Path)-1]
}

func (np *NodePanel) getPanelLevel() int {
	return len(np.Path) - 1
}

func (np *NodePanel) getOpposingPanel() *NodePanel {
	return np.ParentCluster.Panels[int(math.Abs(float64(np.getPanelNumber()-1)))]
}

func (np *NodePanel) getNodeAmount() int {
	amount := 0
	for i := 0; i < len(np.Nodes); i++ {
		if np.Nodes[i] != nil {
			amount++
		}
	}
	return amount
}

func (nc *NodeCluster) getNodeAmount() int {
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

func (n *Node) updateSingleNodePosition(position int, nodeID string) {
	n.NodeChan <- NodeChanMessage{
		Op:     2100,
		Action: SERVER_ACTION,
		Data: UpdateSingleNodePositionData{
			Position: position,
			NodeID:   nodeID,
		},
	}
}

func (n *Node) updateNodePosition() {

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
		n.NodeChan <- NodeChanMessage{
			Op:     2000,
			Action: SERVER_ACTION,
			Data: NodePosition{
				Path:                 n.ContainerNodePanel.Path,
				PartnerInt:           n.PartnerInt,
				CenterCluster:        n.ContainerNodePanel.ParentCluster.Center,
				ParentClusterNodeIDs: parent_cluster_node_ids,
				ChildClusterNodeIDs:  child_cluster_node_ids,
			},
		}
	}

}

func (np *NodePanel) getChildNodeAmount() int {
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

func (pool *NodePool) addNewClusterTreeLevel() {
	ct := pool.ClusterTree
	var new_cluster_level []*NodeCluster
	if len(ct) > 1 {
		new_cluster_level = make([]*NodeCluster, len(ct[len(ct)-1])*2)
		for i := 0; i < len(new_cluster_level)/2; i++ {
			for j := 0; j < 2; j++ {
				var cluster_panels [3]*NodePanel
				new_cluster := &NodeCluster{
					Center: false,
				}
				cluster_panels[2] = ct[len(ct)-1][i].Panels[j]
				cluster_panels[2].ChildCluster = new_cluster

				for k := 0; k < 2; k++ {
					var nodes [3]*Node
					cluster_panels[k] = &NodePanel{
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
		new_cluster_level = make([]*NodeCluster, 3)
		for i := 0; i < 3; i++ {
			var cluster_panels [3]*NodePanel
			new_cluster := &NodeCluster{
				Center: false,
			}
			cluster_panels[2] = ct[0][0].Panels[i]
			cluster_panels[2].ChildCluster = new_cluster

			for j := 0; j < 2; j++ {
				var nodes [3]*Node
				cluster_panels[j] = &NodePanel{
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

func (pool *NodePool) AddNode(node_id string, node_chan chan NodeChanMessage) {
	ct := pool.ClusterTree
	var new_node *Node

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
					new_node = &Node{
						NodeID:             node_id,
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
		pool.AddNode(node_id, node_chan)
	} else {
		new_node.updateNodePosition()
		pool.NodeMap[new_node.NodeID] = new_node
	}
}

func (pool *NodePool) RemoveNode(node_id string) {
	n, ok := pool.NodeMap[node_id]
	if !ok {
		return
	}

	promoted_nodes := make([]*Node, 0)
	cur_panel := n.ContainerNodePanel
	target_panel := cur_panel
	partner_int := n.PartnerInt

	n.ContainerNodePanel.Nodes[n.PartnerInt] = nil
	delete(pool.NodeMap, n.NodeID)

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

		partner_int = n.PartnerInt

		if panelA.Nodes[n.PartnerInt] == nil && panelB.Nodes[n.PartnerInt] == nil {
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
		} else if panelA.Nodes[n.PartnerInt] == nil {
			target_panel = panelB
		} else if panelB.Nodes[n.PartnerInt] == nil {
			target_panel = panelA
		} else if amountA == amountB {
			if panelA.Nodes[n.PartnerInt].Created.After(panelB.Nodes[n.PartnerInt].Created) {
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
		target_node.PartnerInt = n.PartnerInt
		cur_panel.Nodes[n.PartnerInt] = target_node
		target_panel.Nodes[partner_int] = nil
		cur_panel = target_panel

		promoted_nodes = append(promoted_nodes, target_node)
	}

	if len(promoted_nodes) > 0 {
		for i := 0; i < len(promoted_nodes); i++ {
			promoted_nodes[i].updateNodePosition()
		}
		n.ContainerNodePanel = cur_panel
		n.PartnerInt = partner_int
	}

	n.NodeID = ""
	n.updateNodePosition()
}

func NewNodePool() *NodePool {
	cluster_tree := make([][]*NodeCluster, 1)
	cluster_tree[0] = make([]*NodeCluster, 1)

	var center_cluster_panels [3]*NodePanel
	center_cluster := &NodeCluster{
		Center: true,
	}

	for i := 0; i < 3; i++ {
		var nodes [3]*Node
		center_cluster_panels[i] = &NodePanel{
			Path:          []int{i},
			Nodes:         nodes,
			ParentCluster: center_cluster,
			ChildCluster:  nil,
		}
	}

	center_cluster.Panels = center_cluster_panels
	cluster_tree[0][0] = center_cluster

	return &NodePool{
		NodeMap:     make(map[string]*Node),
		ClusterTree: cluster_tree,
	}
}
