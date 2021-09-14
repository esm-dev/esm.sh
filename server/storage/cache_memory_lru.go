package storage

import (
	"errors"
	"net/url"
	"time"

	"github.com/dgraph-io/ristretto"
)

type mLRUCache struct {
	cache *ristretto.Cache
}

func (mc *mLRUCache) Has(key string) bool {
	_, ok := mc.cache.Get(key)
	return ok
}

func (mc *mLRUCache) Get(key string) ([]byte, error) {
	item, itemFound := mc.cache.Get(key)
	if itemFound {
		_, ttlFound := mc.cache.GetTTL(key)
		if ttlFound {
			return item.([]byte), nil
		}
		mc.cache.Del(key)
		return nil, ErrExpired
	}
	return nil, ErrNotFound
}

func (mc *mLRUCache) Set(key string, value []byte, ttl time.Duration) error {
	ok := mc.cache.SetWithTTL(key, value, 0, ttl)
	if ok {
		mc.cache.Wait()
	}
	return nil
}

func (mc *mLRUCache) Delete(key string) error {
	mc.cache.Del(key)
	return nil
}

func (mc *mLRUCache) Flush() error {
	mc.cache.Clear()
	return nil
}

type mcLRUDriver struct{}

func (mcd *mcLRUDriver) Open(region string, options url.Values) (cache Cache, err error) {
	maxCost, err := parseBytesValue(options.Get("maxCost"), 1<<30) // Default maximum cost of cache is 1GB
	if err != nil {
		return nil, errors.New("invalid maxCost value")
	}
	impl, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     maxCost,
		BufferItems: 64,
		Metrics:     isDev,
		/**
		 * Determine cost automatically when cost is zero when set.
		 * This is skipped entirely if the cost is not zero when set.
		 */
		Cost: func(value interface{}) int64 {
			return int64(len(value.([]byte)))
		},
	})
	if err != nil {
		return nil, err
	}
	return &mLRUCache{cache: impl}, nil
}

func init() {
	RegisterCache("memoryLRU", &mcLRUDriver{})
}
