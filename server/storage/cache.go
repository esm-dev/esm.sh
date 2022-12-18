package storage

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/ije/gox/utils"
)

type Cache interface {
	Has(key string) (bool, error)
	Get(key string) ([]byte, error)
	Set(key string, value []byte, ttl time.Duration) error
	Delete(key string) error
	Flush() error
}

var cacheDrivers sync.Map

// New returns a new cache by url
func OpenCache(url string) (cache Cache, err error) {
	if url == "" {
		err = fmt.Errorf("invalid url")
		return
	}

	name, addr := utils.SplitByFirstByte(url, ':')
	driver, ok := cacheDrivers.Load(name)
	if !ok {
		err = fmt.Errorf("unknown driver '%s'", name)
		return
	}

	path, options, err := parseConfigUrl(addr)
	if err != nil {
		return
	}

	cache, err = driver.(CacheDriver).Open(path, options)
	return
}

type CacheDriver interface {
	Open(addr string, args url.Values) (cache Cache, err error)
}

func RegisterCache(name string, driver CacheDriver) error {
	_, ok := cacheDrivers.Load(name)
	if ok {
		return fmt.Errorf("cache driver '%s' has been registered", name)
	}

	cacheDrivers.Store(name, driver)
	return nil
}
