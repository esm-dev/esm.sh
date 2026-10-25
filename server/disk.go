package server

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ije/gox/utils"
)

type DiskStatus uint8

const (
	DiskStatusError DiskStatus = iota
	DiskStatusOk
	DiskStatusLow
	DiskStatusFull
)

type usage struct{ bytes, inodes int64 }

// cacheUsage is the usage of a local cache found by the last scan.
type cacheUsage struct {
	usage
	// [warning] the changes after the scan are read from the filesystem, so the other writers on it count too
	base    usage           // the used plus evicted space of the filesystem at the scan
	hours   map[int64]usage // the usage of the evictable entries by the hour of their last use
	scanned time.Time
}

type localCache struct{ dir, maxSize string }

var (
	purgeLock   sync.Mutex
	diskStatfs  = syscall.Statfs
	cacheUsages = map[string]*cacheUsage{}
	evicted     usage
)

func localCaches() []localCache {
	store := localCache{DefaultNpmRC().StoreDir(), config.NpmStoreMaxSize}
	if config.Storage.Type == "s3" && config.Storage.CacheDir != "" {
		// [warning] never purge a cache dir holding the work dir, it would remove the negative cache, binaries and certs
		cacheDir, err := filepath.Abs(config.Storage.CacheDir)
		rel, _ := filepath.Rel(cacheDir, config.WorkDir)
		if err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			// [warning] the S3 cache goes first on a low filesystem, a miss there costs one S3 read only
			return []localCache{{cacheDir, config.Storage.CacheMaxSize}, store}
		}
	}
	return []localCache{store}
}

// cacheShare returns the share of a filesystem that a cache may take, the max size is like "50%" or "500GB".
func cacheShare(maxSize string, fsSize uint64) (float64, error) {
	if s, ok := strings.CutSuffix(maxSize, "%"); ok {
		if p, err := strconv.ParseFloat(s, 64); err == nil && p > 0 && p <= 100 {
			return p / 100, nil
		}
	} else if n, err := utils.ParseBytes(maxSize); err == nil && n > 0 {
		return min(float64(n)/float64(fsSize), 1), nil
	}
	return 0, fmt.Errorf("invalid cache max size %q", maxSize)
}

