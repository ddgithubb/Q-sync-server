package store

import (
	"time"

	"github.com/aidarkhanov/nanoid/v2"
	cmap "github.com/orcaman/concurrent-map/v2"
)

const (
	AUTH_TOKEN_SIZE            = 40
	AUTH_TOKEN_EXPIRE_DURATION = 30 * 24 * time.Hour
	AUTH_TOKEN_CACHE_TTL       = 24 * time.Hour
)

type AuthTokenData struct {
	userID  string
	token   string
	expires time.Time
}

func (authTokenData *AuthTokenData) Expires() time.Time {
	return authTokenData.expires
}

type AuthTokenStore = cmap.ConcurrentMap[string, *AuthTokenData]

// DeviceID -> AuthTokenData
var authTokenStore *AuthTokenStore = CreateTTLCacheWithInterval[*AuthTokenData](AUTH_TOKEN_CACHE_TTL)

func GenerateAndStoreAuthTokenStore(userID, deviceID string) (string, bool) {
	token, err := nanoid.GenerateString(nanoid.DefaultAlphabet, 40)
	if err != nil {
		return "", false
	}

	authTokenStore.Set(deviceID, &AuthTokenData{
		userID:  userID,
		token:   token,
		expires: time.Now().Add(AUTH_TOKEN_EXPIRE_DURATION),
	})

	return token, true
}

func VerifyAuthToken(deviceID string, token string) (string, bool) {
	tokenData, ok := authTokenStore.Get(deviceID)
	if !ok {
		return "", false
	}

	if tokenData.token != token {
		return "", false
	}

	return tokenData.userID, true
}
