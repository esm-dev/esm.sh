package server

import (
	"context"
	"crypto/rand"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ije/gox/log"
)

const npmPackageTTL = 10 * time.Minute

var npmStore *npmPackageStore

type npmPackageStore struct {
	mu     sync.Mutex
	dir    string
	logger *log.Logger
	items  map[string]*npmPackageUse
}

type npmPackageUse struct {
	refs    int
	expires time.Time
	timer   *time.Timer
}

func (s *npmPackageStore) use(key string) func() {
	s.mu.Lock()
	item := s.items[key]
	if item == nil {
		item = &npmPackageUse{}
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
		item.expires = time.Now().Add(npmPackageTTL)
		if item.timer != nil {
			item.timer.Reset(npmPackageTTL)
			return
		}
		item.timer = time.AfterFunc(npmPackageTTL, func() {
			s.mu.Lock()
			// [warning] Stop and Reset cannot cancel a callback that has already started.
			if s.items[key] != item || item.refs > 0 || time.Now().Before(item.expires) {
				s.mu.Unlock()
				return
			}
			root, err := os.OpenRoot(s.dir)
			tmp := ".purge-" + rand.Text()
			if err == nil {
				defer root.Close()
				// [warning] rename before deleting so a new install cannot see a partial package.
				err = root.Rename(key, tmp)
			}
			if err == nil || os.IsNotExist(err) {
				delete(s.items, key)
			} else {
				item.expires = time.Now().Add(npmPackageTTL)
				item.timer.Reset(npmPackageTTL)
			}
			s.mu.Unlock()
			if err == nil {
				err = root.RemoveAll(tmp)
			}
			if err != nil && !os.IsNotExist(err) && s.logger != nil {
				s.logger.Errorf("npm expiry %s: %v", key, err)
			}
		})
	}
}

func (s *npmPackageStore) restore() error {
	return filepath.WalkDir(s.dir, func(name string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if name == s.dir || !d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if strings.LastIndexByte(d.Name(), '@') > 0 {
			key, err := filepath.Rel(s.dir, name)
			if err != nil {
				return err
			}
			s.use(filepath.ToSlash(key))()
			return fs.SkipDir
		}
		return nil
	})
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
			scope.releases[key] = npmStore.use(key)
		}
		scope.mu.Unlock()
		return func() {}
	}
	return npmStore.use(key)
}

func useNpmPackageDir(ctx context.Context, dir string) func() {
	if npmStore != nil {
		if target, err := os.Readlink(dir); err == nil {
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(dir), target)
			}
			dir = target
		}
		if rel, err := filepath.Rel(npmStore.dir, dir); err == nil && filepath.IsLocal(rel) {
			if key, _, ok := strings.Cut(filepath.ToSlash(rel), "/node_modules/"); ok {
				return useNpmPackage(ctx, key)
			}
		}
	}
	return func() {}
}
