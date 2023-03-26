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

	tempUser := store.NewBaseUser(userID, displayName)

	options, session, err := serverWebAuthN.BeginRegistration(tempUser)
	if err != nil {
		return nil, "", false
	}

	store.StoreSessionData(deviceID, session)
	return options, deviceID, true
}

func FinishRegistration(deviceInfo *sspb.PoolDeviceInfo, credentialData *protocol.ParsedCredentialCreationData) (string, bool) {
	session, exist := store.RetrieveSessionData(deviceInfo.DeviceId)
	if !exist {
		return "", false
	}

	userDevice := store.NewBaseUser(string(session.UserID), session.UserDisplayName)

	credential, err := serverWebAuthN.CreateCredential(userDevice, *session, credentialData)
	if err != nil {
		return "", false
	}

	ok := userDevice.RegisterUserDevice(deviceInfo, credential)
	if !ok {
		return "", false
	}

	token, ok := GenerateAndStoreAuthToken(userDevice.UserID, deviceInfo)
	if !ok {
		return "", false
	}

	return token, true
}

func BeginAuthenticate(userID, deviceID string) (*protocol.CredentialAssertion, bool) {
	userDevice, err := store.GetUserDevice(deviceID)
	if err != nil {
		return nil, false
	}

	_, credential, ok := userDevice.ValidateUserDevice(userID)
	if !ok {
		return nil, false
	}

	credentialDescriptor := []protocol.CredentialDescriptor{
		credential.Descriptor(),
	}

	options, session, err := serverWebAuthN.BeginLogin(userDevice, webauthn.WithAllowedCredentials(credentialDescriptor))
	if err != nil {
		return nil, false
	}

	store.StoreSessionData(deviceID, session)
	return options, true
}

func FinishAuthenticate(userID, deviceID string, credentialData *protocol.ParsedCredentialAssertionData) (string, bool) {
	session, exist := store.RetrieveSessionData(deviceID)
	if !exist {
		return "", false
	}

	userDevice, err := store.GetUserDevice(deviceID)
	if err != nil {
		return "", false
	}

	deviceInfo, _, ok := userDevice.ValidateUserDevice(userID)
	if !ok {
		return "", false
	}

	_, err = serverWebAuthN.ValidateLogin(userDevice, *session, credentialData)
	if err != nil {
		return "", false
	}

	token, ok := GenerateAndStoreAuthToken(userID, deviceInfo)
	if !ok {
		return "", false
	}

	return token, true
}

func BeginDeviceRegistration() {
	// TODO
	// Consistency issue with addDevice (add lightweight locking mechanism?)
	// No more consistency issue!
	// Make sure to let client generate the link token (because if an existing device
	// generates it, then there is a higher chance the token will be stolen)
}

func FinishDeviceRegistration() {
	// TODO
}
