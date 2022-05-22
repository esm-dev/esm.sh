package storage

import (
	"net/url"
	"time"

	"github.com/ije/postdb"
	"github.com/ije/postdb/q"
)

type postDBDriver struct{}

func (driver *postDBDriver) Open(path string, options url.Values) (DB, error) {
	db, err := postdb.Open(path, 0644)
	if err != nil {
		return nil, err
	}
	return &postDB{db}, nil
}

type postDB struct {
	db *postdb.DB
}

func (i *postDB) Get(id string) (store Store, modtime time.Time, err error) {
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

func (i *postDB) Put(id string, category string, store Store) (err error) {
	kv := q.KV{}
	for key, value := range store {
		kv[key] = []byte(value)
	}
	_, err = i.db.Get(q.Alias(id))
	if err == nil {
		err = i.db.Update(q.Alias(id), kv)
	} else if err == postdb.ErrNotFound {
		_, err = i.db.Put(q.Alias(id), kv, q.Tags(category))
	}
	return
}

func (i *postDB) List(category string) (list []ListItem, err error) {
	posts, err := i.db.List(q.Tags(category), q.Select("*"))
	if err != nil {
		return
	}
	for _, post := range posts {
		store := Store{}
		for key, value := range post.KV {
			store[key] = string(value)
		}
		list = append(list, ListItem{Store: store, Modtime: post.Modtime})
	}
	return
}

func (i *postDB) Delete(id string) error {
	_, err := i.db.Delete(q.Alias(id))
	return err
}

func (i *postDB) Close() error {
	return i.db.Close()
}

func init() {
	RegisterDB("postdb", &postDBDriver{})
}
