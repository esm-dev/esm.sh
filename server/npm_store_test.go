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
		}
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(s.dir, keys[0], "node_modules", "dep")); err != nil {
			t.Fatal(err)
		}
		// an unreadable directory before the other packages must not stop the walk, root can read it
		locked := os.Geteuid() != 0
		if err := os.Mkdir(filepath.Join(s.dir, "locked"), 0); err != nil {
			t.Fatal(err)
		}
		if err := s.restore(); (err != nil) != locked {
			t.Fatalf("restore error = %v", err)
		}
		s.mu.Lock()
		if len(s.items) != len(keys) {
			t.Fatalf("registered %d packages, want %d", len(s.items), len(keys))
		}
		expires := map[time.Time]bool{}
		for _, key := range keys {
			item := s.items[key]
			if item == nil {
				t.Fatalf("missing package key %q", key)
			}
			if ttl := time.Until(item.expires); ttl < npmPackageTTL || ttl >= 2*npmPackageTTL {
				t.Fatalf("restored TTL of %s = %v", key, ttl)
			}
			expires[item.expires] = true
		}
		s.mu.Unlock()
		if len(expires) == 1 {
			t.Fatal("restored packages expire at the same time")
		}
		time.Sleep(npmPackageTTL - time.Second)
		synctest.Wait()
		for _, key := range keys {
			if !existsDir(filepath.Join(s.dir, key)) {
				t.Fatalf("package %s expired before its new TTL", key)
			}
		}
		time.Sleep(npmPackageTTL + time.Second)
		synctest.Wait()
		for _, key := range keys {
			if existsDir(filepath.Join(s.dir, key)) {
				t.Fatalf("unused package %s remains", key)
			}
		}
		if len(s.items) != 0 {
			t.Fatalf("expired items remain: %d", len(s.items))
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

func TestNpmStoreKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := useNpmStore(t)
		for _, key := range []string{"", "@babel", "gh", "gh/owner", "pkg@1.0.0/deps-abc", "pkg@1.0.0/node_modules/dep@1.0.0", "../x@1.0.0", "./x@1.0.0", ".purge-x@1.0.0", "/x@1.0.0", "a//b@1.0.0"} {
			s.use(key, npmPackageTTL)()
		}
		if len(s.items) != 0 {
			t.Fatalf("registered keys that are not package roots: %v", s.items)
		}
		const key = "pkg@1.0.0"
		wd := filepath.Join(s.dir, key, "deps-abc")
		pkgDir := filepath.Join(s.dir, "dep@1.0.0", "node_modules", "dep")
		for _, dir := range []string{pkgDir, filepath.Join(wd, "node_modules", "plain"), filepath.Join(s.dir, "@babel", "node_modules", "x")} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Symlink(pkgDir, filepath.Join(wd, "node_modules", "dep")); err != nil {
			t.Fatal(err)
		}
		ctx, release := withNpmPackageScope(context.Background())
		defer release()
		for _, name := range []string{"plain", "missing", "../../../@babel/node_modules/x", "dep"} {
			useNpmPackageDir(ctx, filepath.Join(wd, "node_modules", name))()
		}
		if len(s.items) != 1 || s.items["dep@1.0.0"] == nil {
			t.Fatalf("dir keys = %v, want the link target only", s.items)
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
		s.use(key, npmPackageTTL)()
		time.Sleep(npmPackageTTL - time.Minute)
		release := s.use(key, npmPackageTTL)
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
			t.Fatal("package expired before its TTL")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if existsDir(dir) {
			t.Fatal("package remains after its TTL")
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
				s.use(pkg.String(), npmPackageTTL)()
				time.Sleep(npmPackageTTL - time.Minute)
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
		time.Sleep(npmPackageTTL - time.Minute)
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
