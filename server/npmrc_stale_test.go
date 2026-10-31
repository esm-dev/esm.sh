package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
)

func TestNpmResolutionCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const name = "resolution-cache-test"
		rc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
		var raw npm.PackageJSONRaw
		if err := json.Unmarshal([]byte(`{"name":"resolution-cache-test","version":"1.0.0","main":"index.js","exports":{".":"./index.js"},"dist":{"tarball":"http://fixture/package.tgz"}}`), &raw); err != nil {
			t.Fatal(err)
		}
		calls := 0
		var failure error
		var blocked chan struct{}
		lookup := func(query string) (*npm.PackageJSON, error) {
			return rc.cachedResolution(context.Background(), name, query, "npm:"+name+"@"+query, func(ctx context.Context) (*npm.PackageJSONRaw, error) {
				calls++
				if blocked != nil {
					select {
					case <-blocked:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				return &raw, failure
			})
		}
		check := func(want string) {
			t.Helper()
			p, err := lookup("^1")
			if err != nil || p.Version != want || p.Main != "index.js" || p.Exports.Len() != 1 || p.Dist.Tarball != "http://fixture/package.tgz" {
				t.Fatalf("lookup: %+v %v, want %s", p, err, want)
			}
		}
		check("1.0.0")
		check("1.0.0")
		if calls != 1 {
			t.Fatal("fresh hit fetched metadata")
		}
		time.Sleep(time.Duration(config.NpmQueryCacheTTL)*time.Second + time.Second)
		blocked = make(chan struct{})
		for range 1000 {
			check("1.0.0")
		}
		synctest.Wait()
		if calls != 2 {
			t.Fatalf("refresh calls = %d, want 2", calls)
		}
		time.Sleep(npmQueryTimeout + time.Second)
		synctest.Wait()
		blocked = nil
		time.Sleep(time.Minute)
		failure = errors.New("registry unavailable")
		check("1.0.0")
		synctest.Wait()
		before := calls
		for range 1000 {
			check("1.0.0")
		}
		if calls != before {
			t.Fatal("failed refresh has no cooldown")
		}
		time.Sleep(time.Minute + time.Second)
		failure, raw.Version = nil, "1.1.0"
		check("1.0.0")
		synctest.Wait()
		check("1.1.0")
		other := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry, Token: "other"}}}
		if rc.resolutionKey(name, "^1") == other.resolutionKey(name, "^1") || rc.resolutionFreshKey(name, "key") == other.resolutionFreshKey(name, "key") {
			t.Fatal("credentials share cache keys")
		}
		for _, status := range []int{401, 403, 404} {
			failure = nil
			query := fmt.Sprint("^", status)
			if _, err := lookup(query); err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Duration(config.NpmQueryCacheTTL)*time.Second + time.Second)
			failure = &npmRegistryError{status, "denied"}
			if _, err := lookup(query); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			if _, err := lookup(query); err == nil {
				t.Fatal("denial retained stale metadata")
			}
		}
		failure = nil
		item, _ := npmResolutions.Get(rc.resolutionKey(name, "^1"))
		item.savedAt = time.Now().Add(-time.Duration(config.NpmQueryCacheTTL)*time.Second - npmStaleTTL - time.Second)
		failure = errors.New("offline")
		if _, err := lookup("^1"); err == nil {
			t.Fatal("expired backup was served")
		}
		failure = nil
		check("1.1.0")
		time.Sleep(time.Duration(config.NpmQueryCacheTTL)*time.Second + time.Second)
		blocked = make(chan struct{})
		check("1.1.0")
		synctest.Wait()
		rc.purgeResolutions(name)
		close(blocked)
		synctest.Wait()
		if _, ok := npmResolutions.Get(rc.resolutionKey(name, "^1")); ok {
			t.Fatal("refresh resurrected purge")
		}
		blocked = nil
		for i := range 512 {
			if _, err := lookup(fmt.Sprint("^1.", i)); err != nil {
				t.Fatal(err)
			}
		}
		if npmResolutions.Len() > 256 {
			t.Fatal("unbounded cache")
		}
		deleteCacheItemsWithPrefix("npm:" + name + "@")
	})
}
