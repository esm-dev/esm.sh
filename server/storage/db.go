package storage

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/ije/gox/utils"
)

type Store map[string]string

type DB interface {
	Open(config string, options url.Values) (conn DBConn, err error)
}

type DBConn interface {
	Get(id string) (store Store, modtime time.Time, err error)
	Put(id string, store Store) error
	Delete(id string) error
	Close() error
}

var dbs = sync.Map{}

func OpenDB(url string) (DBConn, error) {
	name, addr := utils.SplitByFirstByte(url, ':')
	db, ok := dbs.Load(name)
	if ok {
		root, options, err := parseConfigUrl(addr)
		if err == nil {
			return db.(DB).Open(root, options)
		}
	}
	return nil, fmt.Errorf("unregistered db '%s'", name)
}

func RegisterDB(name string, db DB) error {
	_, ok := dbs.Load(name)
	if ok {
		return fmt.Errorf("db '%s' has been registered", name)
	}

	dbs.Store(name, db)
	return nil
}