// purgeLocalCaches keeps the local caches below their max sizes and frees space on low filesystems,
// the least recently used npm packages and the oldest S3 cache files go first.
// It returns the number of removed entries.
func purgeLocalCaches() (removed int, err error) {
	purgeLock.Lock()
	defer purgeLock.Unlock()
	// [warning] keep the entries used within a build timeout, a running build may still read them
	keepHour := time.Now().Add(-time.Duration(config.BuildTimeout)*time.Second-10*time.Minute).Unix() / 3600
	storeDir := DefaultNpmRC().StoreDir()
	for _, c := range localCaches() {
		status, stat := diskStatus(c.dir)
		if status == DiskStatusError {
			continue
		}
		fsSize := stat.Blocks * uint64(stat.Bsize)
		share, shareErr := cacheShare(c.maxSize, fsSize)
		if shareErr != nil {
			err = errors.Join(err, shareErr)
			continue
		}
		// [warning] the same share of the inodes too, a cache of tiny files runs out of inodes long before bytes
		limit := usage{int64(share * float64(fsSize)), int64(share * float64(stat.Files))}
		used := usage{int64(fsSize-stat.Bfree*uint64(stat.Bsize)) + evicted.bytes, int64(stat.Files-stat.Ffree) + evicted.inodes}
		u := cacheUsages[c.dir]
		var est usage
		if u != nil {
			est = usage{u.bytes + used.bytes - u.base.bytes, u.inodes + used.inodes - u.base.inodes}
		}
		low := status != DiskStatusOk
		over := u != nil && (est.bytes > limit.bytes || stat.Files > 0 && est.inodes > limit.inodes)
		// [warning] the estimate may be over the max size because of the other writers, check it by a scan, at most hourly
		rescan := u == nil || over && time.Since(u.scanned) >= time.Hour
		if !rescan && !low {
			continue
		}
		root, openErr := os.OpenRoot(c.dir)
		if openErr != nil {
			if !os.IsNotExist(openErr) {
				err = errors.Join(err, openErr)
			}
			continue
		}
		isStore := c.dir == storeDir
		if rescan {
			u = &cacheUsage{base: used, hours: map[int64]usage{}, scanned: time.Now()}
			var unit string
			var unitHour int64
			walkErr := fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					if !errors.Is(walkErr, fs.ErrNotExist) {
						err = errors.Join(err, walkErr)
					}
					return nil
				}
				inUnit := unit != "" && strings.HasPrefix(name, unit)
				isDir := d.IsDir()
				// skip staging installs, generated loaders and temporary cache files
				if !inUnit && name != "." && strings.HasPrefix(d.Name(), ".") {
					if isDir {
						return fs.SkipDir
					}
					return nil
				}
				info, infoErr := d.Info()
				if infoErr != nil {
					return nil
				}
				size := info.Size()
				if st, ok := info.Sys().(*syscall.Stat_t); ok {
					size = int64(st.Blocks) * 512
				}
				u.bytes += size
				u.inodes++
				hour := info.ModTime().Unix() / 3600
				if inUnit {
					hour = unitHour
				} else {
					unit = ""
					// npm packages are evicted as a whole, the S3 cache file by file
					if name == "." || isDir && (!isStore || strings.LastIndexByte(d.Name(), '@') <= 0) {
						return nil
					}
					if isDir {
						unit, unitHour = name+"/", hour
					}
				}
				h := u.hours[hour]
				u.hours[hour] = usage{h.bytes + size, h.inodes + 1}
				return nil
			})
			err = errors.Join(err, walkErr)
			cacheUsages[c.dir] = u
			est = u.usage
		}
		// evict to 90% of the max size, and until the filesystem has twice the low limits free
		need := usage{est.bytes - limit.bytes*9/10, 0}
		if stat.Files > 0 {
			need.inodes = est.inodes - limit.inodes*9/10
		}
		if low {
			need.bytes = max(need.bytes, 2048*MB-int64(stat.Bavail*uint64(stat.Bsize)))
			need.inodes = max(need.inodes, int64(stat.Files/10)-int64(stat.Ffree))
		}
		var cutoff int64
		var freed usage
		for _, hour := range slices.Sorted(maps.Keys(u.hours)) {
			if hour >= keepHour || freed.bytes >= need.bytes && freed.inodes >= need.inodes {
				break
			}
			freed.bytes += u.hours[hour].bytes
			freed.inodes += u.hours[hour].inodes
			delete(u.hours, hour)
			cutoff = hour + 1
		}
		if cutoff > 0 {
			walkErr := fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					if !errors.Is(walkErr, fs.ErrNotExist) {
						err = errors.Join(err, walkErr)
					}
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
				if isDir && (!isStore || strings.LastIndexByte(d.Name(), '@') <= 0) {
					return nil
				}
				if info, infoErr := d.Info(); infoErr == nil && info.ModTime().Unix()/3600 < cutoff {
					target := name
					// [warning] move the package away first, an install must not see a partly removed package
					if isDir {
						tmp := ".purge-" + rand.Text()
						if root.Rename(name, tmp) == nil {
							target = tmp
						}
					}
					if removeErr := root.RemoveAll(target); removeErr != nil {
						err = errors.Join(err, removeErr)
					} else {
						removed++
						// [warning] only the S3 cache dirs, an install may be moving a package into an empty scope dir
						for dir := path.Dir(name); !isStore && dir != "." && root.Remove(dir) == nil; dir = path.Dir(dir) {
						}
					}
				}
				if isDir {
					return fs.SkipDir
				}
				return nil
			})
			err = errors.Join(err, walkErr)
			u.bytes -= freed.bytes
			u.inodes -= freed.inodes
			evicted.bytes += freed.bytes
			evicted.inodes += freed.inodes
		}
		err = errors.Join(err, root.Close())
	}
	return
}

func checkDiskStatus() DiskStatus {
	status := DiskStatusOk
	for _, c := range localCaches() {
		// the worst status wins, an error only replaces ok
		if s, _ := diskStatus(c.dir); s != DiskStatusOk && (status == DiskStatusOk || s > status) {
			status = s
		}
	}
	return status
}

// diskStatus returns the status and the stats of the filesystem holding dir.
func diskStatus(dir string) (status DiskStatus, stat syscall.Statfs_t) {
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
		return DiskStatusError, stat
	}
	avail, inodes := stat.Bavail*uint64(stat.Bsize), 1.0
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
