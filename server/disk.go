package server

import (
	"os"
	"sync"
	"sync/atomic"
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

// npmStorePurgeMu serializes purge operations on the npm store to avoid
// concurrent rename/remove races within this process.
var npmStorePurgeMu sync.Mutex

// npmStorePurging is an atomic flag indicating that a purge of the npm store
// is in progress. Other code paths that access the store can use this flag
// to coordinate with purgeNPMCacheWhenDiskIsLowOrFull.
var npmStorePurging int32

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

	// Serialize purge operations and mark that a purge is in progress so that
	// other code paths can coordinate and avoid racing with this destructive
	// rename/remove of the npm store directory.
	npmStorePurgeMu.Lock()
	atomic.StoreInt32(&npmStorePurging, 1)
	defer func() {
		atomic.StoreInt32(&npmStorePurging, 0)
		npmStorePurgeMu.Unlock()
	}()

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
