package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
)

func TestInstallDependenciesMalformedGitURL(t *testing.T) {
	for _, version := range []string{"git+https://github.com", "git+ssh://github.com", "git://github.com", "https://pkg.pr.new"} {
		pkg := &npm.PackageJSON{Name: "example", Dependencies: map[string]string{"dep": version}}
		if err := new(NpmRC).installDependencies(t.TempDir(), pkg, false, nil); err == nil {
			t.Fatalf("installDependencies(%q) returned no error", version)
		}
	}
}

func TestInstallPackageAtomic(t *testing.T) {
	for _, source := range []string{"npm", "github", "pkg.pr.new"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%v", source, fail), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					workDir, transport := config.WorkDir, http.DefaultTransport
					config.WorkDir = t.TempDir()
					t.Cleanup(func() { config.WorkDir, http.DefaultTransport = workDir, transport })
					pkg := npm.Package{Name: "@scope/atomic-install", Version: "1.0.0"}
					switch source {
					case "github":
						pkg.Name, pkg.Github = "owner/atomic-install", true
					case "pkg.pr.new":
						pkg.PkgPrNew = true
					}
					npmrc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
					key := "npm:" + pkg.Name + "@" + pkg.Version
					setCacheItem(key, &npm.PackageJSON{Name: pkg.Name, Version: pkg.Version, Dist: npm.NpmPackageDist{Tarball: npmRegistry + "atomic-install.tgz"}}, time.Minute)
					t.Cleanup(func() { deleteCacheItem(key) })
					release := make(chan struct{})
					var requests atomic.Int32
					http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
						request := requests.Add(1)
						pr, pw := io.Pipe()
						go func() {
							defer pw.Close()
							gz := gzip.NewWriter(pw)
							tw := tar.NewWriter(gz)
							for _, file := range []struct{ name, content string }{
								{"package.json", fmt.Sprintf(`{"name":%q,"version":"1.0.0","main":"index.js"}`, pkg.Name)},
								{"index.js", "export default 42;"},
							} {
								if err := tw.WriteHeader(&tar.Header{Name: "package/" + file.name, Mode: 0644, Size: int64(len(file.content))}); err != nil {
									t.Error(err)
									return
								}
								if _, err := io.WriteString(tw, file.content); err != nil {
									t.Error(err)
									return
								}
								if file.name == "package.json" && request == 1 {
									if err := tw.Flush(); err != nil {
										t.Error(err)
										return
									}
									if err := gz.Flush(); err != nil {
										t.Error(err)
										return
									}
									<-release
									if fail {
										pw.CloseWithError(errors.New("interrupted tarball"))
										return
									}
								}
							}
							if err := tw.Close(); err != nil {
								t.Error(err)
								return
							}
							if err := gz.Close(); err != nil {
								t.Error(err)
							}
						}()
						return &http.Response{StatusCode: 200, Body: pr}, nil
					})
					first, second := make(chan error, 1), make(chan error, 1)
					go func() {
						_, err := npmrc.installPackage(pkg)
						first <- err
					}()
					synctest.Wait()
					installDir := filepath.Join(npmrc.StoreDir(), pkg.String())
					staged, err := filepath.Glob(filepath.Join(npmrc.StoreDir(), ".install-*", "node_modules", pkg.Name, "package.json"))
					if err != nil || len(staged) != 1 {
						t.Errorf("expected a staged manifest, got %v, %v", staged, err)
					}
					if existsFile(filepath.Join(installDir, "node_modules", pkg.Name, "package.json")) {
						t.Error("published package.json before extraction completed")
					}
					go func() {
						_, err := npmrc.installPackage(pkg)
						second <- err
					}()
					ctx, cancel := context.WithCancel(context.Background())
					canceled := make(chan error, 1)
					go func() {
						_, err := npmrc.installPackageContext(ctx, pkg)
						canceled <- err
					}()
					synctest.Wait()
					if len(first) != 0 || len(second) != 0 || len(canceled) != 0 {
						t.Error("installer returned before extraction completed")
					}
					cancel()
					if err := <-canceled; !errors.Is(err, context.Canceled) {
						t.Errorf("waiting installer did not cancel: %v", err)
					}
					close(release)
					if err := <-first; (err != nil) != fail {
						t.Fatalf("first installer error = %v, want failure = %v", err, fail)
					}
					if err := <-second; err != nil {
						t.Fatal(err)
					}
					if _, err := npmrc.installPackage(pkg); err != nil {
						t.Fatal(err)
					}
					wantRequests := int32(1)
					if fail {
						wantRequests++
					}
					if requests.Load() != wantRequests {
						t.Errorf("got %d downloads, want %d", requests.Load(), wantRequests)
					}
					data, err := os.ReadFile(filepath.Join(installDir, "node_modules", pkg.Name, "index.js"))
					if err != nil || string(data) != "export default 42;" {
						t.Fatalf("installed entry = %q, %v", data, err)
					}
					staged, err = filepath.Glob(filepath.Join(npmrc.StoreDir(), ".install-*"))
					if err != nil || len(staged) != 0 {
						t.Fatalf("staging directories remain: %v, %v", staged, err)
					}
				})
			})
		}
	}
}

