package server

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

// purgeRequest is the JSON body of `POST /purge`.
type purgeRequest struct {
	// URL is an esm.sh URL (e.g. `https://esm.sh/react@19.0.0`) or a bare
	// package specifier (e.g. `react@19.0.0`, `@scope/pkg`, `gh/user/repo@main`).
	URL string `json:"url"`
	// Challenge and Nonce are the solved proof-of-work from
	// `GET /pow/challenge?scope=purge` (see newPowChallenge / powVerify).
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
func parsePurgeInput(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("url is required")
	}
	if u, err := url.Parse(input); err == nil && u.Host != "" {
		input = u.Path
	} else if rest, ok := strings.CutPrefix(input, "esm.sh/"); ok {
		input = "/" + rest
	}
	if i := strings.IndexByte(input, '?'); i >= 0 {
		input = input[:i]
	}
	input = strings.TrimPrefix(input, "/*")
	if !strings.HasPrefix(input, "/") {
		input = "/" + input
	}
	return input, nil
}

// purgePackageCache removes every cached artifact of a resolved package/version:
//   - the in-memory resolution caches of the package (the exact version plus the
//     raw requested specifier, so a stale range/dist-tag/date/git-ref never survives),
//   - the build outputs, source maps and type declarations in the storage,
//   - the build metadata (and its in-memory copy) for each removed module,
//   - the local npm store copy (so the package is re-installed on the next build).
//
// The next request for the purged URL rebuilds the module from scratch.
func purgePackageCache(npmrc *NpmRC, metaDB *BuildMetaDB, esmStorage storage.Storage, logger *log.Logger, esmPath EsmPath, origin string) (*purgeResponse, error) {
	pkgId := esmPath.PackageId()
	resp := &purgeResponse{
		Package:   esmPath.PkgName,
		Version:   esmPath.PkgVersion,
		Purged:    []string{},
		CacheKeys: []string{},
	}

	// 1. drop every in-memory resolution cache of the package. The requested
	// specifier may be a bare name, a semver range, a dist-tag, a date or a
	// git ref, each cached under its own key, so matching a single resolved
	// version is not enough — a manual purge must force a fresh resolution.
	prefixes := []string{
		"npm:" + esmPath.PkgName + "@",
		"404:" + esmPath.PkgName + "@",
		npmrc.getRegistryByPackageName(esmPath.PkgName).Registry + esmPath.PkgName + "@",
	}
	if esmPath.GhPrefix {
		prefixes = append(prefixes, "gh/"+esmPath.PkgName+"@")
	} else if esmPath.PrPrefix {
		prefixes = append(prefixes, "pr/"+esmPath.PkgName+"@")
	}
	for _, prefix := range prefixes {
		resp.CacheKeys = append(resp.CacheKeys, deleteCacheItemsWithPrefix(prefix)...)
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
		// also evict the matching entries from the CDN edge cache, so clients
		// do not keep serving the stale build until its TTLs expire
		purgeCloudflareCache(logger, cdnPurgeURLs(origin, pkgId, resp.Purged))
	}
	return resp, nil
}

// purgeRateAllowed reports whether the client is within the purge rate limit,
// since each purge forces a costly rebuild of the target package. The sliding
// window lives in the shared TTL cache, so idle clients are garbage collected.
func purgeRateAllowed(key string) bool {
	const (
		window = time.Minute
		max    = 5
	)
	key = "purge-rate:" + key
	unlock := cacheMutex.Lock(key)
	defer unlock()
	now := time.Now().UnixMilli()
	var times []int64
	if v, ok := getCacheItem(key); ok {
		times = v.([]int64)
	}
	times = slices.DeleteFunc(times, func(when int64) bool { return now-when > window.Milliseconds() })
	if len(times) >= max {
		setCacheItem(key, times, window)
		return false
	}
	setCacheItem(key, append(times, now), window)
	return true
}
