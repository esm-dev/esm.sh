package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ije/gox/utils"
)

func NewFSStorage(options *StorageOptions) (storage Storage, err error) {
	if options.Endpoint == "" {
		return nil, errors.New("endpoint is required")
	}
	root, err := filepath.Abs(options.Endpoint)
	if err != nil {
		return nil, err
	}
	err = ensureDir(root)
	if err != nil {
		return
	}
	return &fsStorage{root: root}, nil
}

type fsStorage struct {
	root string
}

func (fs *fsStorage) Stat(key string) (stat Stat, err error) {
	filename := filepath.Join(fs.root, key)
	fi, err := os.Lstat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return fi, nil
}

func (fs *fsStorage) List(prefix string) (keys []string, err error) {
	dir := strings.TrimSuffix(utils.CleanPath(prefix)[1:], "/")
	return findFiles(filepath.Join(fs.root, dir), dir)
}

func (fs *fsStorage) Get(key string) (content io.ReadCloser, stat Stat, err error) {
	filename := filepath.Join(fs.root, key)
	file, err := os.Open(filename)
	if err != nil && os.IsNotExist(err) {
		err = ErrNotFound
	}
	if err == nil {
		stat, err = file.Stat()
	}
	if err != nil {
		return
	}
	content = file
	return
}

func (fs *fsStorage) Put(key string, content io.Reader) (err error) {
	filename := filepath.Join(fs.root, key)
	err = ensureDir(filepath.Dir(filename))
	if err != nil {
		return
	}

	file, err := os.Create(filename)
	if err != nil {
		return
	}
	defer file.Close()

	_, err = io.Copy(file, content)
	return
}

func (fs *fsStorage) Delete(keys ...string) (err error) {
	for _, key := range keys {
		os.Remove(filepath.Join(fs.root, key))
	}
	return
}

func (fs *fsStorage) DeleteAll(prefix string) (deletedKeys []string, err error) {
	dir := strings.TrimSuffix(utils.CleanPath(prefix)[1:], "/")
	if dir == "" {
		return nil, errors.New("prefix is required")
	}
	keys, err := fs.List(prefix)
	if err != nil {
		return
	}
	err = os.RemoveAll(filepath.Join(fs.root, dir))
	if err != nil {
		return
	}
	return keys, nil
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
		if os.IsNotExist(err) {
			return []string{}, nil
		}
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
