package clustertree

import (
	"sync"

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
func JoinPool(poolID string, nodeID string, nodeChan chan NodeChanMessage) {
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
	pool.AddNode(nodeID, nodeChan)
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

	pool.Lock()
	pool.RemoveNode(nodeID)
	pool.Unlock()
}

// Send Data to specific node in pool
func SendToNodeInPool(poolID string, nodeID string, targetNodeID string, op int, data interface{}) {
	poolShard := ActivePools.GetShard(poolID)

	poolShard.RLock()

	pool, ok := poolShard.Table[poolID]

	poolShard.RUnlock()

	if !ok {
		return
	}

	pool.RLock()
	if node := pool.NodeMap[targetNodeID]; node != nil {
		node.NodeChan <- NodeChanMessage{
			Op:           op,
			Action:       SERVER_ACTION,
			TargetNodeID: nodeID,
			Data:         data,
		}
	}
	pool.RUnlock()
}
