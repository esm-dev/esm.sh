package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
	"github.com/ije/gox/utils"
)

// proof-of-work parameters for `POST /purge`. Every purge must first solve a
// hashcash-style SHA-256 challenge, so bots cannot drive the expensive
// purge-then-rebuild cycle for free.
const (
	powDifficulty       = 4             // leading zero hex chars required
	powChallengeTTL     = 2 * time.Minute
	powChallengeMaxSize = 10000         // in-memory cap of pending challenges
)

type powChallenge struct {
	salt       string
	difficulty int
	expiresAt  time.Time
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
	ExpiresAt  int64  `json:"expiresAt"`
}

// newPowChallenge mints a one-time proof-of-work challenge.
func newPowChallenge() (*powChallengeResponse, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	now := time.Now()
	challenge := powChallenge{
		salt:       hex.EncodeToString(salt),
		difficulty: powDifficulty,
		expiresAt:  now.Add(powChallengeTTL),
	}
	powChallengeStore.Lock()
	// garbage collect expired challenges
	for k, c := range powChallengeStore.m {
		if now.After(c.expiresAt) {
			delete(powChallengeStore.m, k)
		}
	}
	if len(powChallengeStore.m) >= powChallengeMaxSize {
		powChallengeStore.Unlock()
		return nil, errors.New("too many pending challenges")
	}
	idStr := hex.EncodeToString(id)
	powChallengeStore.m[idStr] = challenge
	powChallengeStore.Unlock()
	return &powChallengeResponse{
		ID:         idStr,
		Salt:       challenge.salt,
		Difficulty: challenge.difficulty,
		ExpiresAt:  challenge.expiresAt.Unix(),
	}, nil
}

// powVerify checks a solved challenge. A challenge is single-use: it is
// consumed on the first verification attempt, so a valid solution cannot be
// replayed to purge repeatedly.
func powVerify(id string, nonce string) bool {
	if id == "" || nonce == "" {
		return false
	}
	powChallengeStore.Lock()
	challenge, ok := powChallengeStore.m[id]
	if ok {
		delete(powChallengeStore.m, id)
	}
	powChallengeStore.Unlock()
	if !ok || time.Now().After(challenge.expiresAt) {
		return false
	}
	sum := sha256.Sum256([]byte(challenge.salt + nonce))
	return strings.HasPrefix(hex.EncodeToString(sum[:]), strings.Repeat("0", challenge.difficulty))
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

// purgeRefreshDistTag returns the package name and whether the requested
// specifier did not pin an exact version. When it did not, the cached "latest"
// (dist-tag) resolution must be dropped before resolving the target — otherwise
// a stale resolution would survive the purge and keep pointing at the old
// version. gh/pr requests are excluded: their resolutions are commitish values
// that never drift like a dist-tag.
func purgeRefreshDistTag(pathname string) (pkgName string, refresh bool) {
	if strings.HasPrefix(pathname, "/gh/") || strings.HasPrefix(pathname, "/github.com/") || strings.HasPrefix(pathname, "/pr/") || strings.HasPrefix(pathname, "/pkg.pr.new/") {
		return "", false
	}
	pkgName, maybeVersion, _ := splitEsmPath(pathname)
	version, _ := utils.SplitByFirstByte(maybeVersion, '&')
	if v, e := url.PathUnescape(version); e == nil {
		version = v
	}
	return pkgName, !npm.IsExactVersion(npm.NormalizePackageVersion(version))
}

// purgePackageCache removes every cached artifact of a resolved package/version:
//   - the in-memory npm resolution caches (so the next request re-queries the registry),
//   - the build outputs, source maps and type declarations in the storage,
//   - the build metadata (and its in-memory copy) for each removed module,
//   - the local npm store copy (so the package is re-installed on the next build).
//
// The next request for the purged URL rebuilds the module from scratch.
func purgePackageCache(npmrc *NpmRC, metaDB *BuildMetaDB, esmStorage storage.Storage, logger *log.Logger, esmPath EsmPath, origin string) (*purgeResponse, error) {
	pkgId := esmPath.PackageId()
	resp := &purgeResponse{
		Package: esmPath.PkgName,
		Version: esmPath.PkgVersion,
	}

	// 1. drop the npm resolution caches so a stale package.json / 404 never
	// survives the purge. Always clear the dist-tag entry as well: a manual
	// purge is an explicit "give me the current version" signal.
	versions := map[string]bool{
		"latest": true,
	}
	versions[npm.NormalizePackageVersion(esmPath.PkgVersion)] = true
	for version := range versions {
		for _, prefix := range []string{"npm:", "404:"} {
			key := prefix + esmPath.PkgName + "@" + version
			if _, ok := getCacheItem(key); ok {
				deleteCacheItem(key)
				resp.CacheKeys = append(resp.CacheKeys, key)
			}
		}
	}
	if esmPath.GhPrefix || esmPath.PrPrefix {
		prefix := "gh/"
		if esmPath.PrPrefix {
			prefix = "pr/"
		}
		key := prefix + esmPath.PkgName + "@" + esmPath.PkgVersion
		if _, ok := getCacheItem(key); ok {
			deleteCacheItem(key)
			resp.CacheKeys = append(resp.CacheKeys, key)
		}
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
	times := l.entries[ip]
	kept := times[:0]
	for _, t := range times {
		if t > cutoff {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.entries[ip] = kept
		return false
	}
	l.entries[ip] = append(kept, now)
	return true
}
