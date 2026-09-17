package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
)

type dtsTestStorage struct {
	storage.Storage
	mu           sync.Mutex
	active, peak int
	puts         map[string]int
	delay        time.Duration
	fail         string
	cancel       context.CancelFunc
}

func (s *dtsTestStorage) Stat(key string) (storage.Stat, error) {
	s.mu.Lock()
	s.active++
	s.peak = max(s.peak, s.active)
	s.mu.Unlock()
	time.Sleep(s.delay)
	stat, err := s.Storage.Stat(key)
	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	return stat, err
}

func (s *dtsTestStorage) Put(key string, content io.Reader) error {
	s.mu.Lock()
	s.active++
	s.peak = max(s.peak, s.active)
	s.puts[path.Base(key)]++
	s.mu.Unlock()
	time.Sleep(s.delay)
	var err error
	if path.Base(key) == s.fail {
		err = errors.New("upload failed")
	} else {
		if strings.HasPrefix(path.Base(key), "child") {
			if _, err = s.Storage.Stat(path.Join(path.Dir(key), "shared.d.ts")); err != nil {
				err = fmt.Errorf("published dependent before shared types: %w", err)
			}
		}
		if err == nil {
			err = s.Storage.Put(key, content)
		}
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	return err
}

func TestTransformDTSConcurrent(t *testing.T) {
	for _, mode := range []string{"success", "upload failure", "canceled", "cancel during upload"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			pkgDir := filepath.Join(root, "node_modules", "pkg")
			if err := os.MkdirAll(pkgDir, 0755); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{
				"shared.d.ts": `export * from "./cycle.d.ts";`,
				"cycle.d.ts":  `export * from "./shared.d.ts";`,
			}
			var entry strings.Builder
			entry.WriteString("export * from \"./missing.d.ts\";\n")
			for i := range 64 {
				name := fmt.Sprintf("child%d.d.ts", i)
				fmt.Fprintf(&entry, "export * from %q;\n", "./"+name)
				files[name] = `export * from "./shared.d.ts";`
			}
			files["index.d.ts"] = entry.String()
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			fs, err := storage.NewFSStorage(filepath.Join(root, "storage"))
			if err != nil {
				t.Fatal(err)
			}
			synctest.Test(t, func(t *testing.T) {
				buildCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s := &dtsTestStorage{Storage: fs, puts: map[string]int{}, delay: time.Second}
				switch mode {
				case "upload failure":
					s.fail = "shared.d.ts"
				case "canceled":
					cancel()
				case "cancel during upload":
					s.cancel = cancel
				}
				ctx := &BuildContext{ctx: buildCtx, storage: s, wd: root, esmPath: EsmPath{PkgName: "pkg", PkgVersion: "1.0.0"}, pkgJson: &npm.PackageJSON{}}
				start := time.Now()
				n, err := transformDTS(ctx, "./index.d.ts", "", nil)
				elapsed := time.Since(start)
				if mode == "success" {
					if err != nil || n != len(files)-1 {
						t.Fatalf("transform = %d, %v", n, err)
					}
					if elapsed > 25*time.Second {
						t.Errorf("serial storage I/O: %v for %d files", elapsed, len(files))
					}
					if s.peak < 2 || s.peak > 16 {
						t.Errorf("storage concurrency = %d, want 2..16", s.peak)
					}
					for name := range files {
						if s.puts[name] != 1 {
							t.Errorf("%s uploaded %d times", name, s.puts[name])
						}
					}
					if n, err := transformDTS(ctx, "./index.d.ts", "", nil); err != nil || n != 0 {
						t.Fatalf("cached transform = %d, %v", n, err)
					}
					for name, n := range s.puts {
						if n != 1 {
							t.Errorf("cached transform uploaded %s again", name)
						}
					}
				} else {
					if err == nil {
						t.Fatal("expected transform failure")
					}
					if strings.Contains(mode, "cancel") && !errors.Is(err, context.Canceled) {
						t.Errorf("expected cancellation, got %v", err)
					}
					if s.puts["index.d.ts"] != 0 {
						t.Error("published entry after failure")
					}
					if mode == "upload failure" {
						for name := range s.puts {
							if strings.HasPrefix(name, "child") {
								t.Errorf("published %s before shared dependency succeeded", name)
							}
						}
					}
				}
				if s.active != 0 {
					t.Errorf("returned with %d storage operations running", s.active)
				}
			})
		})
	}
}

