package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
	setCacheItem("404:example@1.0.0", "boom", time.Minute)

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

	resp, err := purgePackageCache(&NpmRC{}, metaDB, fs, logger, esm, true, "https://esm.sh")
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
	// the resolution caches of the pinned version must be invalidated while
	// the dist-tag entry of a fixed-version purge is left alone
	for _, key := range []string{"npm:example@1.0.0", "404:example@1.0.0"} {
		if _, ok := getCacheItem(key); ok {
			t.Fatalf("expected cache key %s to be purged", key)
		}
	}
	for _, key := range []string{"npm:example@latest", "404:example@latest"} {
		if _, ok := getCacheItem(key); !ok {
			t.Fatalf("expected cache key %s to survive a fixed-version purge", key)
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
	setCacheItem("npm:@scope/pkg@latest", &npm.PackageJSON{Version: "1.0.0"}, time.Minute)
	if err := fs.Put("modules/@scope/pkg@1.0.0/es2022/pkg.mjs", io.NopCloser(strings.NewReader("x"))); err != nil {
		t.Fatal(err)
	}
	resp, err := purgePackageCache(&NpmRC{}, NewBuildMetaDB(fs), fs, logger, esm, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(resp.Purged, "modules/@scope/pkg@1.0.0/es2022/pkg.mjs") {
		t.Fatalf("expected the module file in purged list, got: %v", resp.Purged)
	}
	if !slices.Contains(resp.Purged, "meta:/@scope/pkg@1.0.0/es2022/pkg.mjs") {
		t.Fatalf("expected the build metadata in purged list, got: %v", resp.Purged)
	}
	if !slices.Contains(resp.CacheKeys, "npm:@scope/pkg@latest") {
		t.Fatalf("expected the dist-tag resolution in cache keys, got: %v", resp.CacheKeys)
	}
	if _, _, err := fs.Get("modules/@scope/pkg@1.0.0/es2022/pkg.mjs"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected the scoped package build to be purged, got err=%v", err)
	}
}

func TestPurgeSession(t *testing.T) {
	oldSecret := config.GithubClientSecret
	config.GithubClientSecret = "test-secret"
	defer func() { config.GithubClientSecret = oldSecret }()

	session := &purgeSession{Login: "octocat", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	value := signPurgeSession(session)
	if got := parsePurgeSession(value); got == nil || got.Login != "octocat" {
		t.Fatalf("expected a valid session, got %+v", got)
	}

	// a tampered payload must be rejected
	if parsePurgeSession(value[:len(value)-1]+"0") != nil {
		t.Fatal("expected a tampered session to be rejected")
	}
	// an expired session must be rejected
	expired := signPurgeSession(&purgeSession{Login: "octocat", ExpiresAt: time.Now().Add(-time.Minute).Unix()})
	if parsePurgeSession(expired) != nil {
		t.Fatal("expected an expired session to be rejected")
	}
	// a session signed with another secret must be rejected
	config.GithubClientSecret = "other-secret"
	if parsePurgeSession(value) != nil {
		t.Fatal("expected a session signed with another secret to be rejected")
	}
}

func TestPurgeOAuthLoginRedirect(t *testing.T) {
	oldID, oldSecret, oldOAuthOrigin := config.GithubClientID, config.GithubClientSecret, config.CdnOrigin
	config.PurgeCache = true
	config.GithubClientID = "client-id"
	config.GithubClientSecret = "client-secret"
	config.CdnOrigin = "https://esm.sh"
	defer func() {
		config.GithubClientID, config.GithubClientSecret, config.CdnOrigin = oldID, oldSecret, oldOAuthOrigin
	}()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "https://esm.sh/purge/login", nil)
	purgeOAuthLogin(w, r)
	res := w.Result()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("expected a redirect, got %d", res.StatusCode)
	}
	location := res.Header.Get("Location")
	if !strings.HasPrefix(location, githubOAuthAuthorizeURL) ||
		!strings.Contains(location, "client_id=client-id") ||
		!strings.Contains(location, "scope=read%3Auser") {
		t.Fatalf("unexpected authorize redirect: %s", location)
	}
	var state string
	for _, cookie := range res.Cookies() {
		if cookie.Name == purgeOAuthStateCookie {
			state = cookie.Value
		}
	}
	if state == "" {
		t.Fatal("expected an oauth state cookie")
	}
}

func TestPurgeOAuthCallback(t *testing.T) {
	oldID, oldSecret, oldOrigin := config.GithubClientID, config.GithubClientSecret, config.CdnOrigin
	oldTokenURL, oldUserURL := githubOAuthTokenURL, githubUserAPIURL
	defer func() {
		config.GithubClientID, config.GithubClientSecret, config.CdnOrigin = oldID, oldSecret, oldOrigin
		githubOAuthTokenURL, githubUserAPIURL = oldTokenURL, oldUserURL
	}()
	config.PurgeCache = true
	config.GithubClientID = "client-id"
	config.GithubClientSecret = "client-secret"
	config.CdnOrigin = "https://esm.sh"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"token"}`))
		case "/user":
			if r.Header.Get("Authorization") != "Bearer token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"login":"octocat"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	githubOAuthTokenURL = server.URL + "/token"
	githubUserAPIURL = server.URL + "/user"

	logger, err := log.New("")
	if err != nil {
		t.Fatal(err)
	}
	logger.SetOutput(io.Discard)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "https://esm.sh/purge/callback?code=abc&state=state", nil)
	r.AddCookie(&http.Cookie{Name: purgeOAuthStateCookie, Value: "state"})
	purgeOAuthCallback(w, r, logger)
	res := w.Result()
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/purge" {
		t.Fatalf("expected a redirect to /purge, got %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	var sessionCookie *http.Cookie
	for _, cookie := range res.Cookies() {
		if cookie.Name == purgeSessionCookie {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil || parsePurgeSession(sessionCookie.Value) == nil || parsePurgeSession(sessionCookie.Value).Login != "octocat" {
		t.Fatalf("expected a signed session for octocat, got %+v", sessionCookie)
	}
}

func TestPurgeOAuthCallbackRejectsBadState(t *testing.T) {
	oldID, oldSecret := config.GithubClientID, config.GithubClientSecret
	config.PurgeCache = true
	config.GithubClientID = "client-id"
	config.GithubClientSecret = "client-secret"
	defer func() { config.GithubClientID, config.GithubClientSecret = oldID, oldSecret }()

	logger, err := log.New("")
	if err != nil {
		t.Fatal(err)
	}
	logger.SetOutput(io.Discard)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "https://esm.sh/purge/callback?code=abc&state=forged", nil)
	r.AddCookie(&http.Cookie{Name: purgeOAuthStateCookie, Value: "state"})
	purgeOAuthCallback(w, r, logger)
	if location := w.Result().Header.Get("Location"); !strings.Contains(location, "error=") {
		t.Fatalf("expected an error redirect, got %q", location)
	}
}

func TestCdnPurgeURLs(t *testing.T) {
	urls := cdnPurgeURLs("https://esm.sh", "example@1.0.0", []string{
		"modules/example@1.0.0/es2022/example.mjs",
		"modules/example@1.0.0/es2022/example.mjs",
		"modules/example@1.0.0/X-abc/es2022/example.mjs",
		"modules/x-0123456789abcdef0123456789abcdef01234567/es2022/example.mjs",
		"modules/*example@1.0.0/ea/example.mjs",
		"modules/transform/deadbeef.mjs",
		"types/example@1.0.0/index.d.ts",
		"install:/tmp/example@1.0.0",
		"meta:/example@1.0.0/es2022/example.mjs",
	})
	want := []string{
		"https://esm.sh/example@1.0.0",
		"https://esm.sh/example@1.0.0/es2022/example.mjs",
		"https://esm.sh/example@1.0.0/index.d.ts",
	}
	if !slices.Equal(urls, want) {
		t.Fatalf("cdnPurgeURLs = %v, want %v", urls, want)
	}
}

func TestCloudflarePurge(t *testing.T) {
	oldZone, oldToken, oldBase := config.CloudflareZoneID, config.CloudflareAPIToken, cloudflareAPIBaseURL
	defer func() {
		config.CloudflareZoneID, config.CloudflareAPIToken, cloudflareAPIBaseURL = oldZone, oldToken, oldBase
	}()
	config.CloudflareZoneID = "zone"
	config.CloudflareAPIToken = "token"

	var batches [][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/v4/zones/zone/purge_cache" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Files []string `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		batches = append(batches, body.Files)
		w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	cloudflareAPIBaseURL = server.URL

	logger, err := log.New("")
	if err != nil {
		t.Fatal(err)
	}
	logger.SetOutput(io.Discard)

	var urls []string
	for i := 0; i < 31; i++ {
		urls = append(urls, "https://esm.sh/pkg@"+strconv.Itoa(i))
	}
	purgeCloudflareCache(logger, urls)
	if len(batches) != 2 || len(batches[0]) != cloudflarePurgeBatchSize || len(batches[1]) != 1 {
		t.Fatalf("unexpected cloudflare batches: %v", batches)
	}
	if batches[0][0] != "https://esm.sh/pkg@0" || batches[1][0] != "https://esm.sh/pkg@30" {
		t.Fatalf("unexpected cloudflare urls: %v", batches)
	}
}
