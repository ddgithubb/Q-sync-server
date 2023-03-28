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

// Debugging only
func PrintAuthDB() {
	println("Printing AUTHDB")
	iter := authDB.Items()
	for {
		b1, _, err := iter.Next()
		if err != nil {
			break
		}
		println("AUTHDB user: ", string(b1))
	}
}

// Debugging only
func PrintPoolDB() {
	println("Printing POOLDB")
	iter := poolDB.Items()
	for {
		b1, _, err := iter.Next()
		if err != nil {
			break
		}
		println("POOLDB poolID: ", string(b1))
	}
}

func putUserDevice(user *UserDevice) error {
	b, err := MarshalBinary(user)
	if err != nil {
		return err
	}

	err = authDB.Put([]byte(user.DeviceInfo.DeviceId), b)
	return err
}

func putPool(pool *Pool) error {
	b, err := MarshalBinary(pool)
	if err != nil {
		return err
	}

	err = poolDB.Put([]byte(pool.PoolID), b)
	return err
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

func deletePool(poolID string) error {
	return poolDB.Delete([]byte(poolID))
}
