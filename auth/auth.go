package auth

import (
	"sync-server/sspb"
	"sync-server/store"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

var webauthnConfig *webauthn.Config = &webauthn.Config{
	RPDisplayName: "PoolNet",
	RPID:          "localhost",
	RPOrigins:     []string{"http://localhost:3000"},
	AuthenticatorSelection: protocol.AuthenticatorSelection{
		RequireResidentKey: protocol.ResidentKeyNotRequired(),
		ResidentKey:        protocol.ResidentKeyRequirementDiscouraged,
		UserVerification:   protocol.VerificationPreferred,
	},
	EncodeUserIDAsString: true,
}

var serverWebAuthN *webauthn.WebAuthn = createServerWebAuthN()

func createServerWebAuthN() *webauthn.WebAuthn {
	w, err := webauthn.New(webauthnConfig)
	if err != nil {
		panic("failed to create server webauthn " + err.Error())
	}
	return w
}

func BeginRegistration(displayName string) (*protocol.CredentialCreation, string, bool) {
	userID, err := store.GenerateUserID()
	if err != nil {
		return nil, "", false
	}

	deviceID, err := store.GenerateDeviceID()
	if err != nil {
		return nil, "", false
	}

	tempUser := &store.PoolUser{
		UserID:      userID,
		DisplayName: displayName,
		Devices:     []*store.PoolDevice{},
	}

	options, session, err := serverWebAuthN.BeginRegistration(tempUser)
	if err != nil {
		return nil, "", false
	}

	store.StoreSessionData(deviceID, session)
	return options, deviceID, true
}

func FinishRegistration(deviceInfo *sspb.PoolDeviceInfo, credentialData *protocol.ParsedCredentialCreationData) bool {
	session, exist := store.RetrieveSessionData(deviceInfo.DeviceId)
	if !exist {
		return false
	}

	user := &store.PoolUser{
		UserID:      string(session.UserID),
		DisplayName: session.UserDisplayName,
		Devices:     []*store.PoolDevice{},
	}

	credential, err := serverWebAuthN.CreateCredential(user, *session, credentialData)
	if err != nil {
		return false
	}

	user.AddDevice(deviceInfo, credential)
	err = store.PutUser(user)

	return err == nil
}

func BeginAuthenticate(userID, deviceID string) (*protocol.CredentialAssertion, bool) {
	user, err := store.GetUser(userID)
	if err != nil {
		return nil, false
	}

	device, exists := user.GetDevice(deviceID)
	if !exists {
		return nil, false
	}

	credentialDescriptor := []protocol.CredentialDescriptor{
		device.Credential.Descriptor(),
	}

	options, session, err := serverWebAuthN.BeginLogin(user, webauthn.WithAllowedCredentials(credentialDescriptor))
	if err != nil {
		return nil, false
	}

	store.StoreSessionData(deviceID, session)
	return options, true
}

func FinishAuthenticate(userID, deviceID string, credentialData *protocol.ParsedCredentialAssertionData) bool {
	session, exist := store.RetrieveSessionData(deviceID)
	if !exist {
		return false
	}

	user, err := store.GetUser(userID)
	if err != nil {
		return false
	}

	_, err = serverWebAuthN.ValidateLogin(user, *session, credentialData)
	return err == nil
}

func BeginDeviceRegistration() {
	// TODO
}

func FinishDeviceRegistration() {
	// TODO
}
