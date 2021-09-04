package storage

import (
	"fmt"
	"sync"
	"time"

	"github.com/ije/gox/utils"
)

type Store map[string]string

type DB interface {
	Open(config string) (conn DBConn, err error)
}

type DBConn interface {
	Get(id string) (store Store, modtime time.Time, err error)
	Update(id string, store Store) error // should update the `modtime` field to current time
	Delete(id string) error
	Close() error
}

var dbs = sync.Map{}

func OpenDB(dbUrl string) (DBConn, error) {
	name, config := utils.SplitByFirstByte(dbUrl, ':')
	db, ok := dbs.Load(name)
	if ok {
		return db.(DB).Open(config)
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
