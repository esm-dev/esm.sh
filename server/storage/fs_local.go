package storage

import (
	"io"
	"os"
	"path"
	"time"

	"github.com/ije/gox/utils"
)

type localFS struct{}

func (fs *localFS) Open(root string) (FSConn, error) {
	root = utils.CleanPath(root)
	err := ensureDir(root)
	if err != nil {
		return nil, err
	}
	return &localFSLayer{root: root}, nil
}

type localFSLayer struct {
	root string
}

func (fs *localFSLayer) Exists(name string) (bool, error) {
	fullPath := path.Join(fs.root, name)
	_, err := os.Stat(fullPath)
	if err != nil && os.IsNotExist(err) {
		return false, nil
	}
	return true, nil
}

func (fs *localFSLayer) ReadFile(name string) (file io.ReadSeekCloser, modtime time.Time, err error) {
	fullPath := path.Join(fs.root, name)
	fi, err := os.Stat(fullPath)
	if err != nil {
		return
	}

	modtime = fi.ModTime()
	file, err = os.Open(fullPath)
	return
}

func (fs *localFSLayer) WriteFile(name string, content io.Reader) (err error) {
	fullPath := path.Join(fs.root, name)
	err = ensureDir(path.Dir(fullPath))
	if err != nil {
		return
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return
	}

	_, err = io.Copy(file, content)
	return
}

func init() {
	RegisterFS("local", &localFS{})
}

func ensureDir(dir string) (err error) {
	_, err = os.Stat(dir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
	}
	return
}
