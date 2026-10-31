//go:build unix

package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/iotest"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
)

func TestCheckDiskStatus(t *testing.T) {
	oldConfig, oldStatfs := config, diskStatfs
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() { config, diskStatfs = oldConfig, oldStatfs })
	for _, test := range []struct {
		name   string
		bytes  uint64
		files  uint64
		free   uint64
		err    error
		status DiskStatus
	}{
		{"healthy", 16 * GB, 100000, 10000, nil, DiskStatusOk},
		{"low bytes", 512 * MB, 100000, 10000, nil, DiskStatusLow},
		{"full bytes", 50 * MB, 100000, 10000, nil, DiskStatusFull},
		{"low inodes", 16 * GB, 100000, 4999, nil, DiskStatusLow},
		{"full inodes", 16 * GB, 100000, 999, nil, DiskStatusFull},
		{"no inodes", 16 * GB, 10, 0, nil, DiskStatusFull},
		{"no inode counts", 16 * GB, 0, 0, nil, DiskStatusOk},
		{"stat error", 0, 0, 0, syscall.EIO, DiskStatusError},
	} {
		t.Run(test.name, func(t *testing.T) {
			diskStatfs = func(path string, stat *syscall.Statfs_t) error {
				if path != filepath.Join(config.WorkDir, "npm") {
					t.Errorf("checked %q instead of the npm filesystem", path)
				}
				*stat = syscall.Statfs_t{Bsize: 4096, Bavail: test.bytes / 4096, Files: test.files, Ffree: test.free}
				return test.err
			}
			if got := checkDiskStatus(); got != test.status {
				t.Fatalf("disk status = %v, want %v", got, test.status)
			}
		})
	}
	t.Run("missing store and separate S3 cache filesystem", func(t *testing.T) {
		config.Storage = storage.StorageOptions{Type: "s3", CacheDir: t.TempDir()}
		var paths []string
		diskStatfs = func(path string, stat *syscall.Statfs_t) error {
			paths = append(paths, path)
			if path == filepath.Join(config.WorkDir, "npm") {
				return syscall.ENOENT
			}
			*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 16 * GB / 4096}
			if path == config.Storage.CacheDir {
				stat.Bavail = 50 * MB / 4096
			}
			return nil
		}
		if got := checkDiskStatus(); got != DiskStatusFull {
			t.Fatalf("S3 cache filesystem is full, got %v", got)
		}
		if want := []string{filepath.Join(config.WorkDir, "npm"), config.WorkDir, config.Storage.CacheDir}; !slices.Equal(paths, want) {
			t.Fatalf("checked %v, want %v", paths, want)
		}
	})
}

func TestInstallPackageDiskError(t *testing.T) {
	for _, diskErr := range []error{syscall.ENOSPC, syscall.EIO} {
		t.Run(diskErr.Error(), func(t *testing.T) {
			oldConfig, transport := config, http.DefaultTransport
			cfg := *config
			cfg.WorkDir = t.TempDir()
			config = &cfg
			t.Cleanup(func() { config, http.DefaultTransport = oldConfig, transport })
			npmrc := newTestNpmRC()
			pkg := npm.Package{Name: "disk-error", Version: "1.0.0"}
			key := "npm:" + pkg.String()
			setCacheItem(key, &npm.PackageJSON{Name: pkg.Name, Version: pkg.Version, Dist: npm.NpmPackageDist{Tarball: npmRegistry + "disk-error.tgz"}}, time.Minute)
			t.Cleanup(func() { deleteCacheItem(key) })
			var requests atomic.Int32
			http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)
				body := iotest.ErrReader(&os.PathError{Op: "write", Path: "package.json", Err: diskErr})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(body)}, nil
			})
			if _, err := npmrc.installPackage(pkg); !errors.Is(err, diskErr) {
				t.Fatalf("install error = %v, want %v", err, diskErr)
			}
			if requests.Load() != 1 {
				t.Fatalf("downloads = %d, want 1", requests.Load())
			}
			if dirs, err := filepath.Glob(filepath.Join(npmrc.StoreDir(), ".install-*")); err != nil || len(dirs) != 0 {
				t.Fatalf("temporary directories remain: %v, %v", dirs, err)
			}
		})
	}
}
