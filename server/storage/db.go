package storage

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/ije/gox/utils"
)

type Store map[string]string

type ListItem struct {
	Store   map[string]string `json:"store"`
	Modtime uint32            `json:"modtime"`
}

type DBDriver interface {
	Open(config string, options url.Values) (conn DB, err error)
}

type DB interface {
	Get(id string) (store Store, modtime time.Time, err error)
	Put(id string, category string, store Store) error
	List(category string) ([]ListItem, error)
	Delete(id string) error
	Close() error
}

var dbDrivers = sync.Map{}

func OpenDB(url string) (DB, error) {
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
