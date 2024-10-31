package server

import (
	"bytes"
	"net/url"

	bolt "go.etcd.io/bbolt"
)

var defaultBucketName = []byte("esm")

type Bolt struct{}

func (driver *Bolt) Open(path string, options url.Values) (DataBase, error) {
	db, err := bolt.Open(path, 0644, nil)
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(defaultBucketName)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &BoltDBStub{db}, nil
}

type BoltDBStub struct {
	db *bolt.DB
}

func (i *BoltDBStub) Get(key string) (value []byte, err error) {
	err = i.db.View(func(tx *bolt.Tx) error {
		value = tx.Bucket(defaultBucketName).Get([]byte(key))
		return nil
	})
	return
}

func (i *BoltDBStub) Put(key string, value []byte) (err error) {
	return i.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(defaultBucketName).Put([]byte(key), value)
	})
}

func (i *BoltDBStub) Delete(key string) error {
	return i.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(defaultBucketName).Delete([]byte(key))
	})
}

func (i *BoltDBStub) DeleteAll(prefix string) (deletedKeys []string, err error) {
	err = i.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(defaultBucketName)
		cursor := bucket.Cursor()
		prefixBytes := []byte(prefix)
		for k, _ := cursor.Seek(prefixBytes); k != nil && bytes.HasPrefix(k, prefixBytes); k, _ = cursor.Next() {
			deletedKeys = append(deletedKeys, string(k))
		}
		for _, k := range deletedKeys {
			err := bucket.Delete([]byte(k))
			if err != nil {
				return err
			}
		}
		return nil
	})
	return
}

func (i *BoltDBStub) Close() error {
	return i.db.Close()
}

func init() {
	RegisterDBDriver("bolt", &Bolt{})
}
