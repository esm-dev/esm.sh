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
func NewFSStorage(options *StorageOptions) (Storage, error) {
	if options.Endpoint == "" {
		return nil, errors.New("endpoint is required")
	}
	root, err := filepath.Abs(options.Endpoint)
	if err != nil {
		return nil, err
	}
	if err := ensureDir(root); err != nil {
		return nil, err
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

func (fs *fsStorage) Stat(key string) (Stat, error) {
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

func (fs *fsStorage) Get(key string) (io.ReadCloser, Stat, error) {
	filename, err := fs.safeJoinPath(key)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) || strings.HasSuffix(err.Error(), "not a directory") {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, stat, nil
}

func (fs *fsStorage) List(prefix string) ([]string, error) {
	dir := strings.TrimSuffix(utils.NormalizePathname(prefix)[1:], "/")
	absDir, err := fs.safeJoinPath(dir)
	if err != nil {
		return nil, err
	}
	return findFiles(absDir, dir)
}

func (fs *fsStorage) Put(key string, content io.Reader) error {
	filename, err := fs.safeJoinPath(key)
	if err != nil {
		return ErrNotFound
	}

	dir := filepath.Dir(filename)
	if !strings.HasPrefix(dir, fs.root+string(os.PathSeparator)) && dir != fs.root {
		return errors.New("invalid file path")
	}
	if err := ensureDir(dir); err != nil {
		return err
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := io.Copy(file, content); err != nil {
		os.Remove(filename) // clean up if error occurs
		return err
	}
	return nil
}

func (fs *fsStorage) Delete(key string) error {
	filename, err := fs.safeJoinPath(key)
	if err != nil {
		return ErrNotFound
	}
	return os.Remove(filename)
}

func (fs *fsStorage) DeleteAll(prefix string) ([]string, error) {
	dir := strings.TrimSuffix(utils.NormalizePathname(prefix)[1:], "/")
	if dir == "" {
		return nil, errors.New("prefix is required")
	}

	absDir, err := fs.safeJoinPath(dir)
	if err != nil {
		return nil, ErrNotFound
	}

	keys, err := fs.List(prefix)
	if err != nil {
		return nil, err
	}

	if err := os.RemoveAll(absDir); err != nil {
		return nil, err
	}
	return keys, nil
}

// ensureDir ensures the given directory exists.
func ensureDir(dir string) error {
	_, err := os.Lstat(dir)
	if err != nil && os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
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
			files = append(files, subFiles...)
		} else {
			files = append(files, path)
		}
	}
	return files, nil
}
