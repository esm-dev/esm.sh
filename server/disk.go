package server

import (
	"os"
	"sync"
	"syscall"

	"github.com/ije/gox/log"
)

type DiskStatus uint8

const (
	DiskStatusError DiskStatus = iota
	DiskStatusOk
	DiskStatusLow
	DiskStatusFull
)

// npmStorePurgeLock serializes purge operations on the npm store to avoid
// concurrent rename/remove races within this process.
var npmStorePurgeLock sync.Mutex

func checkDiskStatus() DiskStatus {
	var stat syscall.Statfs_t
	err := syscall.Statfs(config.WorkDir, &stat)
	if err == nil {
		avail := stat.Bavail * uint64(stat.Bsize)
		if avail < 100*MB {
			return DiskStatusFull
		} else if avail < 1024*MB {
			return DiskStatusLow
		}
	} else {
		return DiskStatusError
	}
	return DiskStatusOk
}

func purgeNPMCacheWhenDiskIsLowOrFull(npmrc *NpmRC, logger *log.Logger) {
	if status := checkDiskStatus(); status == DiskStatusOk || status == DiskStatusError {
		return
	}

	npmStorePurgeLock.Lock()
	defer npmStorePurgeLock.Unlock()

	npmDir := npmrc.StoreDir()
	oldDir := npmDir + "_old"

	// Ensure any previous backup directory is removed so that the rename is idempotent.
	if err := os.RemoveAll(oldDir); err != nil {
		logger.Errorf("failed to remove previous npm cache backup directory %s: %v", oldDir, err)
	}

	if err := os.Rename(npmDir, oldDir); err != nil {
		// If the directory does not exist, there's nothing to purge.
		if !os.IsNotExist(err) {
			logger.Errorf("failed to rename npm cache directory %s to %s: %v", npmDir, oldDir, err)
		}
		return
	}

	if err := os.RemoveAll(oldDir); err != nil {
		logger.Errorf("failed to remove npm cache directory %s: %v", oldDir, err)
	}
}
