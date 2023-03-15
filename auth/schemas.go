package auth

import (
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
	DeviceID       string
	DeviceName     string
	DeviceType     int32
	CredentialData *protocol.CredentialCreationResponse
}

type BeginAuthenticateRequest struct {
	UserID   string
	DeviceID string
}

type BeginAuthenticateResponse struct {
	CredentialAssertion *protocol.CredentialAssertion
}

type FinishAuthenticateRequest struct {
	UserID         string
	DeviceID       string
	CredentialData *protocol.CredentialAssertionResponse
}

type FinishAuthenticateResponse struct {
	Token string
}
