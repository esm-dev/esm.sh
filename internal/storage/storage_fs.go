package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ije/gox/utils"
)

type fsStorage struct {
	root string
}

// NewFSStorage creates a new storage instance that stores files on the local filesystem.
func NewFSStorage(root string) (storage Storage, err error) {
	if root == "" {
		return nil, errors.New("root is required")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	err = ensureDir(root)
	if err != nil {
		return
	}
	return &fsStorage{root: root}, nil
}

// joinRootSafe returns filepath.Join(fs.root, key) only when key is
// filepath.IsLocal, so lexical ".." segments cannot escape the storage root.
func (fs *fsStorage) joinRootSafe(key string) (filename string, err error) {
	if key == "" {
		return "", errors.New("key is required")
	}
	if strings.Contains(key, "\x00") {
		return "", ErrInvalidStorageKey
	}
	k := filepath.FromSlash(strings.Trim(filepath.ToSlash(key), "/"))
	if !filepath.IsLocal(k) {
		return "", ErrInvalidStorageKey
	}
	root := filepath.Clean(fs.root)
	full := filepath.Join(root, k)
	return full, nil
}

func (fs *fsStorage) Stat(key string) (stat Stat, err error) {
	filename, err := fs.joinRootSafe(key)
	if err != nil {
		return nil, err
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
	filename, err := fs.joinRootSafe(key)
	if err != nil {
		return
	}
	file, err := os.Open(filename)
	if err != nil && (os.IsNotExist(err) || strings.HasSuffix(err.Error(), "not a directory")) {
		err = ErrNotFound
	}
	if err != nil {
		return
	}
	content = file
	stat, err = os.Stat(filename)
	return
}

func (fs *fsStorage) Put(key string, content io.Reader) (err error) {
	filename, err := fs.joinRootSafe(key)
	if err != nil {
		return err
	}
	err = ensureDir(filepath.Dir(filename))
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
	filename, err := fs.joinRootSafe(key)
	if err != nil {
		return err
	}
	return os.Remove(filename)
}

func (fs *fsStorage) List(prefix string) (keys []string, err error) {
	dir := strings.TrimSuffix(utils.NormalizePathname(prefix)[1:], "/")
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	scanRoot := filepath.Clean(fs.root)
	parentKey := ""
	if dir != "" {
		var ferr error
		scanRoot, ferr = fs.joinRootSafe(dir)
		if ferr != nil {
			return nil, ferr
		}
		parentKey = dir
	}
	return findFiles(scanRoot, parentKey)
}

func (fs *fsStorage) DeleteAll(prefix string) (deletedKeys []string, err error) {
	dir := strings.TrimSuffix(utils.NormalizePathname(prefix)[1:], "/")
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	if dir == "" {
		return nil, errors.New("prefix is required")
	}
	keys, err := fs.List(prefix)
	if err != nil {
		return
	}
	absDir, err := fs.joinRootSafe(dir)
	if err != nil {
		return nil, err
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
