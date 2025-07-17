package storage

import (
	"errors"
	"io"
)

// NewMigrationStorage creates a new storage instance that migrates files from one storage to another.
// It first tries to get files from the front storage, and if not found, it retrieves them from the back storage.
func NewMigrationStorage(frontStorage Storage, backStorage Storage) (storage Storage) {
	return &migrationStorage{
		front: frontStorage,
		back:  backStorage,
	}
}

type migrationStorage struct {
	front Storage
	back  Storage
}

func (m *migrationStorage) Stat(key string) (stat Stat, err error) {
	stat, err = m.front.Stat(key)
	if err == ErrNotFound {
		stat, err = m.back.Stat(key)
	}
	return
}

func (m *migrationStorage) Get(key string) (content io.ReadCloser, stat Stat, err error) {
	content, stat, err = m.front.Get(key)
	if err == ErrNotFound {
		content, stat, err = m.back.Get(key)
		if err == nil {
			pr, pw := io.Pipe()
			go func(content io.ReadCloser) {
				defer pw.Close()
				defer content.Close()
				m.front.Put(key, io.TeeReader(content, pw))
			}(content)
			content = pr
		}
	}
	return
}

func (m *migrationStorage) List(prefix string) (keys []string, err error) {
	return nil, errors.New("List operation is not supported in migration storage")
}

func (m *migrationStorage) Put(key string, content io.Reader) (err error) {
	return m.front.Put(key, content)
}

func (m *migrationStorage) Delete(key string) (err error) {
	m.back.Delete(key)
	return m.front.Delete(key)
}

func (m *migrationStorage) DeleteAll(prefix string) (deletedKeys []string, err error) {
	m.back.DeleteAll(prefix)
	return m.front.DeleteAll(prefix)
}
