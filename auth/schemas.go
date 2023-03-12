package auth

import (
	"sync-server/sspb"

	"github.com/go-webauthn/webauthn/protocol"
)

type BeginRegisterRequest struct {
	DisplayName string
}

type BeginRegisterResponse struct {
	DeviceID           string
	CredentialCreation *protocol.CredentialCreation
}

type FinishRegisterRequest struct {
	DeviceInfo     *sspb.PoolDeviceInfo
	CredentialData *protocol.ParsedCredentialCreationData
}
