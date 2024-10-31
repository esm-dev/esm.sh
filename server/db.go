package server

import (
	"fmt"
	"net/url"
	"sync"
)

var dbDrivers sync.Map

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

func OpenDB(dbUrl string) (DataBase, error) {
	u, err := url.Parse(dbUrl)
	if err != nil {
		return nil, err
	}
	driver, ok := dbDrivers.Load(u.Scheme)
	if ok {
		return driver.(DBDriver).Open(u.Path, u.Query())
	}
	return nil, fmt.Errorf("unregistered db '%s'", u.Scheme)
}

func RegisterDBDriver(name string, driver DBDriver) error {
	if _, ok := dbDrivers.Load(name); ok {
		return fmt.Errorf("db driver '%s' has been registered", name)
	}
	dbDrivers.Store(name, driver)
	return nil
}
