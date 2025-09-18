package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ije/gox/utils"
)

// NewFSStorage creates a new storage instance that stores files on the local filesystem.
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

// safeJoinPath joins and validates that the resulting path is within fs.root
func (fs *fsStorage) safeJoinPath(key string) (string, error) {
	filename := filepath.Join(fs.root, key)
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	// Ensure absPath is within fs.root
	if !strings.HasPrefix(absPath, fs.root+string(os.PathSeparator)) && absPath != fs.root {
		return "", errors.New("invalid file path")
	}
	return absPath, nil
}

func (fs *fsStorage) Stat(key string) (stat Stat, err error) {
	filename, err := fs.safeJoinPath(key)
	if err != nil {
		return nil, ErrNotFound
	}
	fi, err := os.Lstat(filename)
	if err != nil {
		if os.IsNotExist(err) || strings.HasSuffix(err.Error(), "not a directory") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return fi, nil
}

func (fs *fsStorage) Get(key string) (content io.ReadCloser, stat Stat, err error) {
	filename, err := fs.safeJoinPath(key)
	if err != nil {
		err = ErrNotFound
		return
	}
	file, err := os.Open(filename)
	if err != nil && (os.IsNotExist(err) || strings.HasSuffix(err.Error(), "not a directory")) {
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

func (fs *fsStorage) List(prefix string) (keys []string, err error) {
	dir := strings.TrimSuffix(utils.NormalizePathname(prefix)[1:], "/")
	absDir, err := fs.safeJoinPath(dir)
	if err != nil {
		return nil, err
	}
	return findFiles(absDir, dir)
}

func (fs *fsStorage) Put(key string, content io.Reader) (err error) {
	filename, err := fs.safeJoinPath(key)
		return ErrNotFound
	}
	dir := filepath.Dir(filename)
	if !strings.HasPrefix(dir, fs.root+string(os.PathSeparator)) && dir != fs.root {
		return errors.New("invalid file path")
	}
	err = ensureDir(dir)
	if err != nil {
	if err != nil {
		return
	}

	file, err := os.Create(filename)
	if err != nil {
		return
	}

	_, err = io.Copy(file, content)
	file.Close()
	if err != nil {
		os.Remove(filename) // clean up if error occurs
	}
	return
}

func (fs *fsStorage) Delete(key string) (err error) {
	filename, err := fs.safeJoinPath(key)
	if err != nil {
		return ErrNotFound
	}
	return os.Remove(filename)
}

	absDir, err := fs.safeJoinPath(dir)
	if err != nil {
		return nil, ErrNotFound
	}
func (fs *fsStorage) DeleteAll(prefix string) (deletedKeys []string, err error) {
	dir := strings.TrimSuffix(utils.NormalizePathname(prefix)[1:], "/")
	if dir == "" {
		return nil, errors.New("prefix is required")
	}
	keys, err := fs.List(prefix)
	if err != nil {
		return
	}
	err = os.RemoveAll(absDir)
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
