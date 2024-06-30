package storage

import (
	"fmt"
	"net/url"
	"sync"

	"github.com/ije/gox/utils"
)

type DataBase interface {
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
	Delete(key string) error
	DeleteAll(prefix string) ([]string, error)
	Close() error
}

type DBDriver interface {
	Open(config string, options url.Values) (conn DataBase, err error)
}

var dbDrivers = sync.Map{}

func OpenDB(url string) (DataBase, error) {
	name, addr := utils.SplitByFirstByte(url, ':')
	db, ok := dbDrivers.Load(name)
	if ok {
		root, options, err := parseConfigUrl(addr)
		if err == nil {
			return db.(DBDriver).Open(root, options)
		}
	}
	return nil, fmt.Errorf("unregistered db '%s'", name)
}

func RegisterDB(name string, driver DBDriver) error {
	_, ok := dbDrivers.Load(name)
	if ok {
		return fmt.Errorf("db driver '%s' has been registered", name)
	}

	dbDrivers.Store(name, driver)
	return nil
}
