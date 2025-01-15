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
	exp  time.Time
	data any
}

func withCache[T any](key string, cacheTtl time.Duration, fetch func() (T, string, error)) (data T, err error) {
	// check cache first
	if cacheTtl > time.Second {
		if v, ok := cacheStore.Load(key); ok {
			item := v.(*cacheItem)
			if item.exp.After(time.Now()) {
				return item.data.(T), nil
			}
		}
	}

	unlock := cacheMutex.Lock(key)
	defer unlock()

	// check cache again after lock
	if cacheTtl > time.Second {
		if v, ok := cacheStore.Load(key); ok {
			item := v.(*cacheItem)
			if item.exp.After(time.Now()) {
				return item.data.(T), nil
			}
		}
	}

	var aliasKey string
	data, aliasKey, err = fetch()
	if err != nil {
		return
	}

	if cacheTtl > time.Second {
		exp := time.Now().Add(cacheTtl)
		cacheStore.Store(key, &cacheItem{exp, data})
		if aliasKey != "" && aliasKey != key {
			cacheStore.Store(aliasKey, &cacheItem{exp, data})
		}
	}
	return
}

func withLRUCache[T any](key string, fetch func() (T, error)) (data T, err error) {
	cacheKey := "lru:" + key
	cacheTtl := 24 * time.Hour

	// check cache first
	if v, ok := cacheStore.Load(cacheKey); ok {
		item := v.(*cacheItem)
		item.exp = time.Now().Add(cacheTtl)
		el := item.data.(*list.Element)
		cacheLRU.MoveToBack(el)
		return el.Value.(cacheRecord).value.(T), nil
	}

	unlock := cacheMutex.Lock(cacheKey)
	defer unlock()

	// check cache again after lock
	if v, ok := cacheStore.Load(cacheKey); ok {
		item := v.(*cacheItem)
		el := item.data.(*list.Element)
		return el.Value.(cacheRecord).value.(T), nil
	}

	data, err = fetch()
	if err != nil {
		return
	}

	el := cacheLRU.PushBack(cacheRecord{cacheKey, data})
	cacheStore.Store(cacheKey, &cacheItem{time.Now().Add(cacheTtl), el})

	// delete the oldest item if cache is full
	if cacheLRU.Len() > 1000 {
		el := cacheLRU.Front()
		if el != nil {
			cacheLRU.Remove(el)
			cacheStore.Delete(el.Value.(cacheRecord).key)
		}
	}

	return
}

func init() {
	cacheLRU = list.New()
	// cache GC
	go func() {
		tick := time.NewTicker(time.Minute)
		for {
			now := <-tick.C
			expKeys := []string{}
			cacheStore.Range(func(key, value any) bool {
				item := value.(*cacheItem)
				if item.exp.Before(now) {
					expKeys = append(expKeys, key.(string))
				}
				return true
			})
			for _, key := range expKeys {
				if strings.HasPrefix(key, "lru:") {
					item, ok := cacheStore.LoadAndDelete(key)
					if ok {
						cacheLRU.Remove(item.(*cacheItem).data.(*list.Element))
					}
				} else {
					cacheStore.Delete(key)
				}
			}
		}
	}()
}
