package auth

import (
	"sync-server/sspb"
	"sync-server/store"
	"time"

	"github.com/aidarkhanov/nanoid/v2"
	cmap "github.com/orcaman/concurrent-map/v2"
)

const (
	AUTH_TOKEN_SIZE             = 40
	AUTH_TOKEN_EXPIRE_DURATION  = 30 * 24 * time.Hour
	AUTH_TOKEN_CLEANER_INTERVAL = 24 * time.Hour
)

type AuthTokenData struct {
	UserID  string
	Device  *sspb.PoolDeviceInfo
	Token   string
	expires time.Time
}

func (authTokenData *AuthTokenData) Expires() time.Time {
	return authTokenData.expires
}

type AuthTokenStore = cmap.ConcurrentMap[string, *AuthTokenData]

// DeviceID -> AuthTokenData
var authTokenStore *AuthTokenStore = store.CreateTTLCacheWithInterval[*AuthTokenData](AUTH_TOKEN_CLEANER_INTERVAL)

func generateAuthTokenData(userID string, device *sspb.PoolDeviceInfo) (*AuthTokenData, bool) {
	token, err := nanoid.GenerateString(nanoid.DefaultAlphabet, 40)
	if err != nil {
		return nil, false
	}

	tokenData := &AuthTokenData{
		UserID:  userID,
		Device:  device,
		Token:   token,
		expires: time.Now().Add(AUTH_TOKEN_EXPIRE_DURATION),
	}

	return tokenData, true
}

func GenerateAndStoreAuthToken(userID string, device *sspb.PoolDeviceInfo) (string, bool) {
	tokenData, ok := generateAuthTokenData(userID, device)
	if !ok {
		return "", false
	}

	authTokenStore.Set(device.DeviceId, tokenData)

	return tokenData.Token, true
}

func VerifyAuthToken(deviceID string, token string) (*AuthTokenData, bool) {
	tokenData, ok := authTokenStore.Get(deviceID)
	if !ok {
		return nil, false
	}

	if tokenData.Token != token {
		return nil, false
	}

	return tokenData, true
}

func VerfiyAndRefreshAuthToken(deviceID string, token string) (*AuthTokenData, bool) {
	tokenData := authTokenStore.Upsert(deviceID, nil, func(exist bool, valueInMap, newValue *AuthTokenData) *AuthTokenData {
		if !exist {
			return nil
		}

		if valueInMap.Token != token {
			return nil
		}

		tokenData, ok := generateAuthTokenData(valueInMap.UserID, valueInMap.Device)
		if !ok {
			return nil
		}

		return tokenData
	})
	if tokenData == nil {
		return nil, false
	}

	return tokenData, true
}
