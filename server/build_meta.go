package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/storage"
	lru "github.com/hashicorp/golang-lru/v2"
	bolt "go.etcd.io/bbolt"
)

type BuildMeta struct {
	CJS           bool
	CSSInJS       bool
	TypesOnly     bool
	ExportDefault bool
	CSSEntry      string
	Dts           string
	Imports       []string
	Integrity     string
}

func encodeBuildMeta(meta *BuildMeta) []byte {
	buf := bytes.NewBuffer(nil)
	buf.Write([]byte{'E', 'S', 'M', '\r', '\n'})
	if meta.CJS {
		buf.Write([]byte{'j', '\n'})
	}
	if meta.CSSInJS {
		buf.Write([]byte{'c', '\n'})
	}
	if meta.TypesOnly {
		buf.Write([]byte{'t', '\n'})
	}
	if meta.ExportDefault {
		buf.Write([]byte{'e', '\n'})
	}
	// [warning] values are line based, a value with "\n" (e.g. a crafted import specifier) would inject fields
	if meta.CSSEntry != "" && !strings.ContainsRune(meta.CSSEntry, '\n') {
		buf.Write([]byte{'.', ':'})
		buf.WriteString(meta.CSSEntry)
		buf.WriteByte('\n')
	}
	if meta.Dts != "" && !strings.ContainsRune(meta.Dts, '\n') {
		buf.Write([]byte{'d', ':'})
		buf.WriteString(meta.Dts)
		buf.WriteByte('\n')
	}
	for _, path := range meta.Imports {
		if !strings.ContainsRune(path, '\n') {
			buf.Write([]byte{'i', ':'})
			buf.WriteString(path)
			buf.WriteByte('\n')
		}
	}
	if len(meta.Integrity) > 0 {
		buf.Write([]byte{'s', ':'})
		buf.WriteString(meta.Integrity)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func decodeBuildMeta(data []byte) (*BuildMeta, error) {
	meta := &BuildMeta{}
	if len(data) < 5 || !bytes.Equal(data[:5], []byte{'E', 'S', 'M', '\r', '\n'}) {
		return nil, errors.New("invalid build meta")
	}
	for line := range bytes.SplitSeq(data[5:], []byte{'\n'}) {
		switch len(line) {
		case 0:
			// ignore empty line
		case 1:
			switch line[0] {
			case 'j':
				meta.CJS = true
			case 'c':
				meta.CSSInJS = true
			case 't':
				meta.TypesOnly = true
			case 'e':
				meta.ExportDefault = true
			}
		default:
			if line[1] == ':' {
				value := string(line[2:])
				switch line[0] {
				case '.':
					meta.CSSEntry = value
				case 'd':
					meta.Dts = value
				case 'i':
					meta.Imports = append(meta.Imports, value)
				case 's':
					meta.Integrity = value
				}
			}
		}
	}
	return meta, nil
}

var (
	metaBucket         = []byte("meta")
	purgedBucket       = []byte("purged")
	legacyPurgedBucket = []byte("legacy-purged")
)

type BuildMetaDB struct {
	db      *bolt.DB
	cache   *lru.Cache[string, []byte]
	storage storage.Storage
	locks   keyedMutex
	writes  sync.WaitGroup
}

func NewBuildMetaDB(filename string, backStorage storage.Storage) (*BuildMetaDB, error) {
	if err := ensureDir(filepath.Dir(filename)); err != nil {
		return nil, err
	}
	db, err := bolt.Open(filename, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{metaBucket, purgedBucket, legacyPurgedBucket} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	cache, _ := lru.New[string, []byte](lruCacheCapacity)
	return &BuildMetaDB{db: db, cache: cache, storage: backStorage}, nil
}

func (db *BuildMetaDB) Close() error {
	db.writes.Wait()
	return db.db.Close()
}

func (db *BuildMetaDB) lockPackage(key string, exclusive bool) func() {
	return db.locks.lock(key, exclusive)
}

func (db *BuildMetaDB) Get(key string) ([]byte, error) {
	storeKey := normalizeMetaStoreKey(key)
	pkgKey := metaStoreLockKey(storeKey)
	unlockPackage := db.lockPackage(pkgKey, false)
	defer unlockPackage()
	unlock := cacheMutex.Lock(storeKey)
	defer unlock()
	value, found := db.cache.Get(key)
	if !found {
		err := db.db.View(func(tx *bolt.Tx) error {
			// [warning] use a cursor, a miss is saved as an empty value and `Get` may return nil for it
			k, v := tx.Bucket(metaBucket).Cursor().Seek([]byte(storeKey))
			if found = string(k) == storeKey; found {
				value = bytes.Clone(v)
			} else {
				found = tx.Bucket(purgedBucket).Get([]byte(pkgKey)) != nil
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if !found && db.storage != nil {
			if value, err = db.importMeta(storeKey); err != nil {
				return nil, err
			}
			// [warning] persist misses too, otherwise every restart repeats the S3 lookup.
			// write in background, never overwrite a newer value or restore a purged package.
			db.writes.Go(func() {
				db.db.Batch(func(tx *bolt.Tx) error {
					b := tx.Bucket(metaBucket)
					if k, _ := b.Cursor().Seek([]byte(storeKey)); string(k) == storeKey || tx.Bucket(purgedBucket).Get([]byte(pkgKey)) != nil {
						return nil
					}
					return b.Put([]byte(storeKey), value)
				})
			})
		}
		db.cache.Add(key, value)
	}
	if len(value) == 0 {
		return nil, storage.ErrNotFound
	}
	return value, nil
}

// importMeta reads the metadata saved in the storage by older versions.
func (db *BuildMetaDB) importMeta(storeKey string) ([]byte, error) {
	r, _, err := db.storage.Get(storeKey)
	legacyKey := "meta/" + path.Base(storeKey)
	legacy := err == storage.ErrNotFound && storeKey != legacyKey
	if legacy {
		r, _, err = db.storage.Get(legacyKey)
	}
	if err == storage.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value, err := io.ReadAll(r)
	r.Close()
	if err != nil || !legacy {
		return value, err
	}
	marker := []byte("meta-purged/" + strings.TrimPrefix(path.Dir(storeKey), "meta/"))
	unlock := cacheMutex.Lock(string(marker))
	defer unlock()
	var purged []byte
	err = db.db.View(func(tx *bolt.Tx) error {
		purged = bytes.Clone(tx.Bucket(legacyPurgedBucket).Get(marker))
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(purged) == 0 {
		_, err = db.storage.Stat(string(marker))
		if err != nil && err != storage.ErrNotFound {
			return nil, err
		}
		purged = []byte{0}
		if err == nil {
			purged[0] = 1
		}
		err = db.db.Batch(func(tx *bolt.Tx) error {
			return tx.Bucket(legacyPurgedBucket).Put(marker, purged)
		})
		if err != nil {
			return nil, err
		}
	}
	if purged[0] == 1 {
		return nil, nil
	}
	return value, nil
}

func (db *BuildMetaDB) Put(key string, value []byte) error {
	return db.put(key, value)
}

func (db *BuildMetaDB) Delete(key string) error {
	// [warning] save an empty value as a tombstone, so old storage can not restore deleted metadata
	return db.put(key, []byte{})
}

func (db *BuildMetaDB) put(key string, value []byte) error {
	storeKey := normalizeMetaStoreKey(key)
	unlockPackage := db.lockPackage(metaStoreLockKey(storeKey), false)
	defer unlockPackage()
	unlock := cacheMutex.Lock(storeKey)
	defer unlock()
	err := db.db.Batch(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put([]byte(storeKey), value)
	})
	if err == nil {
		db.cache.Add(key, bytes.Clone(value))
	}
	return err
}

// Purge deletes all metadata of the package and blocks importing it from the storage.
// The caller must hold the exclusive package lock.
func (db *BuildMetaDB) Purge(pkgId string) (keys []string, err error) {
	pkgKey := "meta/" + pkgId
	err = db.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(purgedBucket).Put([]byte(pkgKey), []byte{1}); err != nil {
			return err
		}
		b, prefix := tx.Bucket(metaBucket), []byte(pkgKey+"/")
		c := b.Cursor()
		for k, _ := c.Seek(prefix); bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			keys = append(keys, string(k))
		}
		// [warning] do not delete while iterating, the cursor may skip keys
		for _, k := range keys {
			if err := b.Delete([]byte(k)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, key := range db.cache.Keys() {
		if strings.HasPrefix(key, "/"+pkgId+"/") || strings.HasPrefix(key, "/*"+pkgId+"/") {
			db.cache.Remove(key)
		}
	}
	return
}

func normalizeMetaStoreKey(key string) string {
	data := sha256.Sum256([]byte(key))
	prefix := "meta/"
	for segment := range strings.SplitSeq(strings.TrimPrefix(strings.TrimPrefix(key, "/"), "*"), "/") {
		prefix += segment + "/"
		if strings.Contains(strings.TrimPrefix(segment, "@"), "@") {
			return prefix + hex.EncodeToString(data[:])
		}
	}
	return "meta/" + hex.EncodeToString(data[:])
}

func metaStoreLockKey(key string) string {
	if prefix := path.Dir(key); prefix != "meta" {
		return prefix
	}
	return key
}
