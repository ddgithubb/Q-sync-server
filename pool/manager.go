package pool

import (
	"fmt"
	"sync-server/sspb"
	"sync-server/store"

	cmap "github.com/orcaman/concurrent-map/v2"
)

var ActivePools cmap.ConcurrentMap[string, *Pool] = cmap.New[*Pool]()

func fetchPool(poolID string) (*Pool, bool) {
	pool, ok := ActivePools.Get(poolID)
	if ok {
		return pool, true
	}

	poolStore, err := store.GetStoredPool(poolID)
	if err != nil {
		fmt.Println("Failed to fetch pool from store:", poolID, err.Error())
		return nil, false
	}

	newPool := NewNodePool(poolStore)

	pool = ActivePools.Upsert(poolID, nil, func(exist bool, valueInMap, newValue *Pool) *Pool {
		if !exist {
			return newPool
		}
		return valueInMap
	})

	return pool, true
}

func performPoolAction(poolID string, action func(pool *Pool) bool) bool {
	pool, ok := fetchPool(poolID)
	if !ok {
		return false
	}

	pool.Lock()

	ok = action(pool)

	if pool.IsEmpty() {
		ActivePools.Remove(poolID)
	}

	pool.Unlock()

	return ok
}

func UserExistsInPool(poolID string, userID string) bool {
	return performPoolAction(poolID, func(pool *Pool) bool {
		return pool.PoolStore.HasUser(userID)
	})
}

func JoinPool(poolID string, user *sspb.PoolUserInfo) (*sspb.PoolInfo, bool) {
	var poolInfo *sspb.PoolInfo
	ok := performPoolAction(poolID, func(pool *Pool) bool {
		ok := pool.PoolStore.AddUser(user)
		if !ok {
			return false
		}
		for _, node := range pool.NodeMap {
			updateDeviceSSMsg := sspb.BuildSSMessage(
				sspb.SSMessage_ADD_USER,
				&sspb.SSMessage_AddUserData_{
					AddUserData: &sspb.SSMessage_AddUserData{
						UserInfo: user,
					},
				},
			)
			node.NodeChan <- PoolNodeChanMessage{
				Type: PNCMT_SEND_SS_MESSAGE,
				Data: updateDeviceSSMsg,
			}
		}
		poolInfo = pool.PoolInfo()
		return true
	})

	return poolInfo, ok
}

func LeavePool(poolID, userID, deviceID string) bool {
	return performPoolAction(poolID, func(pool *Pool) bool {
		ok, userRemoved := pool.PoolStore.RemoveDevice(userID, deviceID)
		if !ok {
			return false
		}
		if userRemoved {
			for _, node := range pool.NodeMap {
				removeDeviceSSMsg := sspb.BuildSSMessage(
					sspb.SSMessage_REMOVE_USER,
					&sspb.SSMessage_RemoveUserData_{
						RemoveUserData: &sspb.SSMessage_RemoveUserData{
							UserId: userID,
						},
					},
				)
				node.NodeChan <- PoolNodeChanMessage{
					Type: PNCMT_SEND_SS_MESSAGE,
					Data: removeDeviceSSMsg,
				}
			}
		}
		return true
	})
}

func ConnectToPool(poolID, nodeID, userID string, device *sspb.PoolDeviceInfo, nodeChan chan PoolNodeChanMessage) bool {
	return performPoolAction(poolID, func(pool *Pool) bool {
		if !pool.PoolStore.AddDevice(userID, device) {
			return false
		}

		addedNode := pool.AddNode(nodeID, userID, device, nodeChan)

		fmt.Println(len(pool.NodeMap), "node in pool", poolID)

		addNodeData := addedNode.getAddNodeData()

		initNodes := make([]*sspb.SSMessage_AddNodeData, 0, len(pool.NodeMap))

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
					PoolInfo:  pool.PoolInfo(),
				},
			},
		)

		addedNode.NodeChan <- PoolNodeChanMessage{
			Type: PNCMT_SEND_SS_MESSAGE,
			Data: initPoolSSMsg,
		}

		return true
	})
}

func DisconnectFromPool(poolID string, nodeID string) {
	pool, ok := ActivePools.Get(poolID)

	if !ok {
		return
	}

	pool.Lock()

	promotedNodes, _ := pool.RemoveNode(nodeID)

	fmt.Println(len(pool.NodeMap), "node in pool", poolID)

	if pool.IsEmpty() {
		ActivePools.Remove(poolID)
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
	}

	pool.Unlock()
}

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
