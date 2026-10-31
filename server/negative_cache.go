package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"path/filepath"
	"time"

	"github.com/ije/gox/log"
	bolt "go.etcd.io/bbolt"
)

const (
	packageNotFoundTTL = 10 * time.Minute
	// a timed out build is not retried meanwhile, every retry would hold a build slot for the whole timeout again
	buildTimeoutTTL = 10 * time.Minute
	// cap of "permanent" (ttl 0) records, whose keys can come from arbitrary request paths
	negativeCacheMaxTTL = 7 * 24 * time.Hour
)

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
		value := tx.Bucket([]byte("records")).Get(negativeCacheKey(key))
		if len(value) > 8 {
			expires := int64(binary.BigEndian.Uint64(value[:8]))
			if time.Now().UnixMilli() < expires {
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
	if ttl == 0 {
		ttl = negativeCacheMaxTTL
	}
	err := db.Batch(func(tx *bolt.Tx) error {
		value := make([]byte, 8+len(message))
		binary.BigEndian.PutUint64(value[:8], uint64(time.Now().Add(ttl).UnixMilli()))
		copy(value[8:], message)
		return tx.Bucket([]byte("records")).Put(negativeCacheKey(key), value)
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
		p := negativeCacheKey(key)
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

// negativeCacheKey keeps the head of a key, which purges match by prefix, and hashes the rest.
// [warning] keys contain request paths, an unbounded key would let a request store any amount of data
func negativeCacheKey(key string) []byte {
	const maxLen = 512
	if len(key) <= maxLen {
		return []byte(key)
	}
	sum := sha256.Sum256([]byte(key[maxLen-64:]))
	return hex.AppendEncode([]byte(key[:maxLen-64]), sum[:])
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
				// expiry 0 is a legacy "permanent" record, drop it as well
				if expires <= now.UnixMilli() {
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
