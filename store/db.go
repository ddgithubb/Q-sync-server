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

func PutUser(user *PoolUser) error {
	b, err := MarshalBinary(user)
	if err != nil {
		return err
	}
	return authDB.Put(user.WebAuthnID(), b)
}

func GetUser(userID string) (*PoolUser, error) {
	b, err := authDB.Get([]byte(userID))
	if err != nil {
		return nil, err
	}
	user := new(PoolUser)
	err = UnmarshalBinary(b, user)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func HasUser(userID string) (bool, error) {
	return authDB.Has([]byte(userID))
}
