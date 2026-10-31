package server

import (
	"os"
	"path/filepath"
	"syscall"
)

type DiskStatus uint8

const (
	DiskStatusError DiskStatus = iota
	DiskStatusOk
	DiskStatusLow
	DiskStatusFull
)

var diskStatfs = syscall.Statfs

func checkDiskStatus() DiskStatus {
	dirs := []string{DefaultNpmRC().StoreDir()}
	if config.Storage.Type == "s3" && config.Storage.CacheDir != "" {
		dirs = append(dirs, config.Storage.CacheDir)
	}
	status := DiskStatusOk
	for _, dir := range dirs {
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
