package server

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"time"

	"github.com/ije/gox/log"
	bolt "go.etcd.io/bbolt"
)

const packageNotFoundTTL = 10 * time.Minute

var negativeCache *negativeCacheDB

type negativeCacheDB struct {
	*bolt.DB
	logger *log.Logger
}

func openNegativeCache(filename string, logger *log.Logger) (*negativeCacheDB, error) {
	if err := ensureDir(filepath.Dir(filename)); err != nil {
		return nil, err
	}
	db, err := bolt.Open(filename, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("records"))
		return err
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &negativeCacheDB{db, logger}, nil
}

func (db *negativeCacheDB) get(key string) (message string) {
	if db == nil {
		return
	}
	err := db.View(func(tx *bolt.Tx) error {
		value := tx.Bucket([]byte("records")).Get([]byte(key))
		if len(value) > 8 {
			expires := int64(binary.BigEndian.Uint64(value[:8]))
			if expires == 0 || time.Now().UnixMilli() < expires {
				message = string(value[8:])
			}
		}
		return nil
	})
	if err != nil {
		db.logger.Errorf("negative cache read: %v", err)
	}
	return
}

func (db *negativeCacheDB) put(key, message string, ttl time.Duration) {
	if db == nil {
		return
	}
	value := make([]byte, 8+len(message))
	if ttl != 0 {
		binary.BigEndian.PutUint64(value[:8], uint64(time.Now().Add(ttl).UnixMilli()))
	}
	copy(value[8:], message)
	err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("records")).Put([]byte(key), value)
	})
	if err != nil {
		db.logger.Errorf("negative cache write: %v", err)
	}
}

func (db *negativeCacheDB) delete(key string, prefix bool) (keys []string, err error) {
	if db == nil {
		return
	}
	err = db.Update(func(tx *bolt.Tx) error {
		cursor := tx.Bucket([]byte("records")).Cursor()
		p := []byte(key)
		for k, _ := cursor.Seek(p); k != nil && bytes.HasPrefix(k, p); k, _ = cursor.Next() {
			if !prefix && !bytes.Equal(k, p) {
				break
			}
			keys = append(keys, string(k))
			if err := cursor.Delete(); err != nil {
				return err
			}
		}
		return nil
	})
	return
}

func (db *negativeCacheDB) gc(now time.Time) {
	if db == nil {
		return
	}
	err := db.Update(func(tx *bolt.Tx) error {
		cursor := tx.Bucket([]byte("records")).Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			if len(v) > 8 {
				expires := int64(binary.BigEndian.Uint64(v[:8]))
				if expires != 0 && expires <= now.UnixMilli() {
					if err := cursor.Delete(); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		db.logger.Errorf("negative cache cleanup: %v", err)
	}
}
