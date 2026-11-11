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

type BuildMetaDB struct {
	*bolt.DB
	cache   *lru.Cache[string, []byte]
	storage storage.Storage
	locks   keyedMutex
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
		for _, name := range []string{"meta", "purged", "legacy-purged"} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
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
	return &BuildMetaDB{DB: db, cache: cache, storage: backStorage}, nil
}

func (db *BuildMetaDB) lockPackage(key string, exclusive bool) func() {
	return db.locks.lock(key, exclusive)
}

func (db *BuildMetaDB) Get(key string) (value []byte, err error) {
	storeKey := normalizeMetaStoreKey(key)
	unlockPackage := db.lockPackage(metaStoreLockKey(storeKey), false)
	defer unlockPackage()
	unlock := cacheMutex.Lock(storeKey)
	defer unlock()
	var cached bool
	value, cached = db.cache.Get(key)
	if cached {
		if len(value) == 0 {
			err = storage.ErrNotFound
		}
		return
	}
	err = db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket([]byte("meta")).Get([]byte(storeKey))
		cached = v != nil || tx.Bucket([]byte("purged")).Get([]byte(metaStoreLockKey(storeKey))) != nil
		value = bytes.Clone(v)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !cached {
		if db.storage != nil {
			r, _, readErr := db.storage.Get(storeKey)
			legacyKey := "meta/" + path.Base(storeKey)
			legacy := readErr == storage.ErrNotFound && storeKey != legacyKey
			if legacy {
				r, _, readErr = db.storage.Get(legacyKey)
			}
			if readErr == nil {
				value, readErr = io.ReadAll(r)
				r.Close()
				if readErr != nil {
					return nil, readErr
				}
				if legacy {
					marker := "meta-purged/" + strings.TrimPrefix(path.Dir(storeKey), "meta/")
					unlockMarker := cacheMutex.Lock(marker)
					var checked, purged bool
					err = db.View(func(tx *bolt.Tx) error {
						v := tx.Bucket([]byte("legacy-purged")).Get([]byte(marker))
						checked, purged = len(v) != 0, len(v) != 0 && v[0] == 1
						return nil
					})
					if err == nil && !checked {
						_, err = db.storage.Stat(marker)
						purged = err == nil
						if err == nil || err == storage.ErrNotFound {
							v := []byte{0}
							if purged {
								v[0] = 1
							}
							err = db.Batch(func(tx *bolt.Tx) error {
								return tx.Bucket([]byte("legacy-purged")).Put([]byte(marker), v)
							})
						}
					}
					unlockMarker()
					if err != nil {
						return nil, err
					}
					if purged {
						value = nil
					}
				}
			} else if readErr != storage.ErrNotFound {
				return nil, readErr
			}
		}
		// [warning] persist misses too, otherwise every restart repeats the S3 lookup
		err = db.Batch(func(tx *bolt.Tx) error {
			return tx.Bucket([]byte("meta")).Put([]byte(storeKey), value)
		})
		if err != nil {
			return nil, err
		}
	}
	db.cache.Add(key, value)
	if len(value) == 0 {
		err = storage.ErrNotFound
	}
	return
}

func (db *BuildMetaDB) Put(key string, value []byte) (err error) {
	storeKey := normalizeMetaStoreKey(key)
	unlockPackage := db.lockPackage(metaStoreLockKey(storeKey), false)
	defer unlockPackage()
	unlock := cacheMutex.Lock(storeKey)
	defer unlock()
	err = db.Batch(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("meta")).Put([]byte(storeKey), value)
	})
	if err == nil {
		db.cache.Add(key, bytes.Clone(value))
	}
	return
}

func (db *BuildMetaDB) Delete(key string) (err error) {
	storeKey := normalizeMetaStoreKey(key)
	unlockPackage := db.lockPackage(metaStoreLockKey(storeKey), false)
	defer unlockPackage()
	unlock := cacheMutex.Lock(storeKey)
	defer unlock()
	// [warning] retain a tombstone so old storage cannot restore deleted metadata
	err = db.Batch(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("meta")).Put([]byte(storeKey), nil)
	})
	if err == nil {
		db.cache.Remove(key)
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
