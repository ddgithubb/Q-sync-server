package pool

import "sync-server/sspb"

type CreatePoolRequest struct {
	PoolName string
}

type CreatePoolResponse struct {
	PoolInfo *sspb.PoolInfo
}

type JoinPoolRequest struct {
	InviteLink string
}

type JoinPoolResponse struct {
	PoolInfo *sspb.PoolInfo
}

type CreateInviteToPoolResponse struct {
	InviteLink string
}
