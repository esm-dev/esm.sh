package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func useNpmStore(t *testing.T) *npmPackageStore {
	t.Helper()
	oldConfig, oldStore := config, npmStore
	cfg := *config
	cfg.WorkDir = t.TempDir()
	config = &cfg
	s := &npmPackageStore{dir: filepath.Join(cfg.WorkDir, "npm")}
	npmStore = s
	t.Cleanup(func() {
		s.mu.Lock()
		for _, item := range s.items {
			if item.timer != nil {
				item.timer.Stop()
			}
		}
		clear(s.items)
		s.mu.Unlock()
		config, npmStore = oldConfig, oldStore
	})
	return s
}

func TestNpmStoreRestore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := useNpmStore(t)
		keys := []string{"plain@1.0.0", "@scope/pkg@1.0.0", "gh/owner/repo@abc", "pr/@scope/pkg@abc"}
		for _, key := range append(keys, ".install-123", ".loaders") {
			if err := os.MkdirAll(filepath.Join(s.dir, key, "node_modules", "nested@1.0.0"), 0755); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-24 * time.Hour)
			if err := os.Chtimes(filepath.Join(s.dir, key), old, old); err != nil {
				t.Fatal(err)
			}
		}
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(s.dir, keys[0], "node_modules", "dep")); err != nil {
			t.Fatal(err)
		}
		if err := s.restore(); err != nil {
			t.Fatal(err)
		}
		if len(s.items) != len(keys) {
			t.Fatalf("registered %d packages, want %d", len(s.items), len(keys))
		}
		for _, key := range keys {
			if s.items[key] == nil {
				t.Fatalf("missing package key %q", key)
			}
		}
		time.Sleep(9 * time.Minute)
		for _, key := range keys {
			if !existsDir(filepath.Join(s.dir, key)) {
				t.Fatalf("old package %s expired before its new TTL", key)
			}
		}
		s.use(keys[0])()
		time.Sleep(time.Minute)
		synctest.Wait()
		for _, key := range keys[1:] {
			if existsDir(filepath.Join(s.dir, key)) {
				t.Fatalf("unused package %s remains", key)
			}
		}
		if !existsDir(filepath.Join(s.dir, keys[0])) {
			t.Fatal("used package expired before its refreshed TTL")
		}
		time.Sleep(9 * time.Minute)
		synctest.Wait()
		if existsDir(filepath.Join(s.dir, keys[0])) || len(s.items) != 0 {
			t.Fatal("refreshed package did not expire")
		}
		for _, dir := range []string{outside, filepath.Join(s.dir, ".install-123"), filepath.Join(s.dir, ".loaders")} {
			if !existsDir(dir) {
				t.Fatalf("removed unrelated directory %s", dir)
			}
		}
		if dirs, _ := filepath.Glob(filepath.Join(s.dir, ".purge-*")); len(dirs) != 0 {
			t.Fatalf("purge leftovers: %v", dirs)
		}
	})
}

func TestNpmStoreActiveUse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := useNpmStore(t)
		const key = "active@1.0.0"
		dir := filepath.Join(s.dir, key)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		s.use(key)()
		time.Sleep(9 * time.Minute)
		release := s.use(key)
		ctx, releaseScope := withNpmPackageScope(context.Background())
		useNpmPackage(ctx, key)()
		useNpmPackage(ctx, key)()
		release()
		time.Sleep(2 * npmPackageTTL)
		synctest.Wait()
		if !existsDir(dir) {
			t.Fatal("removed a package used by a running build")
		}
		releaseScope()
		time.Sleep(npmPackageTTL - time.Second)
		synctest.Wait()
		if !existsDir(dir) {
			t.Fatal("package expired before ten idle minutes")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if existsDir(dir) {
			t.Fatal("package remains after ten idle minutes")
		}
	})
}

