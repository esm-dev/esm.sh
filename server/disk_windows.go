//go:build windows

package server

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func checkDiskStatus() DiskStatus {
	// `GetDiskFreeSpaceEx` needs an existing directory; use the volume root of
	// the work dir so it works before the work dir has been created.
	volumeRoot := filepath.VolumeName(config.WorkDir)
	path, err := windows.UTF16PtrFromString(volumeRoot + string(filepath.Separator))
	if err != nil {
		return DiskStatusError
	}
	var freeBytesAvailable uint64
	if err := windows.GetDiskFreeSpaceEx(path, &freeBytesAvailable, nil, nil); err != nil {
		return DiskStatusError
	}
	if freeBytesAvailable < 100*MB {
		return DiskStatusFull
	} else if freeBytesAvailable < 1024*MB {
		return DiskStatusLow
	}
	return DiskStatusOk
}
