package storage

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExpired  = errors.New("record is expired")
	drivers     = sync.Map{}
)

type Storage interface {
	Stat(path string) (stat Stat, err error)
	List(prefix string) (files []string, err error)
	Get(path string) (content io.ReadSeekCloser, err error)
	Put(path string, r io.Reader) (written int64, err error)
	Remove(path string) error
	RemoveAll(dir string) error
}

type Stat interface {
	Size() int64
	ModTime() time.Time
}

type Driver interface {
	Open(root string, options url.Values) (conn Storage, err error)
}

func Open(storageUrl string) (Storage, error) {
	u, err := url.Parse(storageUrl)
	if err != nil {
		return nil, err
	}
	driver, ok := drivers.Load(u.Scheme)
	if ok {
		return driver.(Driver).Open(u.Path, u.Query())
	}
	return nil, fmt.Errorf("unregistered storage '%s'", u.Scheme)
}

func Register(name string, driver Driver) error {
	_, ok := drivers.Load(name)
	if ok {
		return fmt.Errorf("fs driver '%s' has been registered", name)
	}

	drivers.Store(name, driver)
	return nil
}
