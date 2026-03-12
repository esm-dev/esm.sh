package server

import (
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
	os.Rename(npmDir, npmDir+"_old")
	exec.Command("rm", "-rf", npmDir+"_old").Start()
}
