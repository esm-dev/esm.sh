//go:build unix

package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	t.Run("S3 cache holding the work dir", func(t *testing.T) {
		for _, cacheDir := range []string{config.WorkDir, filepath.Dir(config.WorkDir)} {
			config.Storage = storage.StorageOptions{Type: "s3", CacheDir: cacheDir}
			if dirs := localCacheDirs(); len(dirs) != 1 {
				t.Fatalf("cache dirs = %v, want the npm store only", dirs)
			}
		}
	})
}

func TestPurgeLocalCaches(t *testing.T) {
	oldConfig, oldStatfs := config, diskStatfs
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() { config, diskStatfs = oldConfig, oldStatfs })
	var requests []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method)
		switch r.Method {
		case http.MethodPut:
			io.Copy(io.Discard, r.Body)
		case http.MethodGet:
			w.Header().Set("Content-Length", "4")
			w.Header().Set("Last-Modified", "Tue, 20 Oct 2026 00:00:00 GMT")
			io.WriteString(w, "data")
		default:
			t.Errorf("unexpected remote operation: %s", r.Method)
			w.WriteHeader(500)
		}
	}))
	defer upstream.Close()
	config.Storage = storage.StorageOptions{
		Type: "s3", CacheDir: t.TempDir(), Endpoint: upstream.URL,
		AccessKeyID: "test", SecretAccessKey: "test",
	}
	s3, err := storage.New(&config.Storage)
	if err != nil {
		t.Fatal(err)
	}
	const key = "meta/pkg@1.0.0/build"
	if err := s3.Put(key, strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	cacheRoot, err := os.Stat(config.Storage.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	store, cache, outside := filepath.Join(config.WorkDir, "npm"), config.Storage.CacheDir, t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"old@1.0.0/node_modules/old/package.json",
		"@scope/old@1.0.0/node_modules/@scope/old/package.json",
		"gh/owner/repo@abc/node_modules/repo/package.json",
		"new@1.0.0/node_modules/new/package.json",
		".install-1/node_modules/new/package.json",
		".loaders/svelte@5.0.0.js",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(store, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(store, name), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(store, "old@1.0.0/node_modules/dep")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cache, "modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "modules/new.mjs"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-24 * time.Hour)
	for _, name := range []string{
		filepath.Join(store, "old@1.0.0"), filepath.Join(store, "@scope/old@1.0.0"), filepath.Join(store, "gh/owner/repo@abc"),
		filepath.Join(store, ".install-1"), filepath.Join(store, ".loaders/svelte@5.0.0.js"), filepath.Join(cache, key),
	} {
		if err := os.Chtimes(name, old, old); err != nil {
			t.Fatal(err)
		}
	}
	diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
		*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 512 * MB / 4096}
		return nil
	}
	if removed, err := purgeLocalCaches(); err != nil || removed != 4 {
		t.Fatalf("purge = %d, %v, want 4 removed", removed, err)
	}
	for _, name := range []string{"old@1.0.0", "@scope/old@1.0.0", "gh/owner/repo@abc"} {
		if _, err := os.Lstat(filepath.Join(store, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("old package %s remains: %v", name, err)
		}
	}
	for _, name := range []string{
		filepath.Join(store, "new@1.0.0/node_modules/new/package.json"), filepath.Join(store, ".install-1/node_modules/new/package.json"),
		filepath.Join(store, ".loaders/svelte@5.0.0.js"), filepath.Join(cache, "modules/new.mjs"), filepath.Join(outside, "keep"),
	} {
		if !existsFile(name) {
			t.Fatalf("purged %s", name)
		}
	}
	if leftovers, _ := filepath.Glob(filepath.Join(store, ".purge-*")); len(leftovers) != 0 {
		t.Fatalf("purge leftovers: %v", leftovers)
	}
	if existsFile(filepath.Join(cache, key)) {
		t.Fatal("old S3 cache file remains")
	}
	if after, err := os.Stat(cache); err != nil || !os.SameFile(cacheRoot, after) {
		t.Fatalf("S3 cache root was replaced: %v", err)
	}
	for range 2 {
		content, _, err := s3.Get(key)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(content)
		content.Close()
		if err != nil || string(data) != "data" {
			t.Fatalf("S3 read after purge = %q, %v", data, err)
		}
	}
	if !slices.Equal(requests, []string{"PUT", "GET"}) {
		t.Fatalf("remote objects or cache reuse affected: %v", requests)
	}

	// stop once the disk has enough free space
	for _, name := range []string{"a@1.0.0", "b@1.0.0"} {
		if err := os.MkdirAll(filepath.Join(store, name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(store, name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	var calls int
	diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
		calls++
		*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 32 * GB / 4096}
		if calls == 1 {
			stat.Bavail = 512 * MB / 4096
		}
		return nil
	}
	if removed, err := purgeLocalCaches(); err != nil || removed != 1 || existsDir(filepath.Join(store, "a@1.0.0")) || !existsDir(filepath.Join(store, "b@1.0.0")) {
		t.Fatalf("purge = %d, %v, want the first old package removed only", removed, err)
	}
	for _, err := range []error{nil, syscall.EIO} {
		diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
			*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 32 * GB / 4096}
			return err
		}
		if removed, purgeErr := purgeLocalCaches(); purgeErr != nil || removed != 0 || !existsDir(filepath.Join(store, "b@1.0.0")) {
			t.Fatalf("purged a healthy or unknown filesystem (%v): %d, %v", err, removed, purgeErr)
		}
	}
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
			oldConfig, transport, oldStatfs := config, http.DefaultTransport, diskStatfs
			cfg := *config
			config = &cfg
			config.WorkDir = t.TempDir()
			config.Storage = storage.StorageOptions{}
			t.Cleanup(func() { config, http.DefaultTransport, diskStatfs = oldConfig, transport, oldStatfs })
			diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
				*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 512 * MB / 4096}
				return nil
			}
			npmrc := newTestNpmRC()
			pkg := npm.Package{Name: "disk-recovery", Version: "1.0.0"}
			key := "npm:" + pkg.String()
			setCacheItem(key, &npm.PackageJSON{Name: pkg.Name, Version: pkg.Version, Dist: npm.NpmPackageDist{Tarball: npmRegistry + "disk-recovery.tgz"}}, time.Minute)
			t.Cleanup(func() { deleteCacheItem(key) })
			// the purge removes a package not used for a day, and keeps the package of a running build
			old := time.Now().Add(-24 * time.Hour)
			oldPackage, usedPackage := filepath.Join(npmrc.StoreDir(), "old@1.0.0"), filepath.Join(npmrc.StoreDir(), "used@1.0.0")
			for _, dir := range []string{oldPackage, usedPackage} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chtimes(oldPackage, old, old); err != nil {
				t.Fatal(err)
			}
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
			} else if err != nil {
				t.Fatal(err)
			}
			if requests.Load() != test.requests {
				t.Fatalf("downloads = %d, want %d", requests.Load(), test.requests)
			}
			if existsDir(oldPackage) == test.purged {
				t.Fatalf("old package removed = %v, want %v", !existsDir(oldPackage), test.purged)
			}
			if !existsDir(usedPackage) {
				t.Fatal("purged the package of a running build")
			}
			for _, pattern := range []string{".install-*", ".purge-*"} {
				if dirs, err := filepath.Glob(filepath.Join(npmrc.StoreDir(), pattern)); err != nil || len(dirs) != 0 {
					t.Fatalf("temporary directories remain: %v, %v", dirs, err)
				}
			}
			if err != nil {
				return
			}
			// an installed package is marked as used again on every install
			installDir := filepath.Join(npmrc.StoreDir(), pkg.String())
			if err := os.Chtimes(installDir, old, old); err != nil {
				t.Fatal(err)
			}
			if _, err := npmrc.installPackage(pkg); err != nil {
				t.Fatal(err)
			}
			if fi, err := os.Stat(installDir); err != nil || fi.ModTime().Before(time.Now().Add(-time.Minute)) {
				t.Fatalf("installed package is not marked as used: %v", err)
			}
		})
	}
}

func TestPurgeOnNoSpace(t *testing.T) {
	for _, test := range []struct {
		err  error
		wake bool
	}{
		{&os.PathError{Op: "write", Path: "a.mjs", Err: syscall.ENOSPC}, true},
		{errors.New("git clone: write error: no space left on device"), true},
		{syscall.EIO, false},
		{nil, false},
	} {
		// the second call must not block on the pending wake
		purgeOnNoSpace(test.err)
		purgeOnNoSpace(test.err)
		select {
		case <-purgeNow:
			if !test.wake {
				t.Fatalf("woke the purge loop on %v", test.err)
			}
		default:
			if test.wake {
				t.Fatalf("did not wake the purge loop on %v", test.err)
			}
		}
	}
}
