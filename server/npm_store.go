package server

import (
	"context"
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ije/gox/log"
)

const npmPackageTTL = time.Hour

var npmStore *npmPackageStore

type npmPackageStore struct {
	mu      sync.Mutex
	purgeMu sync.Mutex
	dir     string
	logger  *log.Logger
	items   map[string]*npmPackageUse
}

type npmPackageUse struct {
	key     string
	refs    int
	expires time.Time
	timer   *time.Timer
	purging chan struct{}
}

func (s *npmPackageStore) use(key string, ttl time.Duration) func() {
	// [warning] an expired key is removed as a whole, so accept package roots only, as `restore` finds them
	for rest := key; ; {
		seg, next, more := strings.Cut(rest, "/")
		if seg == "" || seg[0] == '.' || (strings.LastIndexByte(seg, '@') > 0) == more {
			return func() {}
		}
		if !more {
			break
		}
		rest = next
	}
	s.mu.Lock()
	item := s.items[key]
	for item != nil && item.purging != nil {
		purging := item.purging
		s.mu.Unlock()
		<-purging
		s.mu.Lock()
		item = s.items[key]
	}
	if item == nil {
		item = &npmPackageUse{key: key}
		if s.items == nil {
			s.items = map[string]*npmPackageUse{}
		}
		s.items[key] = item
	}
	item.refs++
	if item.timer != nil {
		item.timer.Stop()
	}
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if item.refs--; item.refs > 0 {
			return
		}
		item.expires = time.Now().Add(ttl)
		if item.timer != nil {
			item.timer.Reset(ttl)
			return
		}
		item.timer = time.AfterFunc(ttl, func() { s.expire(item, false) })
	}
}

func (s *npmPackageStore) expire(item *npmPackageUse, now bool) {
	s.mu.Lock()
	// [warning] Stop and Reset cannot cancel a callback that has already started.
	if s.items[item.key] != item || item.refs > 0 || item.purging != nil || (!now && time.Now().Before(item.expires)) {
		s.mu.Unlock()
		return
	}
	item.purging = make(chan struct{})
	s.mu.Unlock()
	root, err := os.OpenRoot(s.dir)
	tmp := ".purge-" + strconv.FormatUint(rand.Uint64(), 36)
	if err == nil {
		defer root.Close()
		// [warning] `use` waits for the rename only, so move the package away before deleting it
		err = root.Rename(item.key, tmp)
	}
	s.mu.Lock()
	delete(s.items, item.key)
	close(item.purging)
	s.mu.Unlock()
	if err == nil {
		err = root.RemoveAll(tmp)
	}
	if err != nil && !os.IsNotExist(err) && s.logger != nil {
		s.logger.Errorf("npm expiry %s: %v", item.key, err)
	}
}

// purgeIdle removes idle packages, least recently used first, until the disk is no longer low or full.
func (s *npmPackageStore) purgeIdle() {
	s.purgeMu.Lock()
	defer s.purgeMu.Unlock()
	s.mu.Lock()
	idle := make([]*npmPackageUse, 0, len(s.items))
	for _, item := range s.items {
		if item.refs == 0 && item.purging == nil {
			idle = append(idle, item)
		}
	}
	slices.SortFunc(idle, func(a, b *npmPackageUse) int { return a.expires.Compare(b.expires) })
	s.mu.Unlock()
	for _, item := range idle {
		if status, _, _ := diskStatus(s.dir); status != DiskStatusLow && status != DiskStatusFull {
			return
		}
		s.expire(item, true)
	}
}

func (s *npmPackageStore) restore() (errs error) {
	filepath.WalkDir(s.dir, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			// [warning] skip an unreadable directory, the walk must still reach the other packages
			if !os.IsNotExist(err) {
				errs = errors.Join(errs, err)
			}
			return nil
		}
		if name == s.dir || !d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if strings.LastIndexByte(d.Name(), '@') > 0 {
			key, _ := filepath.Rel(s.dir, name)
			// [warning] spread the expiry, the restored packages must not all expire at once
			s.use(filepath.ToSlash(key), npmPackageTTL+rand.N(npmPackageTTL))()
			return fs.SkipDir
		}
		return nil
	})
	return
}

type npmPackageScopeKey struct{}

type npmPackageScope struct {
	mu       sync.Mutex
	releases map[string]func()
}

func withNpmPackageScope(ctx context.Context) (context.Context, func()) {
	if npmStore == nil || ctx.Value(npmPackageScopeKey{}) != nil {
		return ctx, func() {}
	}
	scope := &npmPackageScope{releases: map[string]func(){}}
	return context.WithValue(ctx, npmPackageScopeKey{}, scope), func() {
		scope.mu.Lock()
		defer scope.mu.Unlock()
		for _, release := range scope.releases {
			release()
		}
	}
}

func useNpmPackage(ctx context.Context, key string) func() {
	if npmStore == nil {
		return func() {}
	}
	if scope, ok := ctx.Value(npmPackageScopeKey{}).(*npmPackageScope); ok {
		scope.mu.Lock()
		if scope.releases[key] == nil {
			scope.releases[key] = npmStore.use(key, npmPackageTTL)
		}
		scope.mu.Unlock()
		return func() {}
	}
	return npmStore.use(key, npmPackageTTL)
}

func useNpmPackageDir(ctx context.Context, dir string) func() {
	// [warning] only a link names another store package, a plain dir belongs to the package of the work dir
	if npmStore != nil {
		if target, err := os.Readlink(dir); err == nil {
			if rel, ok := strings.CutPrefix(target, npmStore.dir+string(filepath.Separator)); ok {
				if key, _, ok := strings.Cut(filepath.ToSlash(rel), "/node_modules/"); ok {
					return useNpmPackage(ctx, key)
				}
			}
		}
	}
	return func() {}
}
