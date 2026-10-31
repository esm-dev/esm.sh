package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	lru "github.com/hashicorp/golang-lru/v2"
)

const npmStaleTTL = 24 * time.Hour
const npmResolutionMaxSize = 256 << 10

var npmResolutions, _ = lru.New[string, *npmResolution](256)
var npmResolutionEpoch atomic.Uint64
var npmRefreshSlots = make(chan struct{}, 16)

// Persist only the selected version, never the potentially huge packument.
type npmResolution struct {
	Key     string             `json:"key"`
	SavedAt time.Time          `json:"savedAt"`
	Raw     npm.PackageJSONRaw `json:"raw"`
	info    *npm.PackageJSON
	retryAt atomic.Int64
}

func (rc *NpmRC) resolutionPrefix(pkg string) string {
	reg, _ := json.Marshal(rc.getRegistryByPackageName(pkg))
	sum := sha256.Sum256(append(reg, []byte("\x00"+pkg)...))
	return "npm-resolutions/v1/" + hex.EncodeToString(sum[:]) + "/"
}

func (rc *NpmRC) resolutionKey(pkg, query string) string {
	sum := sha256.Sum256([]byte(query))
	return rc.resolutionPrefix(pkg) + hex.EncodeToString(sum[:]) + ".json"
}

// A fixed set of slots bounds persistent fallback metadata to 64 MiB.
// Hash collisions only discard a backup; the full identity is checked on reads.
func resolutionSlot(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "npm-resolution-slots/v1/" + hex.EncodeToString(sum[:1]) + ".json"
}

func (rc *NpmRC) resolutionFreshKey(pkg, key string) string {
	reg := rc.getRegistryByPackageName(pkg)
	if reg.Registry != npmRegistry || reg.Token != "" || reg.User != "" || reg.Password != "" {
		key += ":" + rc.resolutionPrefix(pkg)
	}
	return key
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
	storageKey := rc.resolutionKey(pkg, query)
	unlock, err := cacheMutex.LockContext(ctx, "resolution:"+storageKey)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if v, ok := getCacheItem(key); ok {
		return v.(*npm.PackageJSON), nil
	}
	epoch := npmResolutionEpoch.Load()
	item, ok := npmResolutions.Get(storageKey)
	if !ok && rc.resolutionStorage != nil {
		release, err := cacheMutex.LockContext(ctx, "resolution-persistence")
		if err != nil {
			return nil, err
		}
		readCtx, stop := context.WithTimeout(ctx, time.Second)
		item, _ = rc.readResolutionObject(readCtx, resolutionSlot(storageKey))
		stop()
		if item != nil && item.Key == storageKey {
			npmResolutions.Add(storageKey, item)
		} else {
			item = nil
		}
		release()
	}
	if item != nil {
		age := time.Since(item.SavedAt)
		if age >= 0 && age <= ttl+npmStaleTTL {
			now, retryAt := time.Now(), item.retryAt.Load()
			if age >= ttl && now.UnixNano() >= retryAt && item.retryAt.CompareAndSwap(retryAt, now.Add(time.Minute).UnixNano()) {
				select {
				case npmRefreshSlots <- struct{}{}:
					go func() {
						defer func() { <-npmRefreshSlots }()
						refreshCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), npmQueryTimeout)
						defer stop()
						rc.refreshResolution(refreshCtx, pkg, key, storageKey, ttl, epoch, fetch)
					}()
				default:
					item.retryAt.Store(retryAt)
				}
			}
			return item.info, nil
		}
	}
	return rc.refreshResolution(ctx, pkg, key, storageKey, ttl, epoch, fetch)
}

