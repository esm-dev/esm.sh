package server

import (
	bolt "go.etcd.io/bbolt"
)

const defaultBucket = "esm"

type boltDB struct {
	bolt *bolt.DB
}

func OpenBoltDB(filename string) (Database, error) {
	boltd, err := bolt.Open(filename, 0644, nil)
	if err != nil {
		return nil, err
	}
	err = boltd.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(defaultBucket))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &boltDB{boltd}, nil
}

func (db *boltDB) Stat() (stat Stat, err error) {
	err = db.bolt.View(func(tx *bolt.Tx) error {
		stat.Records = int64(tx.Bucket([]byte(defaultBucket)).Stats().KeyN)
		return nil
	})
	return stat, nil
}

func (db *boltDB) Get(key string) (value []byte, err error) {
	err = db.bolt.View(func(tx *bolt.Tx) error {
		value = tx.Bucket([]byte(defaultBucket)).Get([]byte(key))
		return nil
	})
	return
}

func (db *boltDB) Put(key string, value []byte) (err error) {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(defaultBucket)).Put([]byte(key), value)
	})
}

func (db *boltDB) Delete(key string) error {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(defaultBucket)).Delete([]byte(key))
	})
}

func (db *boltDB) Close() error {
	return db.bolt.Close()
}
