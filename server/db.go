package server

type Stat struct {
	Records int64
}

// deprecated
type Database interface {
	Get(key string) (value []byte, err error)
	Put(key string, value []byte) (err error)
	Delete(key string) (err error)
	Close() (err error)
}
