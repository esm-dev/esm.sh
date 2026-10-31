package server

import (
	"context"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ije/gox/sync"
	"golang.org/x/sync/semaphore"
)

var onceMap = sync.Map{}

// doOnce executes a function only once for a given id.
func doOnce(id string, fn func() error) (err error) {
	once, _ := onceMap.LoadOrStore(id, &sync.Once{})
	return once.(*sync.Once).Do(fn)
}

var (
	cacheMutex keyedMutex
	cacheStore sync.Map
	cacheLRU   *lru.Cache[string, any]
)

// keyedMutex holds a reference counted read/write lock per key.
// [warning] the count must change under the same lock as the map, otherwise two holders can get different mutexes for one key
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	gate *semaphore.Weighted
	refs int
}

const keyedWriteWeight int64 = 1 << 30

func (m *keyedMutex) lock(key string, exclusive bool) func() {
	unlock, _ := m.lockContext(context.Background(), key, exclusive)
	return unlock
}

func (m *keyedMutex) Lock(key string) func() {
	return m.lock(key, true)
}

func (m *keyedMutex) LockContext(ctx context.Context, key string) (func(), error) {
	return m.lockContext(ctx, key, true)
}

func (m *keyedMutex) lockContext(ctx context.Context, key string, exclusive bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	l := m.locks[key]
	if l == nil {
		if m.locks == nil {
			m.locks = map[string]*keyedLock{}
		}
		l = &keyedLock{gate: semaphore.NewWeighted(keyedWriteWeight)}
		m.locks[key] = l
	}
	l.refs++
	m.mu.Unlock()
	release := func() {
		m.mu.Lock()
		if l.refs--; l.refs == 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}
	weight := int64(1)
	if exclusive {
		weight = keyedWriteWeight
	}
	if err := l.gate.Acquire(ctx, weight); err != nil {
		release()
		return nil, err
	}
	return func() { l.gate.Release(weight); release() }, nil
}

type cacheItem struct {
	exp  int64
	data any
}

func getCacheItem(key string) (item any, ok bool) {
	if v, ok := cacheStore.Load(key); ok {
		item := v.(*cacheItem)
		if item.exp >= time.Now().UnixMilli() {
			return item.data, true
		}
	}
	return nil, false
}

func setCacheItem(key string, data any, cacheTtl time.Duration) {
	exp := time.Now().Add(cacheTtl)
	cacheStore.Store(key, &cacheItem{exp.UnixMilli(), data})
}

func deleteCacheItem(key string) {
	cacheStore.Delete(key)
}

// deleteCacheItemsWithPrefix removes every cached item whose key starts with
// the given prefix and returns the removed keys.
func deleteCacheItemsWithPrefix(prefix string) (deletedKeys []string) {
	cacheStore.Range(func(key, value any) bool {
		if k := key.(string); strings.HasPrefix(k, prefix) && cacheStore.CompareAndDelete(k, value) {
			deletedKeys = append(deletedKeys, k)
		}
		return true
	})
	return
}

func withCache[T any](key string, cacheTtl time.Duration, fetch func() (data T, aliasKey string, err error)) (data T, err error) {
	return withCacheContext(context.Background(), key, cacheTtl, fetch)
}

func withCacheContext[T any](ctx context.Context, key string, cacheTtl time.Duration, fetch func() (data T, aliasKey string, err error)) (data T, err error) {
	if err = ctx.Err(); err != nil {
		return
	}
	if cacheTtl == 0 {
		data, _, err = fetch()
		return
	}

	// check cache store first
	if v, ok := cacheStore.Load(key); ok {
		item := v.(*cacheItem)
		if item.exp >= time.Now().UnixMilli() {
			return item.data.(T), nil
		}
	}

	unlock, err := cacheMutex.LockContext(ctx, key)
	if err != nil {
		return data, err
	}
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

	exp := time.Now().Add(cacheTtl)
	cacheStore.Store(key, &cacheItem{exp.UnixMilli(), data})
	if aliasKey != "" && aliasKey != key {
		cacheStore.Store(aliasKey, &cacheItem{exp.UnixMilli(), data})
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
	negativeCache.gc(now)
	expires := now.UnixMilli()
	cacheStore.Range(func(key, value any) bool {
		item := value.(*cacheItem)
		if item.exp > 0 && item.exp < expires {
			cacheStore.CompareAndDelete(key, value)
		}
		return true
	})
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
