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

type PoolUser struct {
	UserID      string
	DisplayName string
	Devices     []*PoolDevice
}

type PoolDevice struct {
	DeviceInfo *sspb.PoolDeviceInfo
	Credential *webauthn.Credential
}

func GenerateUserID() (string, error) {
	return nanoid.GenerateString(nanoid.DefaultAlphabet, USER_ID_LENGTH)
}

func GenerateDeviceID() (string, error) {
	return nanoid.GenerateString(nanoid.DefaultAlphabet, USER_ID_LENGTH)
}

func (user *PoolUser) PoolUserInfo() *sspb.PoolUserInfo {
	devices := make([]*sspb.PoolDeviceInfo, len(user.Devices))
	for i := 0; i < len(devices); i++ {
		devices[i] = user.Devices[i].DeviceInfo
	}

	return &sspb.PoolUserInfo{
		UserId:      user.UserID,
		DisplayName: user.DisplayName,
		Devices:     devices,
	}
}

func (user *PoolUser) AddDevice(deviceInfo *sspb.PoolDeviceInfo, credential *webauthn.Credential) {
	user.Devices = append(user.Devices, &PoolDevice{
		DeviceInfo: deviceInfo,
		Credential: credential,
	})
}

func (user *PoolUser) GetDevice(deviceID string) (*PoolDevice, bool) {
	for _, device := range user.Devices {
		if device.DeviceInfo.DeviceId == deviceID {
			return device, true
		}
	}
	return nil, false
}

func (user *PoolUser) WebAuthnID() []byte {
	return []byte(user.UserID)
}

func (user *PoolUser) WebAuthnName() string {
	return user.WebAuthnDisplayName()
}

func (user *PoolUser) WebAuthnDisplayName() string {
	return user.DisplayName
}

func (user *PoolUser) WebAuthnCredentials() []webauthn.Credential {
	credentials := make([]webauthn.Credential, len(user.Devices))
	for i := 0; i < len(credentials); i++ {
		credentials[i] = *user.Devices[i].Credential
	}
	return credentials
}

func (user *PoolUser) WebAuthnIcon() string {
	return ""
}
