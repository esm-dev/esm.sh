package storage

import (
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	"github.com/ije/gox/utils"
)

type FileSystemDriver interface {
	Open(root string, options url.Values) (conn FileSystem, err error)
}

type FileSystem interface {
	Exists(path string) (found bool, size int64, modtime time.Time, err error)
	ReadFile(path string, size int64) (content io.ReadSeekCloser, err error)
	WriteFile(path string, r io.Reader) (written int64, err error)
	WriteData(path string, data []byte) error
}

var fsDrivers = sync.Map{}

func OpenFS(fsUrl string) (FileSystem, error) {
	name, addr := utils.SplitByFirstByte(fsUrl, ':')
	fs, ok := fsDrivers.Load(name)
	if ok {
		root, options, err := parseConfigUrl(addr)
		if err == nil {
			return fs.(FileSystemDriver).Open(root, options)
		}
	}
	return nil, fmt.Errorf("unregistered fs '%s'", name)
}

func RegisterFileSystem(name string, driver FileSystemDriver) error {
	_, ok := fsDrivers.Load(name)
	if ok {
		return fmt.Errorf("fs driver '%s' has been registered", name)
	}

	fsDrivers.Store(name, driver)
	return nil
}
