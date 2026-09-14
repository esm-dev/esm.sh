package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/esm-dev/esm.sh/internal/storage"
)

func TestPurgeRejectsUnsafePackage(t *testing.T) {
	oldWorkDir := config.WorkDir
	config.WorkDir = filepath.Join(t.TempDir(), "work")
	t.Cleanup(func() { config.WorkDir = oldWorkDir })
	fs, metaDB, logger := newPurgeTestEnv(t)
	victim := filepath.Join(filepath.Dir(config.WorkDir), "victim@abc1234")
	if err := os.MkdirAll(victim, 0755); err != nil {
		t.Fatal(err)
	}
	esm := EsmPath{PkgName: "../../../victim", PkgVersion: "abc1234", PrPrefix: true}
	if _, err := purgePackageCache(newTestNpmRC(), metaDB, fs, logger, esm, true, "", "/pr/../../../victim@abc1234"); err == nil {
		t.Error("expected an invalid package error")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("directory outside npm store was removed: %v", err)
	}
	for _, pathname := range []string{"/pr/../../../victim@abc1234", "/pkg.pr.new/../victim@abc1234", "/pr/@scope/../victim@abc1234", "/pr/pkg\\..\\victim@abc1234"} {
		if _, _, _, _, _, err := parseEsmPath(newTestNpmRC(), pathname); err == nil {
			t.Errorf("accepted unsafe path %q", pathname)
		}
	}
}

func TestPurgeAllBuildMetadata(t *testing.T) {
	oldWorkDir := config.WorkDir
	config.WorkDir = t.TempDir()
	t.Cleanup(func() { config.WorkDir = oldWorkDir })
	for _, esm := range []EsmPath{
		{PkgName: "purge-meta", PkgVersion: "1.0.0"},
		{PkgName: "@scope/purge-meta", PkgVersion: "1.0.0"},
		{PkgName: "user/purge-meta", PkgVersion: "abc1234", GhPrefix: true},
		{PkgName: "@scope/purge-meta", PkgVersion: "abc1234", PrPrefix: true},
		{PkgName: "user/repo/@scope/purge-meta", PkgVersion: "abc1234", PrPrefix: true},
	} {
		t.Run(esm.PackageId(), func(t *testing.T) {
			fs, metaDB, logger := newPurgeTestEnv(t)
			pkgId := esm.PackageId()
			paths := []string{
				"/" + pkgId + "/X-" + strings.Repeat("a", 50) + "/es2022/pkg.mjs",
				"/*" + pkgId + "/es2022/pkg.mjs",
				"/" + pkgId + "/es2022/types-only.mjs",
				"/" + pkgId + "/es2022/redirect.mjs",
			}
			for i, key := range paths {
				if i < 2 {
					if err := fs.Put(normalizeSavePath("modules"+key), strings.NewReader("module")); err != nil {
						t.Fatal(err)
					}
				}
				if err := metaDB.Put(key, encodeBuildMeta(&BuildMeta{TypesOnly: i == 2})); err != nil {
					t.Fatal(err)
				}
				build := &BuildContext{path: key, metaDB: metaDB}
				if _, ok, err := build.Exists(); err != nil || !ok {
					t.Fatalf("missing seeded metadata: %v", err)
				}
				t.Cleanup(func() { cacheLRU.Remove(key) })
			}
			sibling := "/" + pkgId + "-other/es2022/pkg.mjs"
			if err := metaDB.Put(sibling, encodeBuildMeta(&BuildMeta{})); err != nil {
				t.Fatal(err)
			}
			if _, err := purgePackageCache(newTestNpmRC(), metaDB, fs, logger, esm, true, "", "/"+pkgId); err != nil {
				t.Fatal(err)
			}
			for _, key := range paths {
				build := &BuildContext{path: key, metaDB: metaDB}
				if _, ok, err := build.Exists(); err != nil || ok {
					t.Errorf("cached metadata survived purge: %s (%v)", key, err)
				}
				if _, err := NewBuildMetaDB(fs).Get(key); !errors.Is(err, storage.ErrNotFound) {
					t.Errorf("persisted metadata survived purge: %s (%v)", key, err)
				}
			}
			if _, err := NewBuildMetaDB(fs).Get(sibling); err != nil {
				t.Fatalf("sibling metadata was removed: %v", err)
			}
		})
	}
}

