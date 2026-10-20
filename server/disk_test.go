package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

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
		if want := []string{filepath.Join(config.WorkDir, "npm"), config.WorkDir, config.Storage.CacheDir}; !slices.Equal(paths, want) {
			t.Fatalf("checked %v, want %v", paths, want)
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
	for _, dir := range []string{filepath.Join(config.WorkDir, "npm", "old-package"), filepath.Join(config.WorkDir, "npm_old")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "old"), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "keep")
	if err := os.WriteFile(outside, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(config.Storage.CacheDir, "link")); err != nil {
		t.Fatal(err)
	}
	diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
		*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 512 * MB / 4096}
		return nil
	}
	generation := npmStoreGeneration
	if purged, err := purgeLocalCaches(0, false); err != nil || !purged {
		t.Fatalf("purge = %v, %v", purged, err)
	}
	for _, dir := range localCacheDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("cache %s is not empty: %v, %v", dir, entries, err)
		}
	}
	if _, err := os.Stat(filepath.Join(config.WorkDir, "npm_old")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old npm cache remains: %v", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep" {
		t.Fatalf("purge followed a cache symlink: %q, %v", data, err)
	}
	if after, err := os.Stat(config.Storage.CacheDir); err != nil || !os.SameFile(cacheRoot, after) {
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
	if purged, err := purgeLocalCaches(generation, true); err != nil || purged {
		t.Fatalf("repeated purge for the same failure: %v, %v", purged, err)
	}
	if _, err := os.Stat(filepath.Join(config.Storage.CacheDir, key)); err != nil {
		t.Fatalf("repeated purge removed new cache entries: %v", err)
	}
	diskStatfs = func(_ string, stat *syscall.Statfs_t) error {
		*stat = syscall.Statfs_t{Bsize: 4096, Bavail: 2048 * MB / 4096}
		return nil
	}
	if purged, err := purgeLocalCaches(0, false); err != nil || purged {
		t.Fatalf("purged a healthy filesystem: %v, %v", purged, err)
	}
}
