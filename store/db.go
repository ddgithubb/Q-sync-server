package store

import (
	"bytes"
	"encoding/gob"

	"github.com/akrylysov/pogreb"
)

var (
	authDB *pogreb.DB = startAuthDB()
	poolDB *pogreb.DB = startPoolDB()
)

func startAuthDB() *pogreb.DB {
	db, err := pogreb.Open("ss-auth.db", nil)
	if err != nil {
		panic("error opening ss-auth.db " + err.Error())
	}
	return db
}

func startPoolDB() *pogreb.DB {
	db, err := pogreb.Open("ss-pool.db", nil)
	if err != nil {
		panic("error opening ss-pool.db " + err.Error())
	}
	return db
}

func CloseDB() {
	if authDB != nil {
		authDB.Close()
	}

	if poolDB != nil {
		poolDB.Close()
	}
}

func MarshalBinary(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	err := enc.Encode(v)
	return b.Bytes(), err
}

func UnmarshalBinary(buf []byte, pointer any) error {
	b := bytes.NewBuffer(buf)
	dec := gob.NewDecoder(b)
	return dec.Decode(pointer)
}

func putUserDevice(user *UserDevice) error {
	b, err := MarshalBinary(user)
	if err != nil {
		return err
	}
	return authDB.Put([]byte(user.DeviceInfo.DeviceId), b)
}

func getUserDevice(deviceID string) (*UserDevice, error) {
	b, err := authDB.Get([]byte(deviceID))
	if err != nil {
		return nil, err
	}
	user := new(UserDevice)
	err = UnmarshalBinary(b, user)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func putPool(pool *Pool) error {
	b, err := MarshalBinary(pool)
	if err != nil {
		return err
	}
	return poolDB.Put([]byte(pool.PoolID), b)
}

func getPool(poolID string) (*Pool, error) {
	b, err := poolDB.Get([]byte(poolID))
	if err != nil {
		return nil, err
	}
	pool := new(Pool)
	err = UnmarshalBinary(b, pool)
	if err != nil {
		return nil, err
	}
	return pool, nil
}
