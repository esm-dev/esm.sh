package server

import (
	"bytes"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ije/gox/sync"
)

var bufferPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

// newBuffer returns a new buffer from the buffer pool.
func newBuffer() (buffer *bytes.Buffer, recycle func()) {
	buf := bufferPool.Get().(*bytes.Buffer)
	return buf, func() {
		buf.Reset()
		bufferPool.Put(buf)
	}
}

var onceMap = sync.Map{}

func doOnce(id string, fn func() error) (err error) {
	once, _ := onceMap.LoadOrStore(id, &sync.Once{})
	return once.(*sync.Once).Do(fn)
}

var (
	cacheMutex sync.KeyedMutex
	cacheStore sync.Map
	cacheLRU   *lru.Cache[string, any]
)

type cacheItem struct {
	exp  int64
	data any
}

func withCache[T any](key string, cacheTtl time.Duration, fetch func() (T, string, error)) (data T, err error) {
	// check cache store first
	if cacheTtl > time.Millisecond {
		if v, ok := cacheStore.Load(key); ok {
			item := v.(*cacheItem)
			if item.exp >= time.Now().UnixMilli() {
				return item.data.(T), nil
			}
		}
	}

	unlock := cacheMutex.Lock(key)
	defer unlock()

	// check cache store again after get lock
	if cacheTtl > time.Millisecond {
		if v, ok := cacheStore.Load(key); ok {
			item := v.(*cacheItem)
			if item.exp >= time.Now().UnixMilli() {
				return item.data.(T), nil
			}
		}
	}

	var aliasKey string
	data, aliasKey, err = fetch()
	if err != nil {
		return
	}

	if cacheTtl > time.Millisecond {
		exp := time.Now().Add(cacheTtl)
		cacheStore.Store(key, &cacheItem{exp.UnixMilli(), data})
		if aliasKey != "" && aliasKey != key {
			cacheStore.Store(aliasKey, &cacheItem{exp.UnixMilli(), data})
		}
	}
	return
}

func withLRUCache[T any](key string, fetch func() (T, error)) (data T, err error) {
	// check cache store first
	if v, ok := cacheLRU.Get(key); ok {
		return v.(T), nil
	}

	unlock := cacheMutex.Lock("lru:" + key)
	defer unlock()

	// check cache store again after get lock
	if v, ok := cacheLRU.Get(key); ok {
		return v.(T), nil
	}

	data, err = fetch()
	if err != nil {
		return
	}

	cacheLRU.Add(key, data)
	return
}

func gc(now time.Time) {
	expKeys := []string{}
	cacheStore.Range(func(key, value any) bool {
		item := value.(*cacheItem)
		if item.exp > 0 && item.exp < now.UnixMilli() {
			expKeys = append(expKeys, key.(string))
		}
		return true
	})
	for _, key := range expKeys {
		cacheStore.Delete(key)
	}
}

func init() {
	var err error
	cacheLRU, err = lru.New[string, any](lruCacheCapacity)
	if err != nil {
		panic(err)
	}
	// cache GC
	go func() {
		tick := time.NewTicker(10 * time.Minute)
		for {
			now := <-tick.C
			gc(now)
		}
	}()
}
