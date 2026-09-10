package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

// proof-of-work parameters for `POST /purge`. Every purge must first solve a
// hashcash-style SHA-256 challenge, so bots cannot drive the expensive
// purge-then-rebuild cycle for free.
const (
	powDifficulty       = 4 // leading zero hex chars required
	powChallengeTTL     = 2 * time.Minute
	powChallengeMaxSize = 10000 // in-memory cap of pending challenges
)

type powChallenge struct {
	salt      string
	expiresAt time.Time
}

var powChallengeStore = struct {
	sync.Mutex
	m map[string]powChallenge
}{m: make(map[string]powChallenge)}

// powChallengeResponse is the JSON body of `GET /purge/challenge`.
type powChallengeResponse struct {
	ID         string `json:"id"`
	Salt       string `json:"salt"`
	Difficulty int    `json:"difficulty"`
}

// randomHex returns cryptographically-secure random bytes encoded as hex.
func randomHex(size int) string {
	buffer := make([]byte, size)
	rand.Read(buffer) // crypto/rand.Read never fails since Go 1.24
	return hex.EncodeToString(buffer)
}

// newPowChallenge mints a one-time proof-of-work challenge. Returns nil when
// too many challenges are pending (kept in check by garbage collecting
// expired ones once the store runs full).
func newPowChallenge() *powChallengeResponse {
	now := time.Now()
	powChallengeStore.Lock()
	defer powChallengeStore.Unlock()
	if len(powChallengeStore.m) >= powChallengeMaxSize {
		for id, stored := range powChallengeStore.m {
			if now.After(stored.expiresAt) {
				delete(powChallengeStore.m, id)
			}
		}
		if len(powChallengeStore.m) >= powChallengeMaxSize {
			return nil
		}
	}
	challenge := powChallenge{salt: randomHex(16), expiresAt: now.Add(powChallengeTTL)}
	id := randomHex(16)
	powChallengeStore.m[id] = challenge
	return &powChallengeResponse{ID: id, Salt: challenge.salt, Difficulty: powDifficulty}
}

// powVerify checks a solved challenge. A challenge is single-use: it is
// consumed on the first verification attempt, so a valid solution cannot be
// replayed to purge repeatedly.
func powVerify(id string, nonce string) bool {
	powChallengeStore.Lock()
	challenge, ok := powChallengeStore.m[id]
	delete(powChallengeStore.m, id)
	powChallengeStore.Unlock()
	if !ok || time.Now().After(challenge.expiresAt) {
		return false
	}
	sum := sha256.Sum256([]byte(challenge.salt + nonce))
	return strings.HasPrefix(hex.EncodeToString(sum[:]), strings.Repeat("0", powDifficulty))
}

// purgeLimiter bounds how often a single client can purge caches, since each
// purge forces a costly rebuild of the target package on the next request.
var purgeLimiter = &purgeRateLimiter{
	entries: make(map[string][]int64),
	window:  time.Minute,
	max:     5,
}

// purgeRequest is the JSON body of `POST /purge`.
type purgeRequest struct {
	// URL is an esm.sh URL (e.g. `https://esm.sh/react@19.0.0`) or a bare
	// package specifier (e.g. `react@19.0.0`, `@scope/pkg`, `gh/user/repo@main`).
	URL string `json:"url"`
	// Challenge and Nonce are the solved proof-of-work from
	// `GET /purge/challenge` (see newPowChallenge / powVerify).
	Challenge string `json:"challenge"`
	Nonce     string `json:"nonce"`
}

// purgeResponse reports what a cache purge has removed.
type purgeResponse struct {
	Package   string   `json:"package"`
	Version   string   `json:"version"`
	Purged    []string `json:"purged"`
	CacheKeys []string `json:"cacheKeys,omitempty"`
	Rebuild   string   `json:"rebuild,omitempty"`
}

// parsePurgeInput normalizes a user-supplied esm.sh URL or bare package
// specifier into the pathname form accepted by `parseEsmPath`.
func parsePurgeInput(input string) (pathname string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("url is required")
	}
	// strip the scheme and host of a full URL (only the path matters)
	if i := strings.Index(input, "://"); i >= 0 {
		rest := input[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			input = rest[j:]
		} else {
			input = "/"
		}
	} else if strings.HasPrefix(input, "esm.sh/") {
		input = "/" + strings.TrimPrefix(input, "esm.sh/")
	}
	if !strings.HasPrefix(input, "/") {
		input = "/" + input
	}
	// strip the query string
	if i := strings.IndexByte(input, '?'); i >= 0 {
		input = input[:i]
	}
	// strip the `*` "external all" marker
	if strings.HasPrefix(input, "/*") {
		input = "/" + strings.TrimPrefix(input, "/*")
	}
	return input, nil
}