func TestTransformDTSStorageTimeout(t *testing.T) {
	for _, method := range []string{http.MethodHead, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			root := t.TempDir()
			pkgDir := filepath.Join(root, "node_modules", "pkg")
			if err := os.MkdirAll(pkgDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(pkgDir, "index.d.ts"), []byte("export {};"), 0644); err != nil {
				t.Fatal(err)
			}
			s, err := storage.NewS3Storage(&storage.StorageOptions{Endpoint: "https://storage.test", AccessKeyID: "test", SecretAccessKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			synctest.Test(t, func(t *testing.T) {
				client := http.DefaultClient
				defer func() { http.DefaultClient = client }()
				http.DefaultClient = &http.Client{Transport: ghTestTransport(func(req *http.Request) (*http.Response, error) {
					if req.Method == method {
						<-req.Context().Done()
						return nil, req.Context().Err()
					}
					return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
				})}
				queue := NewBuildQueue(1, time.Second)
				ctx := &BuildContext{storage: s, wd: root, target: "types", esmPath: EsmPath{PkgName: "pkg", PkgVersion: "1.0.0", SubPath: "index.d.ts"}, pkgJson: &npm.PackageJSON{}}
				start := time.Now()
				_, err := queue.Build(context.Background(), ctx)
				if err == nil || err.Error() != "build timeout after 1 seconds" {
					t.Fatalf("expected build timeout, got %v", err)
				}
				if elapsed := time.Since(start); elapsed != time.Second {
					t.Fatalf("storage delayed build timeout: %v", elapsed)
				}
				if len(queue.Snapshot()) != 0 {
					t.Fatal("timed out declaration still occupies the build queue")
				}
			})
		})
	}
}

func TestTransformDTS(t *testing.T) {
	t.Run("does not publish a parent when a dependency fails", func(t *testing.T) {
		root := t.TempDir()
		pkgDir := filepath.Join(root, "node_modules", "pkg")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "index.d.ts"), []byte(`export * from "./child.d.ts";`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "child.d.ts"), []byte(strings.Repeat("x", 1024*1024+1)), 0644); err != nil {
			t.Fatal(err)
		}
		fs, err := storage.NewFSStorage(filepath.Join(root, "storage"))
		if err != nil {
			t.Fatal(err)
		}
		ctx := &BuildContext{
			storage: fs,
			esmPath: EsmPath{PkgName: "pkg", PkgVersion: "1.0.0"},
			wd:      root,
			pkgJson: &npm.PackageJSON{},
		}
		if _, err := transformDTS(ctx, "./index.d.ts", "", nil); err == nil {
			t.Fatal("expected dependency transform to fail")
		}
		savePath := normalizeSavePath(path.Join("types", "/"+ctx.esmPath.PackageId(), "./index.d.ts"))
		if _, err := fs.Stat(savePath); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("parent was published after dependency failure: %v", err)
		}
	})

	t.Run("preserves remote specifiers and counts dependencies", func(t *testing.T) {
		root := t.TempDir()
		pkgDir := filepath.Join(root, "node_modules", "pkg")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		const entry = `export * from "./child.d.ts";
export * from "https://example.com/types.d.ts";
export * from "npm:foo@1.2.3/sub";
`
		if err := os.WriteFile(filepath.Join(pkgDir, "index.d.ts"), []byte(entry), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "child.d.ts"), []byte("export interface Child {}"), 0644); err != nil {
			t.Fatal(err)
		}
		fs, err := storage.NewFSStorage(filepath.Join(root, "storage"))
		if err != nil {
			t.Fatal(err)
		}
		ctx := &BuildContext{
			storage:     fs,
			esmPath:     EsmPath{PkgName: "pkg", PkgVersion: "1.0.0"},
			wd:          root,
			pkgJson:     &npm.PackageJSON{},
			externalAll: true,
		}
		n, err := transformDTS(ctx, "./index.d.ts", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("expected one related dts file, got %d", n)
		}

		savePath := normalizeSavePath(path.Join("types", "/"+ctx.esmPath.PackageId(), "./index.d.ts"))
		content, _, err := fs.Get(savePath)
		if err != nil {
			t.Fatal(err)
		}
		defer content.Close()
		data, err := io.ReadAll(content)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != entry {
			t.Fatalf("transformed dts not match, want:\n%s\ngot:\n%s", entry, data)
		}
	})
}
