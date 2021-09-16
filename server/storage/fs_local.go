package storage

import (
	"io"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/ije/gox/utils"
)

type localFS struct{}

func (fs *localFS) Open(root string, options url.Values) (FS, error) {
	root = utils.CleanPath(root)
	err := ensureDir(root)
	if err != nil {
		return nil, err
	}
	return &localFSLayer{root}, nil
}

type localFSLayer struct {
	root string
}

func (fs *localFSLayer) Exists(name string) (bool, time.Time, error) {
	fullPath := path.Join(fs.root, name)
	fi, err := os.Stat(fullPath)
	if err != nil {
		var modtime time.Time
		if os.IsNotExist(err) {
			err = nil
		}
		return false, modtime, err
	}
	return true, fi.ModTime(), nil
}

func (fs *localFSLayer) ReadFile(name string) (file io.ReadSeekCloser, err error) {
	fullPath := path.Join(fs.root, name)
	return os.Open(fullPath)
}

func (fs *localFSLayer) WriteFile(name string, content io.Reader) (written int64, err error) {
	fullPath := path.Join(fs.root, name)
	err = ensureDir(path.Dir(fullPath))
	if err != nil {
		return
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return
	}

	written, err = io.Copy(file, content)
	if closeError := file.Close(); closeError != nil && err == nil {
		err = closeError
	}
	return
}

func (fs *localFSLayer) WriteData(name string, data []byte) error {
	fullPath := path.Join(fs.root, name)
	err := ensureDir(path.Dir(fullPath))
	if err != nil {
		return err
	}
	return os.WriteFile(fullPath, data, 0666)
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
