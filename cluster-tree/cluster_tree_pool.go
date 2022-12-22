package clustertree

import (
	"sync"
	"time"

	"github.com/segmentio/fasthash/fnv1a"
)

const CONCURRENCY = 32

var ActivePools ConcPoolShards = newConcPoolShards()

type ConcPoolShard struct {
	Table map[string]*NodePool
	sync.RWMutex
}

type ConcPoolShards []*ConcPoolShard

func (cps ConcPoolShards) GetShard(poolID string) *ConcPoolShard {
	return cps[fnv1a.HashString32(poolID)%CONCURRENCY]
}

func newConcPoolShards() ConcPoolShards {
	shards := make([]*ConcPoolShard, CONCURRENCY)

	for i := 0; uint32(i) < CONCURRENCY; i++ {
		shards[i] = &ConcPoolShard{Table: make(map[string]*NodePool)}
	}

	return shards
}

// Joins pool based on pool id, creates a pool IF NOT EXIST
func JoinPool(poolID string, nodeID string, nodeInfo *NodeInfo, nodeChan chan NodeChanMessage) {
	var pool *NodePool

	poolShard := ActivePools.GetShard(poolID)

	poolShard.Lock()

	pool, ok := poolShard.Table[poolID]

	if !ok {
		pool = NewNodePool()
		poolShard.Table[poolID] = pool
	}

	poolShard.Unlock()

	pool.Lock()
	newNode := pool.AddNode(nodeID, nodeInfo, nodeChan)
	initNodesData := make(AddNodesData, 1, len(pool.NodeMap))
	addNewNodeData := make(AddNodesData, 1)
	newAddNodeData := newNode.getAddNodeData()
	addNewNodeData[0] = newAddNodeData
	initNodesData[0] = newAddNodeData
	for _, node := range pool.NodeMap {
		if node.NodeID == nodeID {
			continue
		}
		node.NodeChan <- NodeChanMessage{
			Op:     2010,
			Action: SERVER_ACTION,
			Data:   addNewNodeData,
		}
		initNodesData = append(initNodesData, node.getAddNodeData())
	}
	newNode.NodeChan <- NodeChanMessage{
		Op:     2010,
		Action: SERVER_ACTION,
		Data:   initNodesData,
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
	promotedNodes := pool.RemoveNode(nodeID)
	if len(pool.NodeMap) == 0 {
		cleanPool = true
	} else {
		promotedBasicNodes := make([]*BasicNode, len(promotedNodes))
		for i := 0; i < len(promotedNodes); i++ {
			promotedBasicNodes[i] = promotedNodes[i].getBasicNode()
		}
		removeNodeData := &RemoveNodeData{
			NodeID:        nodeID,
			Timestamp:     time.Now().UnixMilli(),
			PromotedNodes: promotedBasicNodes,
		}
		for _, node := range pool.NodeMap {
			node.NodeChan <- NodeChanMessage{
				Op:     2011,
				Action: SERVER_ACTION,
				Data:   removeNodeData,
			}
		}
	}
	pool.Unlock()

	if cleanPool {
		poolShard.Lock()
		delete(poolShard.Table, poolID)
		poolShard.Unlock()
	}
}

// Send Data to specific node in pool
func SendToNodeInPool(poolID string, nodeID string, targetNodeID string, op int, data interface{}) bool {
	poolShard := ActivePools.GetShard(poolID)

	poolShard.RLock()

	pool, ok := poolShard.Table[poolID]

	poolShard.RUnlock()

	if !ok {
		return false
	}

	var node *Node

	pool.RLock()
	if node = pool.NodeMap[targetNodeID]; node != nil {
		node.NodeChan <- NodeChanMessage{
			Op:           op,
			Action:       SERVER_ACTION,
			TargetNodeID: nodeID,
			Data:         data,
		}
	}
	pool.RUnlock()

	return node != nil
}
