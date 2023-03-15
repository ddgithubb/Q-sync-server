package store

import (
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	cmap "github.com/orcaman/concurrent-map/v2"
)

type SessionData struct {
	data *webauthn.SessionData
}

func (sessionData *SessionData) Expires() time.Time {
	return sessionData.data.Expires
}

type SessionStore = cmap.ConcurrentMap[string, *SessionData]

// DeviceID -> SessionData
var sessionStore *SessionStore = CreateTTLCache[*SessionData]()

func StoreSessionData(deviceID string, sessionData *webauthn.SessionData) bool {
	return sessionStore.SetIfAbsent(deviceID, &SessionData{
		data: sessionData,
	})
}

func RetrieveSessionData(deviceID string) (*webauthn.SessionData, bool) {
	var sessionData *webauthn.SessionData
	var ok bool
	sessionStore.RemoveCb(deviceID, func(key string, v *SessionData, exists bool) bool {
		sessionData = v.data
		ok = exists
		return true
	})
	return sessionData, ok
}