func TestInstallDependenciesAliases(t *testing.T) {
	workDir := config.WorkDir
	config.WorkDir = t.TempDir()
	t.Cleanup(func() { config.WorkDir = workDir })
	npmrc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
	pkg := npm.Package{Name: "@scope/alias-target", Version: "1.0.0"}
	installDir := filepath.Join(npmrc.StoreDir(), pkg.String(), "node_modules", pkg.Name)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "package.json"), []byte(`{"name":"@scope/alias-target","version":"1.0.0","dependencies":{"self":"npm:@scope/alias-target@1.0.0"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	dependencies := map[string]string{pkg.Name: pkg.Version}
	for i := range 100 {
		name := fmt.Sprintf("alias-%d", i)
		if i%2 == 0 {
			name = "@scope/" + name
		}
		dependencies[name] = "npm:" + pkg.String()
	}
	for _, npmMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("npmMode=%v", npmMode), func(t *testing.T) {
			wd := t.TempDir()
			if err := os.Mkdir(filepath.Join(wd, "node_modules"), 0755); err != nil {
				t.Fatal(err)
			}
			root := &npm.PackageJSON{Name: "root", Version: "1.0.0"}
			if npmMode {
				root.PeerDependencies = dependencies
			} else {
				root.Dependencies = dependencies
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := npmrc.installDependenciesContext(ctx, wd, root, npmMode, nil); err != nil {
				t.Fatal(err)
			}
			linked := 0
			for name := range dependencies {
				if target, err := os.Readlink(filepath.Join(wd, "node_modules", name)); err == nil && target == installDir {
					linked++
				}
			}
			if linked != len(dependencies) {
				t.Errorf("linked %d dependencies, want %d", linked, len(dependencies))
			}
			if target, err := os.Readlink(filepath.Join(wd, "node_modules", "self")); err != nil || target != installDir {
				t.Errorf("recursive alias target = %q, %v", target, err)
			}
		})
	}
}

func TestInstallLockCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key := t.Name()
		unlock, err := lockInstall(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		canceled := make(chan error, 1)
		go func() {
			release, err := lockInstall(ctx, key)
			if err == nil {
				release()
			}
			canceled <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-canceled; !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting lock did not cancel: %v", err)
		}
		acquired := make(chan func(), 2)
		for range 2 {
			go func() {
				release, err := lockInstall(context.Background(), key)
				if err != nil {
					t.Error(err)
					return
				}
				acquired <- release
			}()
		}
		synctest.Wait()
		if len(acquired) != 0 {
			t.Fatal("canceled waiter released the owner's lock")
		}
		other, err := lockInstall(context.Background(), key+"/other")
		if err != nil {
			t.Fatal(err)
		}
		other()
		unlock()
		synctest.Wait()
		if len(acquired) != 1 {
			t.Fatalf("acquired %d locks, want one", len(acquired))
		}
		(<-acquired)()
		(<-acquired)()
		if _, ok := installLocks.Load(key); ok {
			t.Fatal("released lock was retained")
		}
	})
}

func TestNpmTooLargeVersion(t *testing.T) {
	useNegativeCache(t)
	workDir, transport := config.WorkDir, http.DefaultTransport
	config.WorkDir = t.TempDir()
	t.Cleanup(func() { config.WorkDir, http.DefaultTransport = workDir, transport })
	reg := &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}
	npmrc := &NpmRC{globalRegistry: reg}
	const name = "negative-size-test"
	for _, version := range []string{"1.0.0", "2.0.0"} {
		key := "npm:" + name + "@" + version
		setCacheItem(key, &npm.PackageJSON{Name: name, Version: version, Dist: npm.NpmPackageDist{Tarball: "https://registry.npmjs.org/" + version + ".tgz"}}, time.Minute)
		t.Cleanup(func() { deleteCacheItem(key) })
	}
	requests := 0
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, ContentLength: maxPackageTarballSize + 1, Body: http.NoBody}, nil
	})
	for _, version := range []string{"1.0.0", "1.0.0", "2.0.0"} {
		_, err := npmrc.installPackage(npm.Package{Name: name, Version: version})
		if !errors.Is(err, errPackageTooLarge) {
			t.Fatalf("%s: expected size error, got %v", version, err)
		}
		if existsDir(filepath.Join(npmrc.StoreDir(), name+"@"+version)) {
			t.Fatal("oversized package left an installation")
		}
	}
	if requests != 2 {
		t.Fatalf("expected one request per version, got %d", requests)
	}
	if negativeCache.get("npm-too-large:"+npmRegistry+name+"@latest") != "" {
		t.Fatal("stored a floating version")
	}
}

func TestPackageNotFoundCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		useNegativeCache(t)
		transport := http.DefaultTransport
		t.Cleanup(func() { http.DefaultTransport = transport })
		for _, test := range []struct{ name, version string }{{"negative-package-test", "latest"}, {"negative-version-test", "1.0.0"}, {"negative-range-test", "^9"}} {
			defer deleteCacheItemsWithPrefix("npm:" + test.name + "@")
			reg := &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}
			npmrc := &NpmRC{globalRegistry: reg}
			requests := 0
			http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if requests == 1 && test.version != "^9" {
					return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
				}
				body := `{"name":"` + test.name + `","version":"1.0.0"}`
				if test.version == "^9" {
					body = `{"versions":{"1.0.0":{"version":"1.0.0"}}}`
					if requests > 1 {
						body = `{"versions":{"9.0.0":{"version":"9.0.0"}}}`
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			for range 2 {
				if _, err := npmrc.getPackageInfo(test.name, test.version); err == nil {
					t.Fatal("expected a missing package/version")
				}
			}
			time.Sleep(packageNotFoundTTL - time.Second)
			if _, err := npmrc.getPackageInfo(test.name, test.version); err == nil || requests != 1 {
				t.Fatalf("negative cache missed: %v, requests=%d", err, requests)
			}
			time.Sleep(time.Second)
			if _, err := npmrc.getPackageInfo(test.name, test.version); err != nil || requests != 2 {
				t.Fatalf("expired miss did not recover: %v, requests=%d", err, requests)
			}
		}
	})
}

func TestResolveSemverVersion(t *testing.T) {
	for _, test := range []struct {
		name    string
		version string
		tags    map[string]string
		want    string
	}{
		{"missing latest", "latest", nil, ""},
		{"unknown tag without latest", "unknown", nil, ""},
		{"dangling latest", "latest", map[string]string{"latest": "3.0.0"}, ""},
		{"latest", "latest", map[string]string{"latest": "1.2.0"}, "1.2.0"},
		{"unknown tag falls back to latest", "unknown", map[string]string{"latest": "1.2.0"}, "1.2.0"},
		{"named tag", "next", map[string]string{"next": "2.0.0-beta.1"}, "2.0.0-beta.1"},
		{"highest matching version", "^1", nil, "1.10.0"},
		{"stable wildcard", "*", nil, "1.10.0"},
		{"no matching version", "^3", nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			metadata := &npm.PackageMetadata{
				DistTags: test.tags,
				Versions: map[string]npm.PackageJSONRaw{
					"1.2.0":        {Version: "1.2.0"},
					"1.10.0":       {Version: "1.10.0"},
					"2.0.0-beta.1": {Version: "2.0.0-beta.1"},
				},
			}
			got, err := resolveSemverVersion(metadata, test.version)
			if got != test.want || (err != nil) != (test.want == "") {
				t.Fatalf("resolveSemverVersion(%q) = %q, %v; want %q", test.version, got, err, test.want)
			}
		})
	}
}

func TestResolveSemverOriginalVersion(t *testing.T) {
	metadata := &npm.PackageMetadata{Versions: map[string]npm.PackageJSONRaw{
		"v1.2.0": {Version: "v1.2.0"},
	}}
	got, err := resolveSemverVersion(metadata, "^1")
	if err != nil || got != "v1.2.0" {
		t.Fatalf("resolved version = %q, %v; want the original metadata key", got, err)
	}
}

func BenchmarkResolveSemverVersion(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			metadata := &npm.PackageMetadata{Versions: make(map[string]npm.PackageJSONRaw, count)}
			for i := range count {
				version := fmt.Sprintf("1.%d.0", i)
				metadata.Versions[version] = npm.PackageJSONRaw{Version: version}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := resolveSemverVersion(metadata, "^1"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestInstallDependenciesSkipsTypes(t *testing.T) {
	transport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = transport })
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected request for a types-only dependency: %s", r.URL)
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
	})
	npmrc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
	err := npmrc.installDependencies(t.TempDir(), &npm.PackageJSON{
		Name: "test", Version: "1.0.0",
		Dependencies: map[string]string{"@types/test": "1.0.0", "types-alias": "npm:@types/test@1.0.0"},
	}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestInstallGithubDenoConfig(t *testing.T) {
	workDir, transport := config.WorkDir, http.DefaultTransport
	config.WorkDir = t.TempDir()
	t.Cleanup(func() { config.WorkDir, http.DefaultTransport = workDir, transport })
	for _, test := range []struct {
		name, filename, content string
	}{
		{"escaped strings", "deno.json", `{"imports":{"quote\"name":"./quote\"file.ts"},"exports":{".":"./a\\b.ts"}}`},
		{"non-string entries", "deno.json", `{"imports":{"ignored":{"default":"./index.ts"}},"exports":{".":{"default":"./index.ts"}}}`},
		{"comments", "deno.jsonc", "{\n// comment\n\"exports\":{\".\":\"./index.ts\"}}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			if err := tw.WriteHeader(&tar.Header{Name: "repo/" + test.filename, Mode: 0644, Size: int64(len(test.content))}); err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(tw, test.content); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(archive.Bytes()))}, nil
			})
			pkg := npm.Package{Github: true, Name: "owner/" + strings.ReplaceAll(test.name, " ", "-"), Version: "abcdef0"}
			info, err := new(NpmRC).installPackage(pkg)
			if err != nil {
				t.Fatal(err)
			}
			if info.Name != pkg.Name || info.Version != pkg.Version {
				t.Fatalf("incorrect generated package identity: %s@%s", info.Name, info.Version)
			}
			switch test.name {
			case "escaped strings":
				if got, _ := info.Imports.Get(`quote"name`); got != `./quote"file.ts` {
					t.Fatalf("import = %q", got)
				}
				if got, _ := info.Exports.Get("."); got != `./a\b.ts` {
					t.Fatalf("export = %q", got)
				}
			case "non-string entries":
				if info.Imports.Len() != 0 || info.Exports.Len() != 0 {
					t.Fatal("non-string entries should be omitted")
				}
			case "comments":
				if got, _ := info.Exports.Get("."); got != "./index.ts" {
					t.Fatalf("export = %q", got)
				}
			}
		})
	}
}

func TestInvalidateDistTagCacheIfNewer(t *testing.T) {
	tests := []struct {
		request string
		invalid bool
	}{
		{"1.2.0", false},  // equal to the cached `latest`
		{"1.1.0", false},  // older
		{"2.0.0", true},   // newer
		{"latest", false}, // non-exact
		{"v2.0.0", true},  // v-prefixed newer
	}
	for _, test := range tests {
		setCacheItem("npm:cache-test@latest", &npm.PackageJSON{Version: "1.2.0"}, time.Minute)
		setCacheItem("404:cache-test@latest", "boom", time.Minute)
		invalidateDistTagCacheIfNewer("cache-test", test.request)
		_, ok := getCacheItem("npm:cache-test@latest")
		if invalid := !ok; invalid != test.invalid {
			t.Fatalf("request %q: expected invalidated=%v, got %v", test.request, test.invalid, invalid)
		}
	}
}

func TestSameURLOrigin(t *testing.T) {
	registryUrl, _ := url.Parse("https://registry.example/package")
	for _, test := range []struct {
		url  string
		want bool
	}{
		{"https://registry.example/tarball.tgz", true},
		{"https://REGISTRY.EXAMPLE:443/tarball.tgz", true},
		{"http://registry.example/tarball.tgz", false},
		{"https://registry.example:444/tarball.tgz", false},
		{"https://tarballs.example/tarball.tgz", false},
	} {
		tarballUrl, _ := url.Parse(test.url)
		if got := sameURLOrigin(registryUrl, tarballUrl); got != test.want {
			t.Errorf("sameURLOrigin(%q, %q) = %v, want %v", registryUrl, tarballUrl, got, test.want)
		}
	}
}

func TestFetchPackageTarballAuthorization(t *testing.T) {
	var tarball bytes.Buffer
	gw := gzip.NewWriter(&tarball)
	tw := tar.NewWriter(gw)
	content := []byte(`{"name":"test-package","version":"1.0.0"}`)
	if err := tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	authorization := make(chan string, 1)
	tarballServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		_, _ = w.Write(tarball.Bytes())
	}))
	defer tarballServer.Close()

	basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:password"))
	for _, test := range []struct {
		name     string
		registry NpmRegistryConfig
		want     string
	}{
		{"same-origin bearer token", NpmRegistryConfig{Registry: tarballServer.URL + "/", Token: "secret"}, "Bearer secret"},
		{"cross-origin bearer token", NpmRegistryConfig{Registry: "https://registry.example/", Token: "secret"}, ""},
		{"same-origin basic auth", NpmRegistryConfig{Registry: tarballServer.URL + "/", User: "user", Password: "password"}, basicAuth},
		{"cross-origin basic auth", NpmRegistryConfig{Registry: "https://registry.example/", User: "user", Password: "password"}, ""},
		{"backup registry", NpmRegistryConfig{Registry: "https://registry.example/", BackupRegistry: tarballServer.URL + "/", Token: "secret"}, "Bearer secret"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reg := &NpmRegistry{NpmRegistryConfig: test.registry}
			if err := fetchPackageTarballContext(context.Background(), reg, t.TempDir(), "test-package", tarballServer.URL+"/test-package.tgz"); err != nil {
				t.Fatal(err)
			}
			if got := <-authorization; got != test.want {
				t.Fatalf("expected Authorization header %q, got %q", test.want, got)
			}
		})
	}

	redirectAuthorization := make(chan string, 1)
	crossOriginServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectAuthorization <- r.Header.Get("Authorization")
		_, _ = w.Write(tarball.Bytes())
	}))
	defer crossOriginServer.Close()
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, crossOriginServer.URL+"/test-package.tgz", http.StatusFound)
	}))
	defer registryServer.Close()

	reg := &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: registryServer.URL + "/", Token: "secret"}}
	if err := fetchPackageTarballContext(context.Background(), reg, t.TempDir(), "test-package", registryServer.URL+"/test-package.tgz"); err != nil {
		t.Fatal(err)
	}
	if got := <-redirectAuthorization; got != "" {
		t.Fatalf("expected redirect to strip Authorization header, got %q", got)
	}
}

func TestFetchPackageTarballBackup(t *testing.T) {
	var tarball bytes.Buffer
	gw := gzip.NewWriter(&tarball)
	tw := tar.NewWriter(gw)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name           string
		primaryStatus  int
		backupStatus   int
		rateLimited    bool
		redirectBackup bool
		wantRequests   []string
		wantErr        bool
	}{
		{"primary succeeds", 200, 200, false, false, []string{"primary Bearer secret"}, false},
		{"first rate limit", 429, 200, false, false, []string{"primary Bearer secret", "backup Bearer secret"}, false},
		{"already rate limited", 429, 200, true, false, []string{"backup Bearer secret"}, false},
		{"backup rate limited", 429, 429, false, false, []string{"primary Bearer secret", "backup Bearer secret"}, true},
		{"backup redirects to untrusted origin", 429, 200, false, true, []string{"primary Bearer secret", "backup Bearer secret", "external "}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan string, 16)
			external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- "external " + r.Header.Get("Authorization")
				_, _ = w.Write(tarball.Bytes())
			}))
			defer external.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- "backup " + r.Header.Get("Authorization")
				if r.URL.RequestURI() != "/test-package.tgz?download=1" {
					t.Errorf("backup request lost path or query: %s", r.URL)
				}
				if test.redirectBackup {
					http.Redirect(w, r, external.URL+"/test-package.tgz", http.StatusFound)
					return
				}
				w.WriteHeader(test.backupStatus)
				_, _ = w.Write(tarball.Bytes())
			}))
			defer backup.Close()
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- "primary " + r.Header.Get("Authorization")
				w.WriteHeader(test.primaryStatus)
				_, _ = w.Write(tarball.Bytes())
			}))
			defer primary.Close()
			reg := &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{
				Registry: primary.URL + "/", BackupRegistry: backup.URL + "/", Token: "secret",
			}}
			if test.rateLimited {
				reg.rateLimited.Store(1)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := fetchPackageTarballContext(ctx, reg, t.TempDir(), "test-package", primary.URL+"/test-package.tgz?download=1")
			if (err != nil) != test.wantErr {
				t.Fatalf("fetchPackageTarballContext() = %v; want error: %v", err, test.wantErr)
			}
			if test.primaryStatus == 429 && !reg.isRateLimited() {
				t.Fatal("registry rate limit was not recorded")
			}
			got := make([]string, 0, len(requests))
			for len(requests) > 0 {
				got = append(got, <-requests)
			}
			if !slices.Equal(got, test.wantRequests) {
				t.Fatalf("requests = %q; want %q", got, test.wantRequests)
			}
		})
	}
}

func TestExtractPackageTarball(t *testing.T) {
	b := make([]byte, 16)
	rand.Read(b)
	installDir := filepath.Join(os.TempDir(), hex.EncodeToString(b))
	defer os.RemoveAll(installDir)

	// Create a malicious tarball with path traversal
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add a normal file
	content := []byte("export const foo = 'bar';")
	header := &tar.Header{
		Name:     "package/index.js",
		Mode:     0644,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	// Add a large file
	largeContent := make([]byte, 1024*1024*51)
	rand.Read(largeContent)
	header = &tar.Header{
		Name:     "package/large.txt",
		Mode:     0644,
		Size:     int64(len(largeContent)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(largeContent); err != nil {
		t.Fatal(err)
	}

	// add a link
	header = &tar.Header{
		Name:     "package/passwd.txt",
		Mode:     0644,
		Typeflag: tar.TypeLink,
		Linkname: "/etc/passwd",
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}

	// Add a malicious file with path traversal
	bad := []byte("bad")
	header = &tar.Header{
		Name:     "/../../../bad/bad.txt",
		Mode:     0644,
		Size:     int64(len(bad)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(bad); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()

	// Call extractPackageTarball with the malicious tarball
	if err := extractPackageTarball(installDir, "test-package", bytes.NewReader(buf.Bytes())); err != nil {
		t.Errorf("extractPackageTarball returned error: %v", err)
	}
	if !existsFile(filepath.Join(installDir, "node_modules", "test-package", "index.js")) {
		t.Fatal("index.js should be extracted")
	}
	if existsFile(filepath.Join(installDir, "node_modules", "test-package", "large.txt")) {
		t.Fatal("large.txt should not be extracted")
	}
	if existsFile(filepath.Join(installDir, "node_modules", "test-package", "passwd.txt")) {
		t.Fatal("passwd.txt should not be extracted")
	}
	if !existsFile(filepath.Join(installDir, "node_modules", "test-package", "bad.txt")) {
		t.Fatal("bad.txt should be extracted in the root directory")
	}
}

func TestExtractPackageTarballRejectsEscapingPackageName(t *testing.T) {
	if err := extractPackageTarball(t.TempDir(), "../escape", bytes.NewReader(nil)); err == nil {
		t.Fatal("expected an invalid package name error")
	}
}

func TestExtractPackageTarballWillNotWriteThroughSymlink(t *testing.T) {
	installDir := t.TempDir()
	destination := t.TempDir()
	if err := os.Mkdir(filepath.Join(installDir, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destination, filepath.Join(installDir, "node_modules", "test-package")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	content := []byte("export const escaped = true")
	if err := tw.WriteHeader(&tar.Header{Name: "package/index.js", Mode: 0644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := extractPackageTarball(installDir, "test-package", bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("expected extraction through a symlink to fail")
	}
	if existsFile(filepath.Join(destination, "index.js")) {
		t.Fatal("tarball escaped the extraction root")
	}
}
