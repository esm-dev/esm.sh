package server

import (
	"log"
	"os"
	"os/exec"
	"syscall"
)

type DiskStatus uint8

const (
	DiskStatusError DiskStatus = iota
	DiskStatusOk
	DiskStatusLow
	DiskStatusFull
)

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

func purgeNPMCache(npmrc *NpmRC) {
	npmDir := npmrc.StoreDir()
	oldDir := npmDir + "_old"

	if err := os.Rename(npmDir, oldDir); err != nil {
		// If the directory does not exist, there's nothing to purge.
		if !os.IsNotExist(err) {
			log.Printf("failed to rename npm cache directory %s to %s: %v", npmDir, oldDir, err)
		}
		return
	}

	go func() {
		if err := os.RemoveAll(oldDir); err != nil {
			log.Printf("failed to remove npm cache directory %s: %v", oldDir, err)
		}
	}()
}
