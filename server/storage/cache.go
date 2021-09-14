package storage

import (
	"fmt"
	"strings"
	"time"

	"github.com/ije/gox/utils"
)

type Cache interface {
	Has(key string) (bool, error)
	Get(key string) ([]byte, error)
	Set(key string, value []byte, ttl time.Duration) error
	Delete(key string) error
	Flush() error
	Notify(name string, args ...string) error
}

var drivers = map[string]CacheDriver{}

// New returns a new cache by url
func OpenCache(url string) (cache Cache, err error) {
	if url == "" {
		err = fmt.Errorf("invalid url")
		return
	}

	path, query := utils.SplitByFirstByte(url, '?')
	name, addr := utils.SplitByFirstByte(path, ':')
	driver, ok := drivers[strings.ToLower(name)]
	if !ok {
		err = fmt.Errorf("Unknown driver '%s'", name)
		return
	}

	args := map[string]string{}
	for _, q := range strings.Split(query, "&") {
		k, v := utils.SplitByFirstByte(q, '=')
		if len(k) > 0 {
			args[k] = v
		}
	}

	cache, err = driver.Open(addr, args)
	return
}

type CacheDriver interface {
	Open(addr string, args map[string]string) (cache Cache, err error)
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
