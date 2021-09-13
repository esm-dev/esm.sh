package storage

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ije/gox/utils"
)

type FS interface {
	Open(config string) (conn FSConn, err error)
}

type FSConn interface {
	Exists(path string) (found bool, modtime time.Time, err error)
	ReadFile(path string) (content io.ReadSeekCloser, err error)
	WriteFile(path string, r io.Reader) (written int64, err error)
	WriteData(path string, data []byte) error
}

var fss = sync.Map{}

func OpenFS(fsUrl string) (FSConn, error) {
	name, config := utils.SplitByFirstByte(fsUrl, ':')
	fs, ok := fss.Load(name)
	if ok {
		return fs.(FS).Open(config)
	}
	return nil, fmt.Errorf("unregistered fs '%s'", name)
}

func RegisterFS(name string, fs FS) error {
	_, ok := fss.Load(name)
	if ok {
		return fmt.Errorf("fs '%s' has been registered", name)
	}

	fss.Store(name, fs)
	return nil
}
