package server

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type DiskStatus uint8

const (
	DiskStatusError DiskStatus = iota
	DiskStatusOk
	DiskStatusLow
	DiskStatusFull
)

var (
	purgeLock  sync.Mutex
	purgeNow   = make(chan struct{}, 1) // wakes the purge loop before its next tick
	diskStatfs = syscall.Statfs
)

// purgeOnNoSpace wakes the purge loop when err is a "no space left on device" error.
func purgeOnNoSpace(err error) {
	// [warning] match the message too, the errors of git, esbuild and the storage may not wrap the errno
	if err != nil && (errors.Is(err, syscall.ENOSPC) || strings.Contains(err.Error(), "no space left on device")) {
		select {
		case purgeNow <- struct{}{}:
		default:
		}
	}
}

func localCacheDirs() []string {
	dirs := []string{DefaultNpmRC().StoreDir()}
	if config.Storage.Type == "s3" && config.Storage.CacheDir != "" {
		// [warning] never purge a cache dir holding the work dir, it would remove the negative cache, binaries and certs
		cacheDir, err := filepath.Abs(config.Storage.CacheDir)
		rel, _ := filepath.Rel(cacheDir, config.WorkDir)
		if err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			dirs = append(dirs, cacheDir)
		}
	}
	return dirs
}

func purgeLocalCaches() (removed int, err error) {
	purgeLock.Lock()
	defer purgeLock.Unlock()
	// [warning] keep the entries used within a build timeout, a running build may still read them
	keepTime := time.Now().Add(-time.Duration(config.BuildTimeout)*time.Second - 10*time.Minute)
	for _, dir := range localCacheDirs()[1:] {
		if status, _, _ := diskStatus(dir); status != DiskStatusLow && status != DiskStatusFull {
			continue
		}
		root, openErr := os.OpenRoot(dir)
		if openErr != nil {
			if !os.IsNotExist(openErr) {
				err = errors.Join(err, openErr)
			}
			continue
		}
		walkErr := fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				err = errors.Join(err, walkErr)
				return nil
			}
			if name == "." {
				return nil
			}
			isDir := d.IsDir()
			if strings.HasPrefix(d.Name(), ".") {
				if isDir {
					return fs.SkipDir
				}
				return nil
			}
			if isDir {
				return nil
			}
			if info, infoErr := d.Info(); infoErr == nil && info.ModTime().Before(keepTime) {
				if removeErr := root.RemoveAll(name); removeErr != nil {
					err = errors.Join(err, removeErr)
				} else {
					removed++
				}
				if _, avail, inodes := diskStatus(dir); avail >= 20*GB && inodes >= 0.1 {
					return fs.SkipAll
				}
			}
			return nil
		})
		err = errors.Join(err, walkErr, root.Close())
	}
	return
}

func checkDiskStatus() DiskStatus {
	status := DiskStatusOk
	for _, dir := range localCacheDirs() {
		// the worst status wins, an error only replaces ok
		if s, _, _ := diskStatus(dir); s != DiskStatusOk && (status == DiskStatusOk || s > status) {
			status = s
		}
	}
	return status
}

// diskStatus returns the status, the available bytes and the free inode ratio of the filesystem holding dir.
func diskStatus(dir string) (status DiskStatus, avail uint64, inodes float64) {
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
		return DiskStatusError, 0, 0
	}
	avail, inodes = stat.Bavail*uint64(stat.Bsize), 1
	if stat.Files > 0 {
		inodes = float64(stat.Ffree) / float64(stat.Files)
	}
	switch {
	case avail < 100*MB || inodes < 0.01:
		status = DiskStatusFull
	case avail < 10*GB || inodes < 0.05:
		status = DiskStatusLow
	default:
		status = DiskStatusOk
	}
	return
}
