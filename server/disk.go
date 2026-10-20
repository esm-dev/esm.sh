package server

import (
	"crypto/rand"
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
	diskStatfs = syscall.Statfs
)

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

// purgeLocalCaches removes old npm packages and S3 cache files from the low filesystems until they have
// twice the low limits free, and returns the number of removed entries.
func purgeLocalCaches() (removed int, err error) {
	purgeLock.Lock()
	defer purgeLock.Unlock()
	// [warning] keep the entries used within a build timeout, a running build may still read them
	keepTime := time.Now().Add(-time.Duration(config.BuildTimeout)*time.Second - 10*time.Minute)
	storeDir := DefaultNpmRC().StoreDir()
	for _, dir := range localCacheDirs() {
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
			// skip staging installs, generated loaders and temporary cache files
			if strings.HasPrefix(d.Name(), ".") {
				if isDir {
					return fs.SkipDir
				}
				return nil
			}
			// npm packages are removed as a whole, the S3 cache file by file
			if isDir && (dir != storeDir || strings.LastIndexByte(d.Name(), '@') <= 0) {
				return nil
			}
			if info, infoErr := d.Info(); infoErr == nil && info.ModTime().Before(keepTime) {
				// [warning] move the package away first, an install must not see a partly removed package
				if dir == storeDir {
					tmp := ".purge-" + rand.Text()
					if root.Rename(name, tmp) == nil {
						name = tmp
					}
				}
				if removeErr := root.RemoveAll(name); removeErr != nil {
					err = errors.Join(err, removeErr)
				} else {
					removed++
				}
				if _, avail, inodes := diskStatus(dir); avail >= 2048*MB && inodes >= 0.1 {
					return fs.SkipAll
				}
			}
			if isDir {
				return fs.SkipDir
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
	case avail < 1024*MB || inodes < 0.05:
		status = DiskStatusLow
	default:
		status = DiskStatusOk
	}
	return
}
