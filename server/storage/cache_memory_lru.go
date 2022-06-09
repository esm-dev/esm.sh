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

func (i *mLRUCache) Has(key string) (bool, error) {
	i.cache.Wait()
	_, ok := i.cache.Get(key)
	return ok, nil
}

func (i *mLRUCache) Get(key string) ([]byte, error) {
	i.cache.Wait()
	item, itemFound := i.cache.Get(key)
	if itemFound {
		_, ttlFound := i.cache.GetTTL(key)
		if ttlFound {
			return item.([]byte), nil
		}
		i.cache.Del(key)
		return nil, ErrExpired
	}
	return nil, ErrNotFound
}

func (i *mLRUCache) Set(key string, value []byte, ttl time.Duration) error {
	i.cache.Wait()
	i.cache.SetWithTTL(key, value, 0, ttl)
	return nil
}

func (i *mLRUCache) Delete(key string) error {
	i.cache.Wait()
	i.cache.Del(key)
	return nil
}

func (i *mLRUCache) Flush() error {
	i.cache.Wait()
	i.cache.Clear()
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
