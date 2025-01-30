package server

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	syncx "github.com/ije/gox/sync"
)

var (
	cacheMutex syncx.KeyedMutex
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

	unlock := cacheMutex.Lock("lru:" + key)
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

	unlock := cacheMutex.Lock(key)
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
