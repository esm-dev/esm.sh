package storage

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
)

type localFSDriver struct{}

func (driver *localFSDriver) Open(root string, options url.Values) (Storage, error) {
	root = filepath.Clean(root)
	err := ensureDir(root)
	if err != nil {
		return nil, err
	}
	return &localFS{root}, nil
}

type localFS struct {
	root string
}

func (fs *localFS) Stat(name string) (Stat, error) {
	fullPath := filepath.Join(fs.root, filepath.Clean(name))
	fi, err := os.Lstat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return fi, nil
}

func (fs *localFS) List(dir string) (files []string, err error) {
	return findFiles(filepath.Join(fs.root, filepath.Clean(dir)), "")
}

func (fs *localFS) Get(name string) (content io.ReadSeekCloser, err error) {
	fullPath := filepath.Join(fs.root, filepath.Clean(name))
	content, err = os.Open(fullPath)
	if err != nil && os.IsNotExist(err) {
		err = ErrNotFound
	}
	return
}

func (fs *localFS) Put(name string, content io.Reader) (written int64, err error) {
	fullPath := filepath.Join(fs.root, filepath.Clean(name))
	err = ensureDir(filepath.Dir(fullPath))
	if err != nil {
		return
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return
	}
	defer file.Close()

	written, err = io.Copy(file, content)
	return
}

func (fs *localFS) Remove(name string) (err error) {
	err = os.Remove(filepath.Join(fs.root, filepath.Clean(name)))
	return
}

func (fs *localFS) RemoveAll(dirname string) (err error) {
	err = os.RemoveAll(filepath.Join(fs.root, filepath.Clean(dirname)))
	return
}

// ensureDir ensures the given directory exists.
func ensureDir(dir string) (err error) {
	_, err = os.Lstat(dir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
	}
	return
}

// findFiles returns a list of files in the given directory.
func findFiles(root string, parentDir string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		path := name
		if parentDir != "" {
			path = parentDir + "/" + name
		}
		if entry.IsDir() {
			subFiles, err := findFiles(filepath.Join(root, name), path)
			if err != nil {
				return nil, err
			}
			newFiles := make([]string, len(files)+len(subFiles))
			copy(newFiles, files)
			copy(newFiles[len(files):], subFiles)
			files = newFiles
		} else {
			files = append(files, path)
		}
	}
	return files, nil
}

func init() {
	Register("fs", &localFSDriver{})
}
