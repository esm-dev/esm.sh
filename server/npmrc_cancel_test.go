package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
)

func TestCanceledResolutionWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const name = "cancel-resolution-test"
		const key = "npm:" + name + "@^1"
		rc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
		t.Cleanup(func() {
			deleteCacheItemsWithPrefix("npm:" + name + "@")
			npmResolutions.Remove(rc.resolutionKey(name, "^1"))
		})
		started, release := make(chan struct{}), make(chan struct{})
		calls := 0
		fetch := func(ctx context.Context) (*npm.PackageJSONRaw, error) {
			calls++
			if calls == 1 {
				close(started)
			}
			select {
			case <-release:
				var raw npm.PackageJSONRaw
				raw.Name, raw.Version = name, "1.0.0"
				return &raw, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		leader := make(chan error, 1)
		go func() {
			_, err := rc.cachedResolution(context.Background(), name, "^1", key, fetch)
			leader <- err
		}()
		defer func() {
			close(release)
			if err := <-leader; err != nil {
				t.Errorf("shared resolution failed: %v", err)
			}
		}()
		<-started
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		waiter := make(chan error, 1)
		go func() {
			_, err := rc.cachedResolution(ctx, name, "^1", key, fetch)
			waiter <- err
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case err := <-waiter:
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		default:
			t.Fatal("canceled caller is still waiting for the shared resolution")
		}
		if calls != 1 {
			t.Fatalf("fetch calls = %d, want 1", calls)
		}
	})
}

func TestCanceledMetadataWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, release := make(chan struct{}), make(chan struct{})
		previous := http.DefaultTransport
		defer func() { http.DefaultTransport = previous }()
		calls := 0
		http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				close(started)
			}
			<-release
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"versions":{"1.0.0":{"name":"cancel-shared-test","version":"1.0.0"}}}`))}, nil
		})
		rc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: "http://shared.invalid/"}}}
		leader := make(chan error, 1)
		go func() {
			_, _, err := rc.fetchPackageMetadataContext(context.Background(), "cancel-shared-test", "^1", false)
			leader <- err
		}()
		<-started
		ctx, cancel := context.WithCancel(context.Background())
		waiter := make(chan error, 1)
		go func() {
			_, _, err := rc.fetchPackageMetadataContext(ctx, "cancel-shared-test", "^1", false)
			waiter <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-waiter; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		close(release)
		if err := <-leader; err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if calls != 1 {
			t.Fatalf("registry calls = %d, want 1", calls)
		}
	})
}

func TestNpmResolutionPurgeSkipsPendingMetadata(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const name = "purge-pending-resolution"
		rc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
		t.Cleanup(func() {
			deleteCacheItemsWithPrefix("npm:" + name + "@")
			npmResolutions.Remove(rc.resolutionKey(name, "latest"))
		})
		started, release := make(chan struct{}), make(chan struct{})
		previous := http.DefaultTransport
		defer func() {
			close(release)
			synctest.Wait()
			http.DefaultTransport = previous
		}()
		calls := 0
		http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			version := "2.0.0"
			if calls == 1 {
				close(started)
				<-release
				version = "1.0.0"
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"name":"` + name + `","version":"` + version + `"}`))}, nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		first := make(chan error, 1)
		go func() {
			_, err := rc.getPackageInfoContext(ctx, name, "latest")
			first <- err
		}()
		<-started
		cancel()
		if err := <-first; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if err := rc.purgeResolutions(name); err != nil {
			t.Fatal(err)
		}
		next := make(chan *npm.PackageJSON, 1)
		go func() {
			info, err := rc.getPackageInfo(name, "latest")
			if err != nil {
				t.Error(err)
			}
			next <- info
		}()
		synctest.Wait()
		select {
		case info := <-next:
			if info == nil || info.Version != "2.0.0" {
				t.Fatalf("resolution after purge = %+v, want 2.0.0", info)
			}
		default:
			t.Fatal("resolution after purge joined an old metadata fetch")
		}
	})
}

func TestCanceledCacheWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var locks keyedMutex
		unlock := locks.Lock("metadata")
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			release, err := locks.LockContext(ctx, "metadata")
			if release != nil {
				release()
			}
			done <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		acquired := make(chan struct{})
		go func() { release := locks.Lock("metadata"); close(acquired); release() }()
		synctest.Wait()
		select {
		case <-acquired:
			t.Fatal("canceled waiter released another owner")
		default:
		}
		unlock()
		<-acquired
	})
}

func TestNpmResolutionRequestDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		previous := http.DefaultTransport
		defer func() { http.DefaultTransport = previous }()
		var calls atomic.Int32
		http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		rc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: "http://deadline.invalid/"}}}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _, _, _, _, err := parseEsmPathContext(ctx, rc, "/cancel-deadline-test@^1")
		synctest.Wait()
		if !errors.Is(err, context.DeadlineExceeded) || calls.Load() == 0 {
			t.Fatalf("resolution: %v, calls: %d", err, calls.Load())
		}
		time.Sleep(npmQueryTimeout + time.Second)
		synctest.Wait()
	})
}
