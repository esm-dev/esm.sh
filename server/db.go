package server

type Stat struct {
	Records int64
}

type Database interface {
	Stat() (stat Stat, err error)
	Get(key string) (value []byte, err error)
	Put(key string, value []byte) (err error)
	Delete(key string) (err error)
	Close() (err error)
}
