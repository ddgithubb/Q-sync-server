package pool

import (
	"fmt"
	"sync-server/sspb"

	cmap "github.com/orcaman/concurrent-map/v2"
)

var ActivePools cmap.ConcurrentMap[string, *Pool] = cmap.New[*Pool]()

func (node *PoolNode) getAddNodeData() *sspb.SSMessage_AddNodeData {
	return &sspb.SSMessage_AddNodeData{
		NodeId: node.NodeID,
		UserId: node.UserID,
		Path:   node.getPathInt32(),
	}
}

func (node *PoolNode) getBasicNode() *sspb.PoolBasicNode {
	return &sspb.PoolBasicNode{
		NodeId: node.NodeID,
		Path:   node.getPathInt32(),
	}
}

// Joins pool based on pool id, creates a pool IF NOT EXIST
func JoinPool(poolID, nodeID, userID string, deviceInfo *PoolDeviceInfo, nodeChan chan PoolNodeChanMessage, userInfo *sspb.PoolUserInfo) { // TEMP userInfo
	pool := ActivePools.Upsert(poolID, nil, func(exist bool, valueInMap, newValue *Pool) *Pool {
		if !exist {
			return NewNodePool()
		}
		return valueInMap
	})

	pool.Lock()

	addedNode := pool.AddNode(nodeID, userID, deviceInfo, nodeChan)

	fmt.Println(len(pool.NodeMap), "node in pool", poolID)

	addNodeData := addedNode.getAddNodeData()

	initNodes := make([]*sspb.SSMessage_AddNodeData, 0, len(pool.NodeMap))

	// TEMP
	addedNode.UserInfo = userInfo

	users := make([]*sspb.PoolUserInfo, 1, len(pool.NodeMap))
	users[0] = userInfo

	for _, node := range pool.NodeMap {
		if node.NodeID == nodeID {
			continue
		}
		updateDeviceSSMsg := sspb.BuildSSMessage(
			sspb.SSMessage_ADD_USER,
			&sspb.SSMessage_AddUserData_{
				AddUserData: &sspb.SSMessage_AddUserData{
					UserInfo: userInfo,
				},
			},
		)
		node.NodeChan <- PoolNodeChanMessage{
			Type: PNCMT_SEND_SS_MESSAGE,
			Data: updateDeviceSSMsg,
		}
		users = append(users, node.UserInfo)
	}
	// TEMP

	for _, node := range pool.NodeMap {
		addNodeSSMsg := sspb.BuildSSMessage(
			sspb.SSMessage_ADD_NODE,
			&sspb.SSMessage_AddNodeData_{
				AddNodeData: addNodeData,
			},
		)
		initNodes = append(initNodes, node.getAddNodeData())

		if node.NodeID != nodeID {
			node.NodeChan <- PoolNodeChanMessage{
				Type: PNCMT_SEND_SS_MESSAGE,
				Data: addNodeSSMsg,
			}
		}
	}

	initPoolSSMsg := sspb.BuildSSMessage(
		sspb.SSMessage_INIT_POOL,
		&sspb.SSMessage_InitPoolData_{
			InitPoolData: &sspb.SSMessage_InitPoolData{
				InitNodes: initNodes,
				PoolInfo: &sspb.PoolInfo{
					PoolId:   poolID,
					PoolName: "TEST_POOL",
					Users:    users,
				},
			},
		},
	)

	addedNode.NodeChan <- PoolNodeChanMessage{
		Type: PNCMT_SEND_SS_MESSAGE,
		Data: initPoolSSMsg,
	}

	pool.Unlock()
}

// Removes node from pool and does the necessary position updates
func RemoveFromPool(poolID string, nodeID string) {
	pool, ok := ActivePools.Get(poolID)

	if !ok {
		return
	}

	cleanPool := false

	pool.Lock()

	promotedNodes, removed_node := pool.RemoveNode(nodeID)

	fmt.Println(len(pool.NodeMap), "node in pool", poolID)

	if len(pool.NodeMap) == 0 {
		cleanPool = true
	} else {
		promotedBasicNodes := make([]*sspb.PoolBasicNode, len(promotedNodes))
		for i := 0; i < len(promotedNodes); i++ {
			promotedBasicNodes[i] = promotedNodes[i].getBasicNode()
		}

		for _, node := range pool.NodeMap {
			removeNodeSSMsg := sspb.BuildSSMessage(
				sspb.SSMessage_REMOVE_NODE,
				&sspb.SSMessage_RemoveNodeData_{
					RemoveNodeData: &sspb.SSMessage_RemoveNodeData{
						NodeId:        nodeID,
						PromotedNodes: promotedBasicNodes,
					},
				},
			)
			node.NodeChan <- PoolNodeChanMessage{
				Type: PNCMT_SEND_SS_MESSAGE,
				Data: removeNodeSSMsg,
			}
		}

		// TEMP

		for _, node := range pool.NodeMap {
			removeDeviceSSMsg := sspb.BuildSSMessage(
				sspb.SSMessage_REMOVE_USER,
				&sspb.SSMessage_RemoveUserData_{
					RemoveUserData: &sspb.SSMessage_RemoveUserData{
						UserId: removed_node.UserID,
					},
				},
			)
			node.NodeChan <- PoolNodeChanMessage{
				Type: PNCMT_SEND_SS_MESSAGE,
				Data: removeDeviceSSMsg,
			}
		}
		// TEMP
	}

	pool.Unlock()

	if cleanPool {
		ActivePools.Remove(poolID)
	}
}

// Send Data to specific node in pool
func SendToNodeInPool(poolID string, nodeID string, targetNodeID string, msgType PoolNodeChanMessageType, data interface{}) bool {
	pool, ok := ActivePools.Get(poolID)

	if !ok {
		return false
	}

	var node *PoolNode

	pool.RLock()
	if node = pool.NodeMap[targetNodeID]; node != nil {
		node.NodeChan <- PoolNodeChanMessage{
			Type: msgType,
			Data: data,
		}
	}
	pool.RUnlock()

	return node != nil
}
