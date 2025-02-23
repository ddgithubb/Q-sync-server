package store

import (
	"sync-server/sspb"
	"time"

	"github.com/aidarkhanov/nanoid/v2"
)

const (
	MAX_POOL_NAME_LENGTH = 50
	POOL_ID_LENGTH       = 21
	MAIN_TEST_POOL_ID    = "MAIN_TEST_POOL_ID"
)

type Pool struct {
	PoolID      string
	PoolName    string
	Created     int64 // In milliseconds
	OwnerUserID string
	Users       []*sspb.PoolUserInfo
}

func (pool *Pool) update() bool {
	var err error
	if len(pool.Users) == 0 {
		err = deletePool(pool.PoolID)
	} else {
		err = putPool(pool)
	}
	return err == nil
}

func NewPool(poolName string, owner *sspb.PoolUserInfo) (*Pool, bool) {
	poolID, err := nanoid.GenerateString(nanoid.DefaultAlphabet, POOL_ID_LENGTH)
	if err != nil {
		return nil, false
	}

	pool := &Pool{
		PoolID:      poolID,
		PoolName:    poolName,
		Created:     time.Now().UnixMilli(),
		OwnerUserID: owner.UserId,
		Users:       []*sspb.PoolUserInfo{owner},
	}

	ok := pool.update()
	if !ok {
		return nil, false
	}

	return pool, true
}

func InitNewTestPool() {
	owner := &sspb.PoolUserInfo{
		UserId: "TEST_USER_ID",
		Devices: []*sspb.PoolDeviceInfo{
			{
				DeviceId: "TEST_DEVICE_ID",
			},
		},
	}
	pool := &Pool{
		PoolID:      "MAIN_TEST_POOL_ID",
		PoolName:    "TEST_POOL_NAME",
		Created:     time.Now().UnixMilli(),
		OwnerUserID: owner.UserId,
		Users:       []*sspb.PoolUserInfo{owner},
	}
	ok := pool.update()
	if !ok {
		panic("Failed to create test pool")
	}
}

func GetStoredPool(poolID string) (*Pool, error) {
	return getPool(poolID)
}

func (pool *Pool) AddUser(userInfo *sspb.PoolUserInfo) bool {
	for _, user := range pool.Users {
		if userInfo.UserId == user.UserId {
			return false
		}
	}

	pool.Users = append(pool.Users, userInfo)

	ok := pool.update()
	if !ok {
		pool.Users = pool.Users[:len(pool.Users)-1]
	}

	return ok
}

func (pool *Pool) RemoveUser(userID string) bool {
	for i, user := range pool.Users {
		if user.UserId == userID {
			pool.Users[i] = pool.Users[len(pool.Users)-1]
			pool.Users = pool.Users[:len(pool.Users)-1]

			ok := pool.update()
			if !ok {
				pool.Users = append(pool.Users, user)
			}

			return ok
		}
	}
	return false
}

func (pool *Pool) AddDevice(userID string, deviceInfo *sspb.PoolDeviceInfo) bool {
	for _, user := range pool.Users {
		if userID == user.UserId {
			for _, device := range user.Devices {
				if device.DeviceId == deviceInfo.DeviceId {
					return true
				}
			}
			user.Devices = append(user.Devices, deviceInfo)

			ok := pool.update()
			if !ok {
				user.Devices = user.Devices[:len(user.Devices)-1]
			}

			return ok
		}
	}
	return false
}

// Returns success and if user was removed
func (pool *Pool) RemoveDevice(userID string, deviceID string) (bool, bool) {
	for i, user := range pool.Users {
		if user.UserId == userID {
			for j, device := range user.Devices {
				if device.DeviceId == deviceID {
					user.Devices[j] = user.Devices[len(user.Devices)-1]
					user.Devices = user.Devices[:len(user.Devices)-1]

					userRemoved := false
					if len(user.Devices) == 0 {
						pool.Users[i] = pool.Users[len(pool.Users)-1]
						pool.Users = pool.Users[:len(pool.Users)-1]
						userRemoved = true
					}

					ok := pool.update()
					if !ok {
						user.Devices = append(user.Devices, device)
					}

					return ok, userRemoved
				}
			}
			break
		}
	}
	return false, false
}

func (pool *Pool) HasUser(userID string) bool {
	for _, user := range pool.Users {
		if user.UserId == userID {
			return true
		}
	}
	return false
}

func (pool *Pool) GetUser(userID string) (*sspb.PoolUserInfo, bool) {
	for _, user := range pool.Users {
		if user.UserId == userID {
			return user, true
		}
	}
	return nil, false
}

func (pool *Pool) GetPoolInfo() *sspb.PoolInfo {
	return &sspb.PoolInfo{
		PoolId:   pool.PoolID,
		PoolName: pool.PoolName,
		Users:    pool.GetUsersCopy(),
	}
}

func (pool *Pool) GetUsersCopy() []*sspb.PoolUserInfo {
	users := make([]*sspb.PoolUserInfo, len(pool.Users))
	copy(users, pool.Users)
	return users
}
