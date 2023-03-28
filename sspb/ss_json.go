package sspb

import (
	"google.golang.org/protobuf/encoding/protojson"
)

func (poolInfo *PoolInfo) MarshalJSON() ([]byte, error) {
	return (protojson.MarshalOptions{
		UseEnumNumbers:  true,
		EmitUnpopulated: true,
	}).Marshal(poolInfo)
}
