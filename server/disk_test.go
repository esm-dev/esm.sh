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
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/iotest"
	"time"
	"unsafe"

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
		{"healthy", 2048 * MB, 100000, 10000, nil, DiskStatusOk},
		{"low bytes", 512 * MB, 100000, 10000, nil, DiskStatusLow},
		{"full bytes", 50 * MB, 100000, 10000, nil, DiskStatusFull},
		{"low inodes", 2048 * MB, 100000, 4999, nil, DiskStatusLow},
		{"full inodes", 2048 * MB, 100000, 999, nil, DiskStatusFull},
		{"no inodes", 2048 * MB, 10, 0, nil, DiskStatusFull},
		{"no inode counts", 2048 * MB, 0, 0, nil, DiskStatusOk},
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
			*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 2048 * MB / 4096}
			if path == config.Storage.CacheDir {
				stat.Bavail = 50 * MB / 4096
			}
			return nil
		}
		if got := checkDiskStatus(); got != DiskStatusFull {
			t.Fatalf("S3 cache filesystem is full, got %v", got)
		}
		if want := []string{config.Storage.CacheDir, filepath.Join(config.WorkDir, "npm"), config.WorkDir}; !slices.Equal(paths, want) {
			t.Fatalf("checked %v, want %v", paths, want)
		}
	})
	t.Run("S3 cache holding the work dir", func(t *testing.T) {
		for _, cacheDir := range []string{config.WorkDir, filepath.Dir(config.WorkDir)} {
			config.Storage = storage.StorageOptions{Type: "s3", CacheDir: cacheDir}
			if caches := localCaches(); len(caches) != 1 {
				t.Fatalf("caches = %v, want the npm store only", caches)
			}
		}
	})
}

