package storage

import (
	"fmt"
	"net/url"
	"sync"

	"github.com/ije/gox/utils"
)

type Queue interface {
	Push(data []byte) error
	Pull() ([]byte, error)
}

var queueDrivers sync.Map

// New returns a new queue by url
func OpenQueue(url string) (queue Queue, err error) {
	if url == "" {
		err = fmt.Errorf("invalid url")
		return
	}

	name, addr := utils.SplitByFirstByte(url, ':')
	driver, ok := queueDrivers.Load(name)
	if !ok {
		err = fmt.Errorf("Unknown driver '%s'", name)
		return
	}

	path, options, err := parseConfigUrl(addr)
	if err != nil {
		return
	}

	queue, err = driver.(QueueDriver).Open(path, options)
	return
}

type QueueDriver interface {
	Open(addr string, args url.Values) (queue Queue, err error)
}

func RegisterQueue(name string, driver QueueDriver) error {
	_, ok := queueDrivers.Load(name)
	if ok {
		return fmt.Errorf("queue driver '%s' has been registered", name)
	}

	queueDrivers.Store(name, driver)
	return nil
}