func TestNpmStoreAccess(t *testing.T) {
	for _, access := range []string{"install", "metadata", "raw", "css module"} {
		t.Run(access, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := useNpmStore(t)
				useNegativeCache(t)
				pkg := npm.Package{Name: "@scope/ttl-test", Version: "1.0.0"}
				dir := filepath.Join(s.dir, pkg.String(), "node_modules", pkg.Name)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				for name, data := range map[string]string{
					"package.json": `{"name":"@scope/ttl-test","version":"1.0.0"}`,
					"style.css":    "body { color: red }",
				} {
					if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
						t.Fatal(err)
					}
				}
				s.use(pkg.String())()
				time.Sleep(9 * time.Minute)
				npmrc := newTestNpmRC()
				switch access {
				case "install":
					if _, err := npmrc.installPackage(pkg); err != nil {
						t.Fatal(err)
					}
				case "metadata":
					if _, err := npmrc.getPackageInfo(pkg.Name, pkg.Version); err != nil {
						t.Fatal(err)
					}
				default:
					fs, err := storage.NewFSStorage(filepath.Join(config.WorkDir, "storage"))
					if err != nil {
						t.Fatal(err)
					}
					logger := new(log.Logger)
					logger.SetOutput(io.Discard)
					handler := esmRouter(fs, logger)
					url := "http://localhost/" + pkg.String() + "/style.css"
					if access == "css module" {
						url += "?module"
					}
					res := httptest.NewRecorder()
					handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, url, nil))
					if res.Code != 200 {
						t.Fatalf("request = %d: %s", res.Code, res.Body.String())
					}
				}
				time.Sleep(npmPackageTTL - time.Second)
				synctest.Wait()
				if !existsDir(dir) {
					t.Fatal("access did not refresh TTL")
				}
				time.Sleep(time.Second)
				synctest.Wait()
				if existsDir(dir) {
					t.Fatal("package did not expire after access")
				}
			})
		})
	}
}

func TestNpmStoreInstall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := useNpmStore(t)
		pkg := npm.Package{Name: "installed-ttl", Version: "1.0.0"}
		key := "npm:" + pkg.String()
		setCacheItem(key, &npm.PackageJSON{Name: pkg.Name, Version: pkg.Version, Dist: npm.NpmPackageDist{Tarball: npmRegistry + "installed-ttl.tgz"}}, time.Hour)
		transport := http.DefaultTransport
		t.Cleanup(func() {
			http.DefaultTransport = transport
			deleteCacheItem(key)
		})
		requests := 0
		http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(testPackageTarball(`{"name":"installed-ttl","version":"1.0.0"}`)))}, nil
		})
		npmrc := newTestNpmRC()
		wd := t.TempDir()
		if err := os.Mkdir(filepath.Join(wd, "node_modules"), 0755); err != nil {
			t.Fatal(err)
		}
		ctx, release := withNpmPackageScope(context.Background())
		if err := npmrc.installDependenciesContext(ctx, wd, &npm.PackageJSON{Dependencies: map[string]string{"alias": "npm:" + pkg.String()}}, false, nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * npmPackageTTL)
		synctest.Wait()
		if !existsDir(filepath.Join(s.dir, pkg.String())) {
			t.Fatal("dependency expired during a build")
		}
		release()
		time.Sleep(9 * time.Minute)
		ctx, release = withNpmPackageScope(context.Background())
		build := &BuildContext{ctx: ctx, wd: wd, npmrc: npmrc}
		if _, _, err := build.resolveDependency("alias", false); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * npmPackageTTL)
		synctest.Wait()
		if !existsDir(filepath.Join(s.dir, pkg.String())) || len(s.items) != 1 {
			t.Fatal("linked dependency was not kept under its package key")
		}
		release()
		time.Sleep(npmPackageTTL)
		synctest.Wait()
		if existsDir(filepath.Join(s.dir, pkg.String())) {
			t.Fatal("installed dependency did not expire")
		}
		setCacheItem(key, &npm.PackageJSON{Name: pkg.Name, Version: pkg.Version, Dist: npm.NpmPackageDist{Tarball: npmRegistry + "installed-ttl.tgz"}}, time.Hour)
		if _, err := npmrc.installPackage(pkg); err != nil {
			t.Fatal(err)
		}
		if requests != 2 || !existsDir(filepath.Join(s.dir, pkg.String())) {
			t.Fatalf("package was not reinstalled after expiry: %d downloads", requests)
		}
		time.Sleep(npmPackageTTL)
		synctest.Wait()
		if existsDir(filepath.Join(s.dir, pkg.String())) {
			t.Fatal("reinstalled package did not expire")
		}
	})
}
