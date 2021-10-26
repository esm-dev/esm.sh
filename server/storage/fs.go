package storage

import (
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	"github.com/ije/gox/utils"
)

type FSDriver interface {
	Open(root string, options url.Values) (conn FS, err error)
}

type FS interface {
	Exists(path string) (found bool, modtime time.Time, err error)
	ReadFile(path string) (content io.ReadSeekCloser, err error)
	WriteFile(path string, r io.Reader) (written int64, err error)
	WriteData(path string, data []byte) error
}

var fsDrivers = sync.Map{}

func OpenFS(fsUrl string) (FS, error) {
	name, addr := utils.SplitByFirstByte(fsUrl, ':')
	fs, ok := fsDrivers.Load(name)
	if ok {
		root, options, err := parseConfigUrl(addr)
		if err == nil {
			return fs.(FSDriver).Open(root, options)
		}
	}
	return nil, fmt.Errorf("unregistered fs '%s'", name)
}

func RegisterFS(name string, driver FSDriver) error {
	_, ok := fsDrivers.Load(name)
	if ok {
		return fmt.Errorf("fs driver '%s' has been registered", name)
	}

	fsDrivers.Store(name, driver)
	return nil
}
