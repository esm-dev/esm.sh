package storage

import (
	"time"

	"github.com/dgraph-io/ristretto"
)

type mLRUCache struct {
	cache *ristretto.Cache
}

func (mc *mLRUCache) Has(key string) (bool, error) {
	_, ok := mc.cache.Get(key)
	return ok, nil
}

func (mc *mLRUCache) Get(key string) ([]byte, error) {
	item, ok := mc.cache.Get(key)
	if ok {
		return item.([]byte), nil
	}
	return nil, nil
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
	mc.cache.Wait()
	return nil
}

func (mc *mLRUCache) Flush() error {
	mc.cache.Clear()
	mc.cache.Wait()
	return nil
}

type mcLRUDriver struct{}

func (mcd *mcLRUDriver) Open(region string, args map[string]string) (cache Cache, err error) {
	impl, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     1, // no max cost, all costs are ZERO (for now)
		BufferItems: 64,
		Metrics:     isDev,
	})
	if err != nil {
		return nil, err
	}
	return &mLRUCache{cache: impl}, nil
}

func init() {
	RegisterCache("memoryLRU", &mcLRUDriver{})
}