// purgePackageCache removes every cached artifact of a resolved package/version:
//   - the in-memory npm resolution caches (so the next request re-queries the registry),
//   - the build outputs, source maps and type declarations in the storage,
//   - the build metadata (and its in-memory copy) for each removed module,
//   - the local npm store copy (so the package is re-installed on the next build).
//
// The next request for the purged URL rebuilds the module from scratch.
func purgePackageCache(npmrc *NpmRC, metaDB *BuildMetaDB, esmStorage storage.Storage, logger *log.Logger, esmPath EsmPath, exactVersion bool, origin string) (*purgeResponse, error) {
	pkgId := esmPath.PackageId()
	resp := &purgeResponse{
		Package:   esmPath.PkgName,
		Version:   esmPath.PkgVersion,
		Purged:    []string{},
		CacheKeys: []string{},
	}
	dropKey := func(key string) {
		if _, ok := getCacheItem(key); ok {
			deleteCacheItem(key)
			resp.CacheKeys = append(resp.CacheKeys, key)
		}
	}

	// 1. drop the npm resolution caches so a stale package.json / 404 never
	// survives the purge. For unpinned requests (bare names / dist-tags /
	// semver ranges) the dist-tag entry is dropped as well: the target was
	// resolved through it, and a manual purge is an explicit "give me the
	// current version" signal.
	versions := map[string]bool{
		npm.NormalizePackageVersion(esmPath.PkgVersion): true,
	}
	if !exactVersion {
		versions["latest"] = true
	}
	for version := range versions {
		dropKey("npm:" + esmPath.PkgName + "@" + version)
		dropKey("404:" + esmPath.PkgName + "@" + version)
	}
	if esmPath.GhPrefix || esmPath.PrPrefix {
		prefix := "gh/"
		if esmPath.PrPrefix {
			prefix = "pr/"
		}
		dropKey(prefix + esmPath.PkgName + "@" + esmPath.PkgVersion)
	}

	// 2. remove build outputs, source maps and type declarations from storage,
	// along with the build metadata of each removed module so it gets rebuilt.
	for _, dir := range []string{"modules/", "types/"} {
		keys, err := esmStorage.DeleteAll(dir + pkgId + "/")
		if err != nil {
			logger.Errorf("storage.DeleteAll(%s%s/): %v", dir, pkgId, err)
			return nil, err
		}
		for _, key := range keys {
			resp.Purged = append(resp.Purged, key)
			if strings.HasPrefix(key, "modules/") {
				// drop the build metadata (and its in-memory copy) so the module
				// is rebuilt on the next request. Files like source maps and
				// tree-shaken variants have no metadata of their own, so a
				// "not found" here is expected and safe to ignore — the router
				// also self-heals a stale meta by rebuilding on a missing file.
				buildPath := "/" + strings.TrimPrefix(key, "modules/")
				metaDB.Delete(buildPath)
				cacheLRU.Remove(buildPath)
				resp.Purged = append(resp.Purged, "meta:"+buildPath)
			}
		}
	}

	// 3. remove the local npm store copy so the tarball is re-fetched.
	installDir := filepath.Join(npmrc.StoreDir(), pkgId)
	if existsDir(installDir) {
		if err := os.RemoveAll(installDir); err != nil {
			logger.Errorf("removeAll(%s): %v", installDir, err)
			return nil, err
		}
		resp.Purged = append(resp.Purged, "install:"+installDir)
	}

	if origin != "" {
		resp.Rebuild = origin + "/" + pkgId
	}
	return resp, nil
}

// purgeRateLimiter is a tiny in-memory per-IP limiter that keeps the (costly)
// purge-then-rebuild cycle from being abused.
type purgeRateLimiter struct {
	mu      sync.Mutex
	entries map[string][]int64
	window  time.Duration
	max     int
}

func (l *purgeRateLimiter) allow(ip string) bool {
	now := time.Now().UnixMilli()
	cutoff := now - l.window.Milliseconds()
	l.mu.Lock()
	defer l.mu.Unlock()
	times := slices.DeleteFunc(l.entries[ip], func(when int64) bool { return when <= cutoff })
	if len(times) >= l.max {
		l.entries[ip] = times
		return false
	}
	l.entries[ip] = append(times, now)
	return true
}
