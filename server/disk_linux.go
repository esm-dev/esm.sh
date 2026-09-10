//go:build linux

package server

import "syscall"

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
