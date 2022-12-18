package storage

import (
	"net/url"

	bolt "go.etcd.io/bbolt"
)

var defaultBucket = []byte("default")

type boltDBDriver struct{}

func (driver *boltDBDriver) Open(path string, options url.Values) (DataBase, error) {
	db, err := bolt.Open(path, 0644, nil)
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(defaultBucket)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &boltDB{db}, nil
}

type boltDB struct {
	db *bolt.DB
}

func (i *boltDB) Get(key string) (value []byte, err error) {
	err = i.db.View(func(tx *bolt.Tx) error {
		value = tx.Bucket(defaultBucket).Get([]byte(key))
		return nil
	})
	return
}

func (i *boltDB) Put(key string, value []byte) (err error) {
	return i.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(defaultBucket).Put([]byte(key), value)
	})
}

func (i *boltDB) Delete(key string) error {
	return i.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(defaultBucket).Delete([]byte(key))
	})
}

func (i *boltDB) Close() error {
	return i.db.Close()
}

func init() {
	RegisterDB("bolt", &boltDBDriver{})
}
