package server

import (
	"sync"
	"time"
)

var (
	cacheLocks sync.Map
	cacheStore sync.Map
)

type CacheItem struct {
	data any
	exp  time.Time
}

func withCache[T any](key string, cacheTtl time.Duration, fetch func() (T, error)) (r T, err error) {
	// check cache first
	if cacheTtl > 0 {
		if v, ok := cacheStore.Load(key); ok {
			item := v.(*CacheItem)
			if item.exp.After(time.Now()) {
				return item.data.(T), nil
			}
		}
	}

	lock, _ := cacheLocks.LoadOrStore(key, &sync.Mutex{})
	defer cacheLocks.Delete(key)

	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	// check cache again after lock
	if cacheTtl > 0 {
		if v, ok := cacheStore.Load(key); ok {
			item := v.(*CacheItem)
			if item.exp.After(time.Now()) {
				return item.data.(T), nil
			}
		}
	}

	r, err = fetch()
	if err != nil {
		return
	}

	if cacheTtl > 0 {
		cacheStore.Store(key, &CacheItem{
			data: r,
			exp:  time.Now().Add(cacheTtl),
		})
	}
	return
}

func init() {
	// cache GC
	go func() {
		tick := time.NewTicker(10 * time.Minute)
		for {
			now := <-tick.C
			expKeys := []any{}
			cacheStore.Range(func(key, value any) bool {
				item := value.(*CacheItem)
				if item.exp.Before(now) {
					expKeys = append(expKeys, key)
				}
				return true
			})
			for _, key := range expKeys {
				cacheStore.Delete(key)
			}
		}
	}()
}
