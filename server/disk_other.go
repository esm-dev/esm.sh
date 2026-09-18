//go:build !linux && !windows

package server

// checkDiskStatus reports whether the disk the work dir lives on is getting
// low or full, so the npm cache can be purged before storage breaks. Platforms
// without a cheap free-space query are treated as always healthy.
func checkDiskStatus() DiskStatus {
	return DiskStatusOk
}
