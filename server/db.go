package server

type Database interface {
	Get(key string) (value []byte, err error)
	Put(key string, value []byte) (err error)
	Delete(key string) error
	Close() error
}
