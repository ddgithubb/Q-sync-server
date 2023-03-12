package store

import (
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	cmap "github.com/orcaman/concurrent-map/v2"
)

const (
	CLEAN_INTERVAL time.Duration = 5 * time.Minute
)

type SessionStore struct {
	store cmap.ConcurrentMap[string, *webauthn.SessionData]
}

// DeviceID -> SessionData
var sessionStore *SessionStore = createSessionStore()

func createSessionStore() *SessionStore {
	sessionStore := &SessionStore{}
	sessionStore.store = cmap.New[*webauthn.SessionData]()
	sessionStore.startCleaner()
	return sessionStore
}

func StoreSessionData(deviceID string, sessionData *webauthn.SessionData) bool {
	return sessionStore.store.SetIfAbsent(deviceID, sessionData)
}

func RetrieveSessionData(deviceID string) (*webauthn.SessionData, bool) {
	var sessionData *webauthn.SessionData
	var ok bool
	sessionStore.store.RemoveCb(deviceID, func(key string, v *webauthn.SessionData, exists bool) bool {
		sessionData = v
		ok = exists
		return true
	})
	return sessionData, ok
}

func (sessionStore *SessionStore) startCleaner() {
	go func() {
		ticker := time.NewTicker(CLEAN_INTERVAL)
		defer ticker.Stop()
		for {
			<-ticker.C
			now := time.Now()
			iter := sessionStore.store.IterBuffered()
			for entry := range iter {
				if now.After(entry.Val.Expires) {
					sessionStore.store.Remove(entry.Key)
				}
			}
		}
	}()
}
