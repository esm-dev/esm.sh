package storage

import (
	"net/url"
	"time"

	"github.com/postui/postdb"
	"github.com/postui/postdb/q"
)

type postDB struct{}

func (fs *postDB) Open(path string, options url.Values) (DBConn, error) {
	db, err := postdb.Open(path, 0644)
	if err != nil {
		return nil, err
	}
	return &postDBInstance{db}, nil
}

type postDBInstance struct {
	db *postdb.DB
}

func (i *postDBInstance) Get(id string) (store Store, modtime time.Time, err error) {
	post, err := i.db.Get(q.Alias(id), q.Select("*"))
	if err != nil {
		if err == postdb.ErrNotFound {
			err = ErrNotFound
		}
		return
	}

	store = Store{}
	for key, value := range post.KV {
		store[key] = string(value)
	}
	modtime = time.Unix(int64(post.Modtime), 0)
	return
}

func (i *postDBInstance) Put(id string, store Store) (err error) {
	_, err = i.db.Get(q.Alias(id))
	if err == nil {
		kv := q.KV{}
		for key, value := range store {
			kv[key] = []byte(value)
		}
		err = i.db.Update(q.Alias(id), kv)
	} else if err == postdb.ErrNotFound {
		kv := q.KV{}
		for key, value := range store {
			kv[key] = []byte(value)
		}
		_, err = i.db.Put(q.Alias(id), kv)
	}
	return
}

func (i *postDBInstance) Delete(id string) error {
	_, err := i.db.Delete(q.Alias(id))
	return err
}

func (i *postDBInstance) Close() error {
	return i.db.Close()
}

func init() {
	RegisterDB("postdb", &postDB{})
}
