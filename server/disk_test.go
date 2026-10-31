//go:build unix

package server

import (
	"bytes"
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

func TestInstallPackageDiskRecovery(t *testing.T) {
	for _, test := range []struct {
		name     string
		err      error
		failures int32
		requests int32
		purged   bool
	}{
		{"recovered", syscall.ENOSPC, 1, 2, true},
		{"still full", syscall.ENOSPC, 2, 2, true},
		{"other error", syscall.EIO, 1, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := useNpmStore(t)
			transport, oldStatfs := http.DefaultTransport, diskStatfs
			t.Cleanup(func() { http.DefaultTransport, diskStatfs = transport, oldStatfs })
			diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
				*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 512 * MB / 4096}
				return nil
			}
			npmrc := newTestNpmRC()
			pkg := npm.Package{Name: "disk-recovery", Version: "1.0.0"}
			key := "npm:" + pkg.String()
			setCacheItem(key, &npm.PackageJSON{Name: pkg.Name, Version: pkg.Version, Dist: npm.NpmPackageDist{Tarball: npmRegistry + "disk-recovery.tgz"}}, time.Minute)
			t.Cleanup(func() { deleteCacheItem(key) })
			// the purge removes an idle package, and keeps the package of a running build
			for _, name := range []string{"idle@1.0.0", "used@1.0.0"} {
				if err := os.MkdirAll(filepath.Join(s.dir, name), 0755); err != nil {
					t.Fatal(err)
				}
			}
			s.use("idle@1.0.0", npmPackageTTL)()
			defer s.use("used@1.0.0", npmPackageTTL)()
			var requests atomic.Int32
			http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
				var body io.Reader = bytes.NewReader(testPackageTarball(`{"name":"disk-recovery","version":"1.0.0"}`))
				if requests.Add(1) <= test.failures {
					body = iotest.ErrReader(&os.PathError{Op: "write", Path: "package.json", Err: test.err})
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(body)}, nil
			})
			_, err := npmrc.installPackage(pkg)
			if test.failures >= test.requests {
				if !errors.Is(err, test.err) {
					t.Fatalf("install error = %v, want %v", err, test.err)
				}
			} else if err != nil || !existsDir(filepath.Join(s.dir, pkg.String())) {
				t.Fatalf("install after the purge failed: %v", err)
			}
			if requests.Load() != test.requests {
				t.Fatalf("downloads = %d, want %d", requests.Load(), test.requests)
			}
			if existsDir(filepath.Join(s.dir, "idle@1.0.0")) == test.purged {
				t.Fatalf("idle package removed = %v, want %v", !test.purged, test.purged)
			}
			if !existsDir(filepath.Join(s.dir, "used@1.0.0")) {
				t.Fatal("purged the package of a running build")
			}
			for _, pattern := range []string{".install-*", ".purge-*"} {
				if dirs, err := filepath.Glob(filepath.Join(s.dir, pattern)); err != nil || len(dirs) != 0 {
					t.Fatalf("temporary directories remain: %v, %v", dirs, err)
				}
			}
		})
	}
}

func TestNpmStorePurgeIdle(t *testing.T) {
	s := useNpmStore(t)
	oldStatfs := diskStatfs
	t.Cleanup(func() { diskStatfs = oldStatfs })
	keys := []string{"a@1.0.0", "b@1.0.0", "c@1.0.0"}
	for _, key := range keys {
		if err := os.MkdirAll(filepath.Join(s.dir, key), 0755); err != nil {
			t.Fatal(err)
		}
		s.use(key, npmPackageTTL)()
		time.Sleep(time.Millisecond)
	}
	// the least recently used package goes first, and the purge stops once the disk is ok
	var calls int
	diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
		calls++
		*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 16 * GB / 4096}
		if calls == 1 {
			stat.Bavail = 512 * MB / 4096
		}
		return nil
	}
	s.purgeIdle()
	if existsDir(filepath.Join(s.dir, keys[0])) || !existsDir(filepath.Join(s.dir, keys[1])) || !existsDir(filepath.Join(s.dir, keys[2])) || len(s.items) != 2 {
		t.Fatal("purge did not remove the least recently used package only")
	}
	for _, err := range []error{nil, syscall.EIO} {
		diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
			*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 16 * GB / 4096}
			return err
		}
		if s.purgeIdle(); len(s.items) != 2 {
			t.Fatalf("purged on a healthy or unknown filesystem (%v)", err)
		}
	}
}