func (rc *NpmRC) refreshResolution(ctx context.Context, pkg, key, storageKey string, ttl time.Duration, epoch uint64, fetch func(context.Context) (*npm.PackageJSONRaw, error)) (*npm.PackageJSON, error) {
	raw, err := fetch(ctx)
	unlock, lockErr := cacheMutex.LockContext(ctx, "resolution-persistence")
	if lockErr != nil {
		return nil, lockErr
	}
	defer unlock()
	if epoch != npmResolutionEpoch.Load() {
		if err != nil {
			return nil, err
		}
		return raw.ToNpmPackage(), nil
	}
	if err != nil {
		// A definitive denial/not-found must not resurrect a previously allowed version.
		var registryErr *npmRegistryError
		if strings.HasSuffix(err.Error(), " not found") || (errors.As(err, &registryErr) && registryErr.status >= 400 && registryErr.status < 500 && registryErr.status != 408 && registryErr.status != 429) {
			rc.deleteResolution(storageKey)
			deleteCacheItem(key)
		}
		return nil, err
	}
	item := &npmResolution{Key: storageKey, SavedAt: time.Now(), Raw: *raw, info: raw.ToNpmPackage()}
	data, marshalErr := json.Marshal(item)
	if marshalErr == nil && len(data) <= npmResolutionMaxSize {
		npmResolutions.Add(storageKey, item)
		if rc.resolutionStorage != nil {
			writeCtx, cancel := context.WithTimeout(ctx, time.Second)
			if s, ok := rc.resolutionStorage.(interface {
				PutContext(context.Context, string, io.Reader) error
			}); ok {
				s.PutContext(writeCtx, resolutionSlot(storageKey), bytes.NewReader(data))
			} else {
				rc.resolutionStorage.Put(resolutionSlot(storageKey), bytes.NewReader(data))
			}
			cancel()
		}
	} else {
		rc.deleteResolution(storageKey)
	}
	setCacheItem(key, item.info, ttl)
	// Existing callers use the exact-version alias when installing dependencies.
	setCacheItem(rc.resolutionFreshKey(pkg, "npm:"+pkg+"@"+raw.Version), item.info, ttl)
	return item.info, nil
}

// The caller holds resolution-persistence, also used by writers and purge.
func (rc *NpmRC) deleteResolution(key string) {
	npmResolutions.Remove(key)
	if rc.resolutionStorage == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	item, _ := rc.readResolutionObject(ctx, resolutionSlot(key))
	if item != nil && item.Key == key {
		rc.resolutionStorage.Delete(resolutionSlot(key))
	}
}

func (rc *NpmRC) readResolutionObject(ctx context.Context, key string) (*npmResolution, error) {
	var content io.ReadCloser
	var err error
	if s, ok := rc.resolutionStorage.(interface {
		GetContext(context.Context, string) (io.ReadCloser, storage.Stat, error)
	}); ok {
		content, _, err = s.GetContext(ctx, key)
	} else {
		content, _, err = rc.resolutionStorage.Get(key)
	}
	if err != nil {
		return nil, err
	}
	defer content.Close()
	var item npmResolution
	if err = json.NewDecoder(io.LimitReader(content, npmResolutionMaxSize)).Decode(&item); err != nil {
		return nil, err
	}
	if !npm.IsExactVersion(item.Raw.Version) {
		return nil, errors.New("invalid resolution version")
	}
	item.info = item.Raw.ToNpmPackage()
	return &item, nil
}

func (rc *NpmRC) purgeResolutions(pkg string) error {
	prefix := rc.resolutionPrefix(pkg)
	unlock := cacheMutex.Lock("resolution-persistence")
	defer unlock()
	npmResolutionEpoch.Add(1)
	for _, key := range npmResolutions.Keys() {
		if strings.HasPrefix(key, prefix) {
			npmResolutions.Remove(key)
		}
	}
	if rc.resolutionStorage != nil {
		keys, err := rc.resolutionStorage.List("npm-resolution-slots/v1/")
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		for _, key := range keys {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			item, readErr := rc.readResolutionObject(ctx, key)
			cancel()
			if readErr != nil && !errors.Is(readErr, storage.ErrNotFound) {
				return readErr
			}
			if item != nil && strings.HasPrefix(item.Key, prefix) {
				if err := rc.resolutionStorage.Delete(key); err != nil && !errors.Is(err, storage.ErrNotFound) {
					return err
				}
			}
		}
	}
	return nil
}