func TestPurgeLegacyMetadata(t *testing.T) {
	oldWorkDir := config.WorkDir
	config.WorkDir = t.TempDir()
	t.Cleanup(func() { config.WorkDir = oldWorkDir })
	fs, metaDB, logger := newPurgeTestEnv(t)
	esm := EsmPath{PkgName: "purge-legacy", PkgVersion: "1.0.0"}
	paths := []string{
		"/purge-legacy@1.0.0/es2022/warm.mjs",
		"/*purge-legacy@1.0.0/X-" + strings.Repeat("a", 50) + "/es2022/cold.mjs",
	}
	for _, key := range paths {
		hash := sha256.Sum256([]byte(key))
		if err := fs.Put("meta/"+hex.EncodeToString(hash[:]), strings.NewReader(string(encodeBuildMeta(&BuildMeta{})))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := metaDB.Get(paths[0]); err != nil {
		t.Fatalf("legacy metadata was not reused: %v", err)
	}
	if _, err := fs.Stat(normalizeMetaStoreKey(paths[0])); err != nil {
		t.Fatalf("legacy metadata was not migrated: %v", err)
	}
	if _, err := purgePackageCache(newTestNpmRC(), metaDB, fs, logger, esm, true, "", "/purge-legacy@1.0.0"); err != nil {
		t.Fatal(err)
	}
	for _, key := range paths {
		if _, err := NewBuildMetaDB(fs).Get(key); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("legacy metadata survived purge and restart: %s (%v)", key, err)
		}
	}
	if err := metaDB.Put(paths[0], encodeBuildMeta(&BuildMeta{ExportDefault: true})); err != nil {
		t.Fatal(err)
	}
	if _, err := NewBuildMetaDB(fs).Get(paths[0]); err != nil {
		t.Fatalf("purge marker blocked new metadata: %v", err)
	}
}

func TestPurgeEmptyCacheKeys(t *testing.T) {
	body, err := json.Marshal(purgeResponse{Purged: []string{}, CacheKeys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if string(report["cacheKeys"]) != "[]" {
		t.Fatalf("missing cacheKeys array required by purge page: %s", body)
	}
}

func TestPurgeRouteRegressions(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"purge-missing","version":"1.0.0","dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"name":"purge-missing","version":"1.0.0"}}}`))
	}))
	defer registry.Close()
	oldConfig, oldNpmRC := config, defaultNpmRC
	cfg := *config
	cfg.WorkDir = t.TempDir()
	cfg.NpmRegistry = registry.URL + "/"
	cfg.PurgeAPI = PurgeAPIConfig{Enable: true}
	config, defaultNpmRC = &cfg, nil
	t.Cleanup(func() { config, defaultNpmRC = oldConfig, oldNpmRC })
	fs, _, logger := newPurgeTestEnv(t)
	handler := esmRouter(fs, logger)

	t.Run("negative cache", func(t *testing.T) {
		key := "404:purge-missing@latest"
		setCacheItem(key, "package not found", time.Minute)
		t.Cleanup(func() {
			deleteCacheItem(key)
			deleteCacheItemsWithPrefix("npm:purge-missing@")
		})
		challenge, err := newPowChallenge("purge")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(purgeRequest{URL: "purge-missing", Challenge: challenge.ID, Nonce: solvePowForTest(challenge)})
		req := httptest.NewRequest(http.MethodPost, "https://esm.sh/purge", strings.NewReader(string(body)))
		req.RemoteAddr = "192.0.2.100:1234"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != 200 {
			t.Fatalf("HTTP %d: %s", res.Code, res.Body.String())
		}
		var report purgeResponse
		if err := json.Unmarshal(res.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if report.Version != "1.0.0" || !report.ResolutionOnly || !slices.Contains(report.CacheKeys, key) {
			t.Fatalf("unexpected purge report: %+v", report)
		}
	})

	t.Run("spoofed forwarded address", func(t *testing.T) {
		t.Cleanup(func() { deleteCacheItemsWithPrefix("purge-rate:ip:") })
		for i := range 6 {
			req := httptest.NewRequest(http.MethodPost, "https://esm.sh/purge", strings.NewReader("{}"))
			req.RemoteAddr = "192.0.2.101:1234"
			req.Header.Set("X-Real-IP", "198.51.100."+string(rune('1'+i)))
			req.Header.Set("X-Forwarded-For", "198.51.100."+string(rune('1'+i)))
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			want := 400
			if i == 5 {
				want = 429
			}
			if res.Code != want {
				t.Fatalf("request %d: HTTP %d, want %d", i+1, res.Code, want)
			}
		}
	})
}
