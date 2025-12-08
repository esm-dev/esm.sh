package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/esm-dev/esm.sh/internal/storage"
	lru "github.com/hashicorp/golang-lru/v2"
)

type MetaDB struct {
	cache   *lru.Cache[string, []byte]
	storage storage.Storage
	oldDB   Database
}

func NewMetaDB(storage storage.Storage) *MetaDB {
	cache, err := lru.New[string, []byte](lruCacheCapacity)
	if err != nil {
		panic(err)
	}
	return &MetaDB{cache: cache, storage: storage}
}

func (db *MetaDB) Get(key string) (value []byte, err error) {
	var cached bool
	value, cached = db.cache.Get(key)
	if cached {
		return
	}
	r, _, err := db.storage.Get(getMetaStoreKey(key))
	if err != nil {
		if err == storage.ErrNotFound && db.oldDB != nil {
			value, err = db.oldDB.Get(key)
			if err == nil {
				db.Put(key, value)
				db.cache.Add(key, value)
				return
			}
		}
		return
	}
	defer r.Close()
	value, err = io.ReadAll(r)
	if err == nil {
		db.cache.Add(key, value)
	}
	return
}

func (db *MetaDB) Put(key string, value []byte) (err error) {
	err = db.storage.Put(getMetaStoreKey(key), bytes.NewReader(value))
	if err == nil {
		db.cache.Add(key, value)
	}
	return
}

func (db *MetaDB) Delete(key string) (err error) {
	err = db.storage.Delete(getMetaStoreKey(key))
	if err == nil {
		db.cache.Remove(key)
	}
	return
}

func getMetaStoreKey(key string) string {
	data := sha256.Sum256([]byte(key))
	return "meta/" + hex.EncodeToString(data[:])
}