func TestPurgeLocalCaches(t *testing.T) {
	oldConfig, oldStatfs := config, diskStatfs
	config = &Config{WorkDir: t.TempDir(), NpmStoreMaxSize: "50%"}
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
		Type: "s3", CacheDir: t.TempDir(), CacheMaxSize: "20%", Endpoint: upstream.URL,
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
	statfs := func(avail uint64, err error) func(string, *syscall.Statfs_t) error {
		return func(_ string, stat *syscall.Statfs_t) error {
			*stat = syscall.Statfs_t{Bsize: 4096, Blocks: 10240 * MB / 4096, Bfree: avail / 4096, Bavail: avail / 4096}
			return err
		}
	}
	diskStatfs = statfs(512*MB, nil)
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
	if existsFile(filepath.Join(cache, key)) || existsDir(filepath.Join(cache, "meta")) {
		t.Fatal("old S3 cache file or its empty dirs remain")
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

	// a healthy or unknown filesystem below the max sizes is left as is
	idle := filepath.Join(store, "idle@1.0.0")
	if err := os.MkdirAll(idle, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(idle, old, old); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{nil, syscall.EIO} {
		diskStatfs = statfs(4096*MB, err)
		if removed, purgeErr := purgeLocalCaches(); purgeErr != nil || removed != 0 || !existsDir(idle) {
			t.Fatalf("purged a healthy or unknown filesystem (%v): %d, %v", err, removed, purgeErr)
		}
	}
}

func TestPurgeLocalCachesMaxSize(t *testing.T) {
	oldConfig, oldStatfs := config, diskStatfs
	t.Cleanup(func() { config, diskStatfs = oldConfig, oldStatfs })
	// a healthy 100GB filesystem, the inodes are counted when files is set
	var fsUsed uint64 = 51200 * MB
	statfs := func(files, ffree uint64) func(string, *syscall.Statfs_t) error {
		return func(_ string, stat *syscall.Statfs_t) error {
			free := (102400*MB - fsUsed) / 4096
			*stat = syscall.Statfs_t{Bsize: 4096, Blocks: 102400 * MB / 4096, Bfree: free, Bavail: free, Files: files, Ffree: ffree}
			return nil
		}
	}
	writePackage := func(name string, files int, size int, lastUse time.Time) {
		dir := filepath.Join(DefaultNpmRC().StoreDir(), name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for i := range files {
			if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(i)), make([]byte, size), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(dir, lastUse, lastUse); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()

	t.Run("bytes", func(t *testing.T) {
		config = &Config{WorkDir: t.TempDir(), NpmStoreMaxSize: "3MB"}
		diskStatfs = statfs(0, 0)
		// the least recently used package goes first, not the first by name
		writePackage("a@1.0.0", 1, MB, now.Add(-3*time.Hour))
		writePackage("b@1.0.0", 1, MB, now.Add(-30*time.Hour))
		writePackage("c@1.0.0", 1, MB, now.Add(-2*time.Hour))
		if removed, err := purgeLocalCaches(); err != nil || removed != 1 {
			t.Fatalf("purge = %d, %v, want 1 removed", removed, err)
		}
		for name, want := range map[string]bool{"a@1.0.0": true, "b@1.0.0": false, "c@1.0.0": true} {
			if existsDir(filepath.Join(DefaultNpmRC().StoreDir(), name)) != want {
				t.Fatalf("%s kept = %v, want %v", name, !want, want)
			}
		}
		// the writes of another cache on the filesystem push the estimate over the max size,
		// it is checked by a scan after an hour, which finds the real usage
		fsUsed += MB
		u := cacheUsages[DefaultNpmRC().StoreDir()]
		u.scanned = now
		if removed, err := purgeLocalCaches(); err != nil || removed != 0 || u != cacheUsages[DefaultNpmRC().StoreDir()] {
			t.Fatalf("purge within the hour = %d, %v, want no scan", removed, err)
		}
		u.scanned = now.Add(-2 * time.Hour)
		if removed, err := purgeLocalCaches(); err != nil || removed != 0 || u == cacheUsages[DefaultNpmRC().StoreDir()] {
			t.Fatalf("purge after the hour = %d, %v, want a scan and 0 removed", removed, err)
		}
		writePackage("d@1.0.0", 2, MB, now.Add(-30*time.Hour))
		fsUsed += 2 * MB
		cacheUsages[DefaultNpmRC().StoreDir()].scanned = now.Add(-2 * time.Hour)
		if removed, err := purgeLocalCaches(); err != nil || removed != 1 || existsDir(filepath.Join(DefaultNpmRC().StoreDir(), "d@1.0.0")) {
			t.Fatalf("purge after the next scan = %d, %v, want d@1.0.0 removed", removed, err)
		}
	})

	t.Run("inodes", func(t *testing.T) {
		// 1% of 1000 inodes allows 10, and the package with many tiny files goes
		config = &Config{WorkDir: t.TempDir(), NpmStoreMaxSize: "1%"}
		diskStatfs = statfs(1000, 900)
		writePackage("tiny@1.0.0", 20, 1, now.Add(-30*time.Hour))
		writePackage("used@1.0.0", 1, 1, now.Add(-2*time.Hour))
		if removed, err := purgeLocalCaches(); err != nil || removed != 1 || existsDir(filepath.Join(DefaultNpmRC().StoreDir(), "tiny@1.0.0")) || !existsDir(filepath.Join(DefaultNpmRC().StoreDir(), "used@1.0.0")) {
			t.Fatalf("purge = %d, %v, want tiny@1.0.0 removed only", removed, err)
		}
	})

	t.Run("low", func(t *testing.T) {
		// 20% of 1000 inodes allows 200, the store has 4 packages of 31 inodes
		config = &Config{WorkDir: t.TempDir(), NpmStoreMaxSize: "20%"}
		diskStatfs = statfs(1000, 500)
		store := DefaultNpmRC().StoreDir()
		for i, name := range []string{"a@1.0.0", "b@1.0.0", "c@1.0.0", "d@1.0.0"} {
			writePackage(name, 30, 1, now.Add(-time.Duration(40-i)*time.Hour))
		}
		if removed, err := purgeLocalCaches(); err != nil || removed != 0 {
			t.Fatalf("purge = %d, %v, want 0 removed", removed, err)
		}
		// the other writers push the estimate over the max size, but the low filesystem needs 60 inodes only
		diskStatfs = statfs(1000, 40)
		if removed, err := purgeLocalCaches(); err != nil || removed != 2 || existsDir(filepath.Join(store, "b@1.0.0")) || !existsDir(filepath.Join(store, "c@1.0.0")) {
			t.Fatalf("purge on a low filesystem = %d, %v, want a@1.0.0 and b@1.0.0 removed", removed, err)
		}
		// a package added after the scan is found by the next scan of the low filesystem
		writePackage("e@1.0.0", 30, 1, now.Add(-50*time.Hour))
		cacheUsages[store].scanned = now.Add(-10 * time.Minute)
		if removed, err := purgeLocalCaches(); err != nil || removed != 2 || existsDir(filepath.Join(store, "e@1.0.0")) || !existsDir(filepath.Join(store, "d@1.0.0")) {
			t.Fatalf("purge after the next scan = %d, %v, want e@1.0.0 and c@1.0.0 removed", removed, err)
		}
	})

	t.Run("no block count", func(t *testing.T) {
		config = &Config{WorkDir: t.TempDir(), NpmStoreMaxSize: "50%"}
		diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
			*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 4096 * MB / 4096}
			return nil
		}
		writePackage("a@1.0.0", 1, MB, now.Add(-30*time.Hour))
		if removed, err := purgeLocalCaches(); err != nil || removed != 0 {
			t.Fatalf("purge = %d, %v, want 0 removed", removed, err)
		}
	})

	t.Run("separate filesystems", func(t *testing.T) {
		config = &Config{WorkDir: t.TempDir(), NpmStoreMaxSize: "3MB"}
		config.Storage = storage.StorageOptions{Type: "s3", CacheDir: t.TempDir(), CacheMaxSize: "2MB"}
		var cacheUsed uint64
		storeStatfs := statfs(0, 0)
		diskStatfs = func(path string, stat *syscall.Statfs_t) error {
			if !strings.HasPrefix(path, config.Storage.CacheDir) {
				return storeStatfs(path, stat)
			}
			free := (102400*MB - cacheUsed) / 4096
			*stat = syscall.Statfs_t{Bsize: 4096, Blocks: 102400 * MB / 4096, Bfree: free, Bavail: free}
			*(*[2]int32)(unsafe.Pointer(&stat.Fsid)) = [2]int32{1, 1}
			return nil
		}
		writePackage("a@1.0.0", 1, MB, now.Add(-30*time.Hour))
		writePackage("b@1.0.0", 1, MB, now.Add(-30*time.Hour))
		if removed, err := purgeLocalCaches(); err != nil || removed != 0 {
			t.Fatalf("purge = %d, %v, want 0 removed", removed, err)
		}
		for i := range 4 {
			name := filepath.Join(config.Storage.CacheDir, strconv.Itoa(i))
			if err := os.WriteFile(name, make([]byte, MB), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(name, now.Add(-time.Duration(40-i)*time.Hour), now.Add(-time.Duration(40-i)*time.Hour)); err != nil {
				t.Fatal(err)
			}
		}
		cacheUsed += 4 * MB
		u := cacheUsages[DefaultNpmRC().StoreDir()]
		u.scanned = now.Add(-2 * time.Hour)
		cacheUsages[config.Storage.CacheDir].scanned = now.Add(-2 * time.Hour)
		// the S3 cache evictions on its own filesystem leave the store estimate as is
		if removed, err := purgeLocalCaches(); err != nil || removed != 3 || u != cacheUsages[DefaultNpmRC().StoreDir()] {
			t.Fatalf("purge = %d, %v, want 3 S3 cache files removed and no store scan", removed, err)
		}
	})
}

func TestCacheShare(t *testing.T) {
	for _, test := range []struct {
		maxSize string
		fsSize  uint64
		share   float64
		ok      bool
	}{
		{"50%", 0, 0.5, true},
		{"12.5%", 100, 0.125, true},
		{"100GB", 1000 * 1024 * MB, 0.1, true},
		{"2TB", 1024 * 1024 * MB, 1, true},
		{"", 100, 0, false},
		{"0%", 100, 0, false},
		{"101%", 100, 0, false},
		{"0GB", 100, 0, false},
		{"lots", 100, 0, false},
	} {
		share, err := cacheShare(test.maxSize, test.fsSize)
		if (err == nil) != test.ok || share != test.share {
			t.Fatalf("cacheShare(%q) = %v, %v, want %v", test.maxSize, share, err, test.share)
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
