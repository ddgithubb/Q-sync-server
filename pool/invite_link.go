package pool

import (
	"strings"
	"sync-server/store"
	"time"

	"github.com/aidarkhanov/nanoid/v2"
	cmap "github.com/orcaman/concurrent-map/v2"
)

const (
	INVITE_LINK_TOKEN_ALPHABET   = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	INVITE_LINK_TOKEN_SIZE       = 8
	INVITE_LINK_PREFIX           = "invite"
	INVITE_LINK_CLEANER_INTERVAL = 1 * time.Hour
	INVITE_LINK_DEFAULT_DURATION = 24 * time.Hour
)

type InvitePoolLink struct {
	poolID    string // Pool associated with link
	unlimited bool   // Whether or not there are unlimited uses
	uses      int    // Number of uses left
	expires   time.Time
}

func (invitePoolLink *InvitePoolLink) Expires() time.Time {
	return invitePoolLink.expires
}

type InviteLinkStore = cmap.ConcurrentMap[string, *InvitePoolLink]

// Link string -> InvitePoolLink
var inviteLinkStore *InviteLinkStore = store.CreateTTLCacheWithInterval[*InvitePoolLink](INVITE_LINK_CLEANER_INTERVAL)

func generateInviteLink() (string, error) {
	linkToken, err := nanoid.GenerateString(INVITE_LINK_TOKEN_ALPHABET, 8)
	if err != nil {
		return "", err
	}
	return INVITE_LINK_PREFIX + ":" + linkToken, nil
}

func generateAndStoreInviteLink(poolID string, validDuration time.Duration, unlimited bool, uses int) (string, bool) {
	link, err := generateInviteLink()
	if err != nil {
		return "", false
	}

	inviteLinkStore.Set(link, &InvitePoolLink{
		poolID:    poolID,
		unlimited: unlimited,
		uses:      uses,
		expires:   time.Now().Add(validDuration),
	})

	return link, true
}

// Not in use
func GenerateAndStoreInviteLink(poolID string, validDuration time.Duration) (string, bool) {
	return generateAndStoreInviteLink(poolID, validDuration, true, 0)
}

func GenerateAndStoreNewInviteLinkDefault(poolID string) (string, bool) {
	return generateAndStoreInviteLink(poolID, INVITE_LINK_DEFAULT_DURATION, true, 0)
}

// Not in use
func GenerateAndStoreInviteLinkWithUses(poolID string, validDuration time.Duration, uses int) (string, bool) {
	if uses == 0 {
		return "", false
	}

	return generateAndStoreInviteLink(poolID, validDuration, false, uses)
}

// Returns poolID if successful, otherwise bool is false
func UseInviteLink(link string) (string, bool) {
	split := strings.Split(link, ":")
	if len(split) != 2 || split[0] != INVITE_LINK_PREFIX {
		return "", false
	}

	invitePoolLink, ok := inviteLinkStore.Get(link)
	if !ok {
		return "", false
	}

	if !invitePoolLink.unlimited {
		invitePoolLink.uses--
		if invitePoolLink.uses <= 0 {
			inviteLinkStore.Remove(link)
		}
	}

	return invitePoolLink.poolID, true
}
