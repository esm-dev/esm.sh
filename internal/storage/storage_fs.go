package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type fsStorage struct {
	root *os.Root
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
	fsRoot, err := os.OpenRoot(root)
	if err != nil {
		return
	}
	return &fsStorage{root: fsRoot}, nil
}

// [warning] lexical checks alone do not prevent symlinks from escaping storage, always access the file via `fs.root`.
func cleanKey(key string) (filename string, err error) {
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
	return filepath.Clean(k), nil
}

func (fs *fsStorage) Stat(key string) (stat Stat, err error) {
	filename, err := cleanKey(key)
	if err != nil {
		return nil, err
	}
	fi, err := fs.root.Lstat(filename)
	if err != nil {
		if os.IsNotExist(err) || strings.HasSuffix(err.Error(), "not a directory") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if fi.IsDir() {
		return nil, ErrNotFound
	}
	return fi, nil
}

func (fs *fsStorage) StatContext(ctx context.Context, key string) (stat Stat, err error) {
	if err = ctx.Err(); err != nil {
		return
	}
	return fs.Stat(key)
}

func (fs *fsStorage) Get(key string) (content io.ReadCloser, stat Stat, err error) {
	filename, err := cleanKey(key)
	if err != nil {
		return
	}
	file, err := fs.root.Open(filename)
	if err != nil && (os.IsNotExist(err) || strings.HasSuffix(err.Error(), "not a directory")) {
		err = ErrNotFound
	}
	if err != nil {
		return
	}
	fi, err := file.Stat()
	if err == nil && fi.IsDir() {
		err = ErrNotFound
	}
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, fi, nil
}

func (fs *fsStorage) Put(key string, content io.Reader) (err error) {
	filename, err := cleanKey(key)
	if err != nil {
		return err
	}
	err = fs.root.MkdirAll(filepath.Dir(filename), 0755)
	if err != nil {
		return
	}

	tmpfile := filepath.Join(filepath.Dir(filename), ".storage-"+rand.Text())
	file, err := fs.root.OpenFile(tmpfile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return
	}
	defer fs.root.Remove(tmpfile)

	_, err = io.Copy(file, content)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return fs.root.Rename(tmpfile, filename)
}

func (fs *fsStorage) PutContext(ctx context.Context, key string, content io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fs.Put(key, content)
}

func (fs *fsStorage) Delete(key string) (err error) {
	filename, err := cleanKey(key)
	if err != nil {
		return err
	}
	return fs.root.Remove(filename)
}

func (fs *fsStorage) List(prefix string) (keys []string, err error) {
	dir := strings.Trim(strings.TrimSpace(filepath.ToSlash(prefix)), "/")
	if dir == "" {
		dir = "."
	}
	dir, err = cleanKey(dir)
	if err != nil {
		return nil, err
	}
	return findFiles(fs.root, filepath.ToSlash(dir))
}

func (fs *fsStorage) DeleteAll(prefix string) (deletedKeys []string, err error) {
	dir := strings.Trim(strings.TrimSpace(filepath.ToSlash(prefix)), "/")
	if dir == "" || filepath.Clean(dir) == "." {
		return nil, errors.New("prefix is required")
	}
	dir, err = cleanKey(dir)
	if err != nil {
		return nil, err
	}
	keys, err := findFiles(fs.root, filepath.ToSlash(dir))
	if err != nil {
		return nil, err
	}
	err = fs.root.RemoveAll(dir)
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
func findFiles(root *os.Root, parentDir string) ([]string, error) {
	entries, err := fs.ReadDir(root.FS(), parentDir)
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
		if parentDir != "." {
			path = parentDir + "/" + name
		}
		if entry.IsDir() {
			subFiles, err := findFiles(root, path)
			if err != nil {
				return nil, err
			}
			files = append(files, subFiles...)
		} else if !strings.HasPrefix(name, ".storage-") {
			files = append(files, path)
		}
	}
	return files, nil
}
