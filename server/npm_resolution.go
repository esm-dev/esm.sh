package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/maphash"
	"strings"
	"sync/atomic"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	lru "github.com/hashicorp/golang-lru/v2"
)

const npmStaleTTL = 24 * time.Hour

var npmResolutions, _ = lru.New[string, *npmResolution](256)
var npmRefreshSlots = make(chan struct{}, 16)

// A purge bumps the epoch of a package, so a fetch started before it must not fill the caches.
// Packages share a fixed set of epochs, a collision only skips one cache fill.
var npmResolutionEpochs [256]atomic.Uint64
var npmResolutionSeed = maphash.MakeSeed()

type npmResolution struct {
	savedAt time.Time
	info    *npm.PackageJSON
	retryAt atomic.Int64
}

func npmResolutionEpoch(pkg string) *atomic.Uint64 {
	return &npmResolutionEpochs[maphash.String(npmResolutionSeed, pkg)%uint64(len(npmResolutionEpochs))]
}

// [warning] hold it while checking the epoch and filling the caches, a purge must not interleave
func lockNpmResolution(pkg string) func() {
	return cacheMutex.Lock("npm-resolution:" + pkg)
}

func (rc *NpmRC) resolutionPrefix(pkg string) string {
	reg, _ := json.Marshal(rc.getRegistryByPackageName(pkg))
	sum := sha256.Sum256(append(reg, []byte("\x00"+pkg)...))
	return hex.EncodeToString(sum[:]) + "/"
}

func (rc *NpmRC) resolutionKey(pkg, query string) string {
	return rc.resolutionPrefix(pkg) + query
}

func (rc *NpmRC) resolutionFreshKey(pkg, key string) string {
	reg := rc.getRegistryByPackageName(pkg)
	if reg.Registry != npmRegistry || reg.Token != "" || reg.User != "" || reg.Password != "" {
		key += ":" + rc.resolutionPrefix(pkg)
	}
	return key
}

func (rc *NpmRC) packumentKey(pkg string) string {
	reg := rc.getRegistryByPackageName(pkg)
	if reg.Token != "" || reg.User != "" || reg.Password != "" {
		return reg.Registry + pkg + rc.resolutionPrefix(pkg)
	}
	return reg.Registry + pkg
}

func (rc *NpmRC) cachedResolution(ctx context.Context, pkg, query, key string, fetch func(context.Context) (*npm.PackageJSONRaw, error)) (*npm.PackageJSON, error) {
	ctx, cancel := context.WithTimeout(ctx, npmQueryTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	missingKey := "404:" + rc.getRegistryByPackageName(pkg).Registry + pkg + "@"
	for _, k := range []string{missingKey, missingKey + query} {
		if message := negativeCache.get(k); message != "" {
			return nil, errors.New(message)
		}
	}
	ttl := time.Duration(config.NpmQueryCacheTTL) * time.Second
	if ttl <= 0 {
		raw, err := fetch(ctx)
		if err != nil {
			return nil, err
		}
		return raw.ToNpmPackage(), nil
	}
	key = rc.resolutionFreshKey(pkg, key)
	if v, ok := getCacheItem(key); ok {
		return v.(*npm.PackageJSON), nil
	}
	lruKey := rc.resolutionKey(pkg, query)
	unlock, err := cacheMutex.LockContext(ctx, "resolution:"+lruKey)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if v, ok := getCacheItem(key); ok {
		return v.(*npm.PackageJSON), nil
	}
	epoch := npmResolutionEpoch(pkg).Load()
	if item, ok := npmResolutions.Get(lruKey); ok {
		age := time.Since(item.savedAt)
		if age >= 0 && age <= ttl+npmStaleTTL {
			now, retryAt := time.Now(), item.retryAt.Load()
			if age >= ttl && now.UnixNano() >= retryAt && item.retryAt.CompareAndSwap(retryAt, now.Add(time.Minute).UnixNano()) {
				select {
				case npmRefreshSlots <- struct{}{}:
					go func() {
						defer func() { <-npmRefreshSlots }()
						refreshCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), npmQueryTimeout)
						defer stop()
						rc.refreshResolution(refreshCtx, pkg, key, lruKey, ttl, epoch, fetch)
					}()
				default:
					item.retryAt.Store(retryAt)
				}
			}
			return item.info, nil
		}
	}
	return rc.refreshResolution(ctx, pkg, key, lruKey, ttl, epoch, fetch)
}

func (rc *NpmRC) refreshResolution(ctx context.Context, pkg, key, lruKey string, ttl time.Duration, epoch uint64, fetch func(context.Context) (*npm.PackageJSONRaw, error)) (*npm.PackageJSON, error) {
	raw, err := fetch(ctx)
	unlock := lockNpmResolution(pkg)
	defer unlock()
	if epoch != npmResolutionEpoch(pkg).Load() {
		if err != nil {
			return nil, err
		}
		return raw.ToNpmPackage(), nil
	}
	if err != nil {
		// A definitive denial/not-found must not resurrect a previously allowed version.
		var registryErr *npmRegistryError
		if strings.HasSuffix(err.Error(), " not found") || (errors.As(err, &registryErr) && registryErr.status >= 400 && registryErr.status < 500 && registryErr.status != 408 && registryErr.status != 429) {
			npmResolutions.Remove(lruKey)
			deleteCacheItem(key)
		}
		return nil, err
	}
	item := &npmResolution{savedAt: time.Now(), info: raw.ToNpmPackage()}
	npmResolutions.Add(lruKey, item)
	setCacheItem(key, item.info, ttl)
	// Existing callers use the exact-version alias when installing dependencies.
	setCacheItem(rc.resolutionFreshKey(pkg, "npm:"+pkg+"@"+raw.Version), item.info, ttl)
	return item.info, nil
}

func (rc *NpmRC) purgeResolutions(pkg string) {
	prefix := rc.resolutionPrefix(pkg)
	unlock := lockNpmResolution(pkg)
	defer unlock()
	npmResolutionEpoch(pkg).Add(1)
	packumentCache.Remove(rc.packumentKey(pkg))
	for _, key := range npmResolutions.Keys() {
		if strings.HasPrefix(key, prefix) {
			npmResolutions.Remove(key)
		}
	}
}
