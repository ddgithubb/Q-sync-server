package store

import (
	"sync-server/sspb"

	"github.com/aidarkhanov/nanoid/v2"
	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	USER_ID_LENGTH   = 21
	DEVICE_ID_LENGTH = 21
)

type UserDevice struct {
	UserID      string
	DisplayName string
	DeviceInfo  *sspb.PoolDeviceInfo
	Credential  *webauthn.Credential
}

func GenerateUserID() (string, error) {
	return nanoid.GenerateString(nanoid.DefaultAlphabet, USER_ID_LENGTH)
}

func GenerateDeviceID() (string, error) {
	return nanoid.GenerateString(nanoid.DefaultAlphabet, USER_ID_LENGTH)
}

func GetUserDevice(deviceID string) (*UserDevice, error) {
	return getUserDevice(deviceID)
}

func NewBaseUser(userID, displayName string) *UserDevice {
	user := &UserDevice{
		UserID:      userID,
		DisplayName: displayName,
	}

	return user
}

func (user *UserDevice) RegisterUserDevice(deviceInfo *sspb.PoolDeviceInfo, credential *webauthn.Credential) bool {
	user.DeviceInfo = deviceInfo
	user.Credential = credential

	err := putUserDevice(user)
	return err == nil
}

func (user *UserDevice) ValidateUserDevice(userID string) (*sspb.PoolDeviceInfo, *webauthn.Credential, bool) {
	if user.UserID != userID {
		return nil, nil, false
	}
	return user.DeviceInfo, user.Credential, true
}

func (user *UserDevice) PoolUserInfo() *sspb.PoolUserInfo {
	return &sspb.PoolUserInfo{
		UserId:      user.UserID,
		DisplayName: user.DisplayName,
		Devices:     []*sspb.PoolDeviceInfo{user.DeviceInfo},
	}
}

func (user *UserDevice) WebAuthnID() []byte {
	return []byte(user.UserID)
}

func (user *UserDevice) WebAuthnName() string {
	return user.WebAuthnDisplayName()
}

func (user *UserDevice) WebAuthnDisplayName() string {
	return user.DisplayName
}

func (user *UserDevice) WebAuthnCredentials() []webauthn.Credential {
	return []webauthn.Credential{*user.Credential}
}

func (user *UserDevice) WebAuthnIcon() string {
	return ""
}
