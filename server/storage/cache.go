package storage

import (
	"fmt"
	"net/url"
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

var drivers = map[string]CacheDriver{}

// New returns a new cache by url
func OpenCache(url string) (cache Cache, err error) {
	if url == "" {
		err = fmt.Errorf("invalid url")
		return
	}

	name, addr := utils.SplitByFirstByte(url, ':')
	driver, ok := drivers[name]
	if !ok {
		err = fmt.Errorf("Unknown driver '%s'", name)
		return
	}
	path, options, err := parseConfigUrl(addr)
	if err != nil {
		return
	}

	cache, err = driver.Open(path, options)
	return
}

type CacheDriver interface {
	Open(addr string, args url.Values) (cache Cache, err error)
}

func RegisterCache(name string, driver CacheDriver) {
	if driver == nil {
		panic("cache: Register driver is nil")
	}
	if _, dup := drivers[name]; dup {
		panic("cache: Register called twice for driver " + name)
	}
	drivers[name] = driver
}
