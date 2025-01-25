package server

import (
	"container/list"
	"strings"
	"sync"
	"time"

	syncx "github.com/ije/gox/sync"
)

var (
	cacheMutex syncx.KeyedMutex
	cacheStore sync.Map
	cacheLRU   *list.List
)

type cacheRecord struct {
	key   string
	value any
}

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
	cacheKey := "lru:" + key

	// check cache store first
	if v, ok := cacheStore.Load(cacheKey); ok {
		el := v.(*cacheItem).data.(*list.Element)
		cacheLRU.MoveToFront(el)
		return el.Value.(cacheRecord).value.(T), nil
	}

	unlock := cacheMutex.Lock(cacheKey)
	defer unlock()

	// check cache store again after get lock
	if v, ok := cacheStore.Load(cacheKey); ok {
		el := v.(*cacheItem).data.(*list.Element)
		return el.Value.(cacheRecord).value.(T), nil
	}

	data, err = fetch()
	if err != nil {
		return
	}

	el := cacheLRU.PushFront(cacheRecord{cacheKey, data})
	cacheStore.Store(cacheKey, &cacheItem{-1, el})

	// delete the oldest item if cache store is full
	if cacheLRU.Len() > 1000 {
		el := cacheLRU.Back()
		if el != nil {
			cacheLRU.Remove(el)
			cacheStore.Delete(el.Value.(cacheRecord).key)
		}
	}

	return
}

func gc(now time.Time) {
	expKeys := []string{}
	cacheStore.Range(func(key, value any) bool {
		if !strings.HasPrefix(key.(string), "lru:") {
			item := value.(*cacheItem)
			if item.exp > 0 && item.exp < now.UnixMilli() {
				expKeys = append(expKeys, key.(string))
			}
		}
		return true
	})
	for _, key := range expKeys {
		cacheStore.Delete(key)
	}
}

func init() {
	cacheLRU = list.New()
	// cache GC
	go func() {
		tick := time.NewTicker(10 * time.Minute)
		for {
			now := <-tick.C
			gc(now)
		}
	}()
}
