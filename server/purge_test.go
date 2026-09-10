package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestParsePurgeInput(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"react", "/react"},
		{"react@19.0.0", "/react@19.0.0"},
		{"react@19.0.0/es2022/react.mjs", "/react@19.0.0/es2022/react.mjs"},
		{"https://esm.sh/react@19.0.0", "/react@19.0.0"},
		{"https://esm.sh/react@19.0.0/es2022/react.mjs?dev", "/react@19.0.0/es2022/react.mjs"},
		{"esm.sh/@scope/pkg@1.0.0", "/@scope/pkg@1.0.0"},
		{"@scope/pkg", "/@scope/pkg"},
		{"gh/user/repo@main", "/gh/user/repo@main"},
		{"https://esm.sh/gh/user/repo@abc1234", "/gh/user/repo@abc1234"},
		{"/*react@1.0.0", "/react@1.0.0"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := parsePurgeInput(test.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.expected {
				t.Fatalf("parsePurgeInput(%q) = %q, want %q", test.input, got, test.expected)
			}
		})
	}
	if _, err := parsePurgeInput(""); err == nil {
		t.Fatal("expected an error for empty input")
	}
}

// solvePowForTest solves a challenge the same way the purge page does.
func solvePowForTest(challenge *powChallengeResponse) string {
	target := strings.Repeat("0", challenge.Difficulty)
	for nonce := 0; ; nonce++ {
		sum := sha256.Sum256([]byte(challenge.Salt + strconv.Itoa(nonce)))
		if strings.HasPrefix(hex.EncodeToString(sum[:]), target) {
			return strconv.Itoa(nonce)
		}
	}
}

func TestPowChallenge(t *testing.T) {
	challenge := newPowChallenge()
	if challenge == nil {
		t.Fatal("expected a challenge to be minted")
	}
	nonce := solvePowForTest(challenge)

	// a correctly solved challenge passes and is consumed (single-use)
	if !powVerify(challenge.ID, nonce) {
		t.Fatal("expected the solved challenge to verify")
	}
	if powVerify(challenge.ID, nonce) {
		t.Fatal("expected the challenge to be single-use")
	}

	// an unsolved challenge is rejected
	unsolved := newPowChallenge()
	if powVerify(unsolved.ID, "0") {
		t.Fatal("expected an invalid nonce to be rejected")
	}

	// an expired challenge is rejected
	powChallengeStore.Lock()
	powChallengeStore.m["expired"] = powChallenge{salt: "s", expiresAt: time.Now().Add(-time.Minute)}
	powChallengeStore.Unlock()
	if powVerify("expired", "0") {
		t.Fatal("expected an expired challenge to be rejected")
	}
}

func TestPurgePackageCache(t *testing.T) {
	wd := t.TempDir()
	fs, err := storage.NewFSStorage(filepath.Join(wd, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	logger, err := log.New("")
	if err != nil {
		t.Fatal(err)
	}
	logger.SetOutput(io.Discard)

	metaDB := NewBuildMetaDB(fs)
	esm := EsmPath{PkgName: "example", PkgVersion: "1.0.0"}

	// seed build outputs, types, metadata, resolution caches and the local store
	for _, key := range []string{
		"modules/example@1.0.0/es2022/example.mjs",
		"modules/example@1.0.0/es2022/example.mjs.map",
		"modules/example@1.0.0/esnext/example.mjs",
		"types/example@1.0.0/index.d.ts",
	} {
		if err := fs.Put(key, io.NopCloser(strings.NewReader("x"))); err != nil {
			t.Fatal(err)
		}
	}
	buildPath := "/example@1.0.0/es2022/example.mjs"
	if err := metaDB.Put(buildPath, encodeBuildMeta(&BuildMeta{Dts: "/example@1.0.0/es2022/index.d.ts"})); err != nil {
		t.Fatal(err)
	}
	setCacheItem("npm:example@latest", &npm.PackageJSON{Version: "1.0.0"}, time.Minute)
	setCacheItem("404:example@latest", "boom", time.Minute)
	setCacheItem("npm:example@1.0.0", &npm.PackageJSON{Version: "1.0.0"}, time.Minute)

	oldWorkDir := config.WorkDir
	config.WorkDir = filepath.Join(wd, "esmd")
	defer func() { config.WorkDir = oldWorkDir }()
	installDir := filepath.Join(config.WorkDir, "npm", esm.PackageId())
	if err := os.MkdirAll(filepath.Join(installDir, "node_modules", esm.PkgName), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "node_modules", esm.PkgName, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	resp, err := purgePackageCache(&NpmRC{}, metaDB, fs, logger, esm, "https://esm.sh")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Package != "example" || resp.Version != "1.0.0" {
		t.Fatalf("unexpected purge response identity: %+v", resp)
	}
	if resp.Rebuild != "https://esm.sh/example@1.0.0" {
		t.Fatalf("rebuild = %q", resp.Rebuild)
	}

	// all build outputs and types must be gone
	for _, key := range []string{
		"modules/example@1.0.0/es2022/example.mjs",
		"modules/example@1.0.0/es2022/example.mjs.map",
		"modules/example@1.0.0/esnext/example.mjs",
		"types/example@1.0.0/index.d.ts",
	} {
		if _, _, err := fs.Get(key); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("expected %s to be purged, got err=%v", key, err)
		}
	}
	// the build metadata must be gone as well
	if _, err := metaDB.Get(buildPath); err == nil {
		t.Fatal("expected build metadata to be purged")
	}
	// the resolution caches must be invalidated
	for _, key := range []string{"npm:example@latest", "404:example@latest", "npm:example@1.0.0"} {
		if _, ok := getCacheItem(key); ok {
			t.Fatalf("expected cache key %s to be purged", key)
		}
	}
	// the local npm store copy must be removed
	if _, err := os.Lstat(installDir); !os.IsNotExist(err) {
		t.Fatalf("expected install dir %s to be removed, got err=%v", installDir, err)
	}
}

func TestPurgePackageCacheScoped(t *testing.T) {
	wd := t.TempDir()
	fs, err := storage.NewFSStorage(filepath.Join(wd, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	logger, err := log.New("")
	if err != nil {
		t.Fatal(err)
	}
	logger.SetOutput(io.Discard)

	esm := EsmPath{PkgName: "@scope/pkg", PkgVersion: "1.0.0"}
	if err := fs.Put("modules/@scope/pkg@1.0.0/es2022/pkg.mjs", io.NopCloser(strings.NewReader("x"))); err != nil {
		t.Fatal(err)
	}
	resp, err := purgePackageCache(&NpmRC{}, NewBuildMetaDB(fs), fs, logger, esm, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(resp.Purged, "modules/@scope/pkg@1.0.0/es2022/pkg.mjs") {
		t.Fatalf("expected the module file in purged list, got: %v", resp.Purged)
	}
	if !slices.Contains(resp.Purged, "meta:/@scope/pkg@1.0.0/es2022/pkg.mjs") {
		t.Fatalf("expected the build metadata in purged list, got: %v", resp.Purged)
	}
	if _, _, err := fs.Get("modules/@scope/pkg@1.0.0/es2022/pkg.mjs"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected the scoped package build to be purged, got err=%v", err)
	}
}
