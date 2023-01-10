package pool

import (
	"fmt"
	"sync"
	"sync-server/sstypes"
	"time"

	"github.com/segmentio/fasthash/fnv1a"
)

const CONCURRENCY = 1

var ActivePools ConcPoolShards = newConcPoolShards()

type ConcPoolShard struct {
	Table map[string]*Pool
	sync.RWMutex
}

type ConcPoolShards []*ConcPoolShard

func (cps ConcPoolShards) GetShard(poolID string) *ConcPoolShard {
	return cps[fnv1a.HashString32(poolID)%CONCURRENCY]
}

func newConcPoolShards() ConcPoolShards {
	shards := make([]*ConcPoolShard, CONCURRENCY)

	for i := 0; uint32(i) < CONCURRENCY; i++ {
		shards[i] = &ConcPoolShard{Table: make(map[string]*Pool)}
	}

	return shards
}

func (node *PoolNode) getAddNodeData() *sstypes.SSMessage_AddNodeData {
	return &sstypes.SSMessage_AddNodeData{
		NodeId:    node.NodeID,
		UserId:    node.UserID,
		DeviceId:  node.DeviceInfo.DeviceId,
		Path:      node.getPathInt32(),
		Timestamp: node.Created.UnixMilli(),
	}
}

func (node *PoolNode) getBasicNode() *sstypes.PoolBasicNode {
	return &sstypes.PoolBasicNode{
		NodeId: node.NodeID,
		Path:   node.getPathInt32(),
	}
}

// Joins pool based on pool id, creates a pool IF NOT EXIST
func JoinPool(poolID, nodeID, userID string, deviceInfo *PoolDeviceInfo, nodeChan chan PoolNodeChanMessage, userInfo *sstypes.PoolUserInfo) { // TEMP userInfo
	var pool *Pool

	poolShard := ActivePools.GetShard(poolID)

	poolShard.Lock()

	pool, ok := poolShard.Table[poolID]

	if !ok {
		pool = NewNodePool()
		poolShard.Table[poolID] = pool
	}

	poolShard.Unlock()

	pool.Lock()

	addedNode := pool.AddNode(nodeID, userID, deviceInfo, nodeChan)

	fmt.Println(len(pool.NodeMap), "node in pool", poolID)

	addNodeData := addedNode.getAddNodeData()

	initNodes := make([]*sstypes.SSMessage_AddNodeData, 0, len(pool.NodeMap) - 1)

	// TEMP
	addedNode.UserInfo = userInfo

	updateUsers := make([]*sstypes.SSMessage_UpdateUserData, 1, len(pool.NodeMap))
	updateUsers[0] = &sstypes.SSMessage_UpdateUserData{
		UserInfo: userInfo,
	}

	for _, node := range pool.NodeMap {
		if node.NodeID == nodeID {
			continue
		}
		updateDeviceSSMsg := sstypes.BuildSSMessage(
			sstypes.SSMessage_UPDATE_USER,
			&sstypes.SSMessage_UpdateUserData_{
				UpdateUserData: &sstypes.SSMessage_UpdateUserData{
					UserInfo: userInfo,
				},
			},
		)
		node.NodeChan <- PoolNodeChanMessage{
			Type: PNCMT_SEND_SS_MESSAGE,
			Data: updateDeviceSSMsg,
		}
		updateUsers = append(updateUsers, &sstypes.SSMessage_UpdateUserData{
			UserInfo: node.UserInfo,
		})
	}
	// TEMP
	
	for _, node := range pool.NodeMap {
		if node.NodeID == nodeID {
			continue
		}
		addNodeSSMsg := sstypes.BuildSSMessage(
			sstypes.SSMessage_ADD_NODE,
			&sstypes.SSMessage_AddNodeData_{
				AddNodeData: addNodeData,
			},
		)
		node.NodeChan <- PoolNodeChanMessage{
			Type: PNCMT_SEND_SS_MESSAGE,
			Data: addNodeSSMsg,
		}
		initNodes = append(initNodes, node.getAddNodeData())
	}

	initPoolSSMsg := sstypes.BuildSSMessage(
		sstypes.SSMessage_INIT_POOL,
		&sstypes.SSMessage_InitPoolData_{
			InitPoolData: &sstypes.SSMessage_InitPoolData{
				MyNode: addNodeData,
				InitNodes: initNodes,
				UpdateUsers: updateUsers,
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
	poolShard := ActivePools.GetShard(poolID)

	poolShard.RLock()

	pool, ok := poolShard.Table[poolID]

	poolShard.RUnlock()

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
		promotedBasicNodes := make([]*sstypes.PoolBasicNode, len(promotedNodes))
		for i := 0; i < len(promotedNodes); i++ {
			promotedBasicNodes[i] = promotedNodes[i].getBasicNode()
		}

		for _, node := range pool.NodeMap {
			removeNodeSSMsg := sstypes.BuildSSMessage(
				sstypes.SSMessage_REMOVE_NODE,
				&sstypes.SSMessage_RemoveNodeData_{
					RemoveNodeData: &sstypes.SSMessage_RemoveNodeData{
						NodeId:        nodeID,
						Timestamp:     time.Now().UnixMilli(),
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
			removeDeviceSSMsg := sstypes.BuildSSMessage(
				sstypes.SSMessage_REMOVE_USER,
				&sstypes.SSMessage_RemoveUserData_{
					RemoveUserData: &sstypes.SSMessage_RemoveUserData{
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
		poolShard.Lock()
		delete(poolShard.Table, poolID)
		poolShard.Unlock()
	}
}

// Send Data to specific node in pool
func SendToNodeInPool(poolID string, nodeID string, targetNodeID string, msgType PoolNodeChanMessageType, data interface{}) bool {
	poolShard := ActivePools.GetShard(poolID)

	poolShard.RLock()

	pool, ok := poolShard.Table[poolID]

	poolShard.RUnlock()

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
