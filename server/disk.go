package server

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

type DiskStatus uint8

const (
	DiskStatusError DiskStatus = iota
	DiskStatusOk
	DiskStatusLow
	DiskStatusFull
)

var (
	npmStoreLock       sync.RWMutex
	npmStoreGeneration uint64
	diskStatfs         = syscall.Statfs
)

func localCacheDirs() []string {
	dirs := []string{filepath.Join(config.WorkDir, "npm")}
	if config.Storage.Type == "s3" && config.Storage.CacheDir != "" {
		dirs = append(dirs, config.Storage.CacheDir)
	}
	return dirs
}

func purgeLocalCaches(generation uint64, force bool) (purged bool, err error) {
	if !force && checkDiskStatus() == DiskStatusOk {
		return false, nil
	}
	npmStoreLock.Lock()
	defer npmStoreLock.Unlock()
	if force {
		if generation != npmStoreGeneration {
			return false, nil
		}
	} else {
		switch checkDiskStatus() {
		case DiskStatusOk:
			return false, nil
		case DiskStatusError:
			return false, errors.New("failed to check local cache disk status")
		}
	}
	npmStoreGeneration++

	err = os.RemoveAll(filepath.Join(config.WorkDir, "npm_old"))
	for _, dir := range localCacheDirs() {
		root, openErr := os.OpenRoot(dir)
		if os.IsNotExist(openErr) {
			continue
		}
		if openErr != nil {
			err = errors.Join(err, openErr)
			continue
		}
		// [warning] keep the root: S3 cache storage holds an open handle, and renaming can fail on a full disk.
		entries, readErr := fs.ReadDir(root.FS(), ".")
		err = errors.Join(err, readErr)
		for _, entry := range entries {
			if removeErr := root.RemoveAll(entry.Name()); removeErr != nil {
				err = errors.Join(err, fmt.Errorf("purge %s: %w", filepath.Join(dir, entry.Name()), removeErr))
			}
		}
		err = errors.Join(err, root.Close())
	}
	return true, err
}

func checkDiskStatus() DiskStatus {
	status := DiskStatusOk
	for _, dir := range localCacheDirs() {
		var stat syscall.Statfs_t
		var err error
		for {
			err = diskStatfs(dir, &stat)
			parent := filepath.Dir(dir)
			if !os.IsNotExist(err) || parent == dir {
				break
			}
			dir = parent
		}
		if err != nil {
			if status == DiskStatusOk {
				status = DiskStatusError
			}
			continue
		}
		avail := stat.Bavail * uint64(stat.Bsize)
		if avail < 100*MB || (stat.Files > 0 && (stat.Ffree == 0 || stat.Ffree < stat.Files/100)) {
			return DiskStatusFull
		}
		if avail < 1024*MB || (stat.Files > 0 && stat.Ffree < stat.Files/20) {
			status = DiskStatusLow
		}
	}
	return status
}
