package storage

import (
	"net/url"
)

type chanQueueDriver struct{}

func (fs *chanQueueDriver) Open(path string, options url.Values) (Queue, error) {
	return &chanQueue{make(chan []byte, 1000)}, nil
}

type chanQueue struct {
	c chan []byte
}

func (q *chanQueue) Push(data []byte) error {
	q.c <- data
	return nil
}

func (q *chanQueue) Pull() ([]byte, error) {
	return <-q.c, nil
}

func init() {
	RegisterQueue("chan", &chanQueueDriver{})
}
