package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"github.com/esm-dev/esm.sh/internal/storage"
	lru "github.com/hashicorp/golang-lru/v2"
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
	if meta.CSSEntry != "" {
		buf.Write([]byte{'.', ':'})
		buf.WriteString(meta.CSSEntry)
		buf.WriteByte('\n')
	}
	if meta.Dts != "" {
		buf.Write([]byte{'d', ':'})
		buf.WriteString(meta.Dts)
		buf.WriteByte('\n')
	}
	if len(meta.Imports) > 0 {
		for _, path := range meta.Imports {
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
	cache   *lru.Cache[string, []byte]
	storage storage.Storage
}

func NewBuildMetaDB(backStorage storage.Storage) *BuildMetaDB {
	cache, err := lru.New[string, []byte](lruCacheCapacity)
	if err != nil {
		panic(err)
	}
	return &BuildMetaDB{cache: cache, storage: backStorage}
}

func (db *BuildMetaDB) Get(key string) (value []byte, err error) {
	storeKey := normalizeMetaStoreKey(key)
	unlock := cacheMutex.Lock(metaStoreLockKey(storeKey))
	defer unlock()
	var cached bool
	value, cached = db.cache.Get(key)
	if cached {
		return
	}
	r, _, err := db.storage.Get(storeKey)
	legacy := false
	legacyKey := "meta/" + path.Base(storeKey)
	if err == storage.ErrNotFound && storeKey != legacyKey {
		r, _, err = db.storage.Get(legacyKey)
		if err == nil {
			// Reuse old metadata until this package has been purged.
			_, markerErr := db.storage.Stat("meta-purged/" + strings.TrimPrefix(path.Dir(storeKey), "meta/"))
			if markerErr != storage.ErrNotFound {
				r.Close()
				if markerErr != nil {
					return nil, markerErr
				}
				return nil, storage.ErrNotFound
			}
			legacy = true
		}
	}
	if err != nil {
		return
	}
	defer r.Close()
	value, err = io.ReadAll(r)
	if err == nil && legacy {
		err = db.storage.Put(storeKey, bytes.NewReader(value))
	}
	if err == nil {
		db.cache.Add(key, value)
	}
	return
}

func (db *BuildMetaDB) Put(key string, value []byte) (err error) {
	storeKey := normalizeMetaStoreKey(key)
	unlock := cacheMutex.Lock(metaStoreLockKey(storeKey))
	defer unlock()
	err = db.storage.Put(storeKey, bytes.NewReader(value))
	if err == nil {
		db.cache.Add(key, value)
	}
	return
}

func (db *BuildMetaDB) Delete(key string) (err error) {
	storeKey := normalizeMetaStoreKey(key)
	unlock := cacheMutex.Lock(metaStoreLockKey(storeKey))
	defer unlock()
	for _, name := range []string{storeKey, "meta/" + path.Base(storeKey)} {
		if deleteErr := db.storage.Delete(name); deleteErr != nil && deleteErr != storage.ErrNotFound && !errors.Is(deleteErr, os.ErrNotExist) {
			err = errors.Join(err, deleteErr)
		}
	}
	db.cache.Remove(key)
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
